package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeClaude writes a bash script that mimics the claude CLI: it prints
// `stdoutBody` to stdout, optionally prints `stderrBody` to stderr, and
// exits with `exitCode`. Returns the absolute path to the script.
func writeFakeClaude(t *testing.T, stdoutBody, stderrBody string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	body := "#!/bin/sh\n"
	if stdoutBody != "" {
		body += "cat <<'EOF_STDOUT'\n" + stdoutBody + "\nEOF_STDOUT\n"
	}
	if stderrBody != "" {
		body += "cat 1>&2 <<'EOF_STDERR'\n" + stderrBody + "\nEOF_STDERR\n"
	}
	body += "exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return sign + string(digits)
}

func TestClaudeCLI_JSONEnvelopeSuccess(t *testing.T) {
	envelope := `{"result":"hello","is_error":false,"session_id":"sid","usage":{"input_tokens":10,"output_tokens":3}}`
	path := writeFakeClaude(t, envelope, "", 0)

	client := NewClaudeCLIClientWithPath(path)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		System: "system",
		Messages: []Message{
			{Role: RoleUser, Content: "say hello"},
			{Role: RoleAssistant, Content: "ignored"},
		},
		Model: "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("Content = %q, want hello", resp.Content)
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 3 {
		t.Errorf("tokens = (%d,%d), want (10,3)", resp.InputTokens, resp.OutputTokens)
	}
	if resp.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q, want claude-sonnet-4", resp.Model)
	}
}

func TestClaudeCLI_JSONEnvelope_IsError(t *testing.T) {
	envelope := `{"result":"boom","is_error":true,"session_id":"sid"}`
	path := writeFakeClaude(t, envelope, "", 0)
	client := NewClaudeCLIClientWithPath(path)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("status = %d, want 500", apiErr.StatusCode)
	}
}

func TestClaudeCLI_RawOutputFallback(t *testing.T) {
	// Non-JSON output should be returned verbatim (minus code fences).
	raw := "```text\nplain output\n```"
	path := writeFakeClaude(t, raw, "", 0)
	client := NewClaudeCLIClientWithPath(path)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
		Model:    "raw-model",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "plain output" {
		t.Errorf("Content = %q, want plain output", resp.Content)
	}
	if resp.Model != "raw-model" {
		t.Errorf("Model = %q, want raw-model", resp.Model)
	}
}

func TestClaudeCLI_EmptyOutputErrors(t *testing.T) {
	path := writeFakeClaude(t, "", "", 0)
	client := NewClaudeCLIClientWithPath(path)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty output") {
		t.Errorf("expected empty output error, got %v", err)
	}
}

func TestClaudeCLI_ErrorClassification(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantStatus int
		wantRetry  bool
	}{
		{"billing", "Your credit balance is too low", 400, false},
		{"quota exceeded", "Out of extra usage quota exceeded for this period", 400, false},
		{"auth", "Unauthorized: invalid api key", 401, false},
		{"rate limit", "Too many requests, rate limit hit", 429, true},
		{"overloaded", "Service overloaded, capacity exceeded (503)", 529, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFakeClaude(t, "", tt.stderr, 1)
			client := NewClaudeCLIClientWithPath(path)
			_, err := client.Complete(context.Background(), CompletionRequest{
				Messages: []Message{{Role: RoleUser, Content: "x"}},
			})
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *APIError, got %T %v", err, err)
			}
			if apiErr.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", apiErr.StatusCode, tt.wantStatus)
			}
			if apiErr.Retryable != tt.wantRetry {
				t.Errorf("retryable = %v, want %v", apiErr.Retryable, tt.wantRetry)
			}
		})
	}
}

func TestClaudeCLI_UnclassifiedErrorWraps(t *testing.T) {
	path := writeFakeClaude(t, "", "something weird happened", 1)
	client := NewClaudeCLIClientWithPath(path)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "something weird") {
		t.Errorf("error should include stderr text, got %v", err)
	}
}

func TestClaudeCLI_WithSkipPermissions_PassesFlag(t *testing.T) {
	// Echo back the args so we can verify --dangerously-skip-permissions is included.
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\necho \"$@\" | tr ' ' '\\n' >&2\necho '{\"result\":\"ok\",\"is_error\":false}'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := NewClaudeCLIClientWithPath(path).WithSkipPermissions()
	if _, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
		Model:    "m",
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	// Re-run capturing stderr to see the args. The first call's stderr was
	// consumed by Complete on success path; we re-run with a fresh client
	// against the same script.
	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
		Model:    "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Direct argument check by invoking buildArgs.
	args := client.buildArgs(CompletionRequest{Model: "m"})
	found := false
	for _, a := range args {
		if a == "--dangerously-skip-permissions" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --dangerously-skip-permissions flag in args, got %v", args)
	}
}

func TestClaudeCLI_BuildArgs_OmitsModelWhenEmpty(t *testing.T) {
	client := NewClaudeCLIClient()
	args := client.buildArgs(CompletionRequest{})
	for _, a := range args {
		if a == "--model" {
			t.Errorf("did not expect --model with empty Model, got %v", args)
		}
	}
}

func TestClaudeCLI_BuildArgs_IncludesModelWhenSet(t *testing.T) {
	client := NewClaudeCLIClient()
	args := client.buildArgs(CompletionRequest{Model: "claude-haiku"})
	if !contains(args, "claude-haiku") {
		t.Errorf("expected model in args, got %v", args)
	}
}

func TestBuildCLIPrompt_OmitsAssistantMessages(t *testing.T) {
	req := CompletionRequest{
		System: "sys",
		Messages: []Message{
			{Role: RoleUser, Content: "user1"},
			{Role: RoleAssistant, Content: "skip"},
			{Role: RoleUser, Content: "user2"},
		},
	}
	out := buildCLIPrompt(req)
	if !strings.Contains(out, "sys") || !strings.Contains(out, "user1") || !strings.Contains(out, "user2") {
		t.Errorf("missing expected content: %q", out)
	}
	if strings.Contains(out, "skip") {
		t.Errorf("assistant messages should be omitted: %q", out)
	}
}

func TestBuildStrippedEnv_AllowlistDropsSecrets(t *testing.T) {
	// Allowlist enforced after security audit H2: only the documented set of
	// safe vars survives; every secret-bearing variable gets stripped even
	// when present in the operator's shell environment.
	t.Setenv("ANTHROPIC_API_KEY", "sk-anthropic")
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("GITHUB_TOKEN", "ghp_xxx")
	t.Setenv("GEMINI_API_KEY", "ai-gemini")
	t.Setenv("FIRECRAWL_API_KEY", "fc-x")
	t.Setenv("PATH", "/usr/bin:/bin")

	env := buildStrippedEnv()
	for _, e := range env {
		for _, secret := range []string{
			"ANTHROPIC_API_KEY=", "OPENAI_API_KEY=", "GITHUB_TOKEN=",
			"GEMINI_API_KEY=", "FIRECRAWL_API_KEY=",
		} {
			if strings.HasPrefix(e, secret) {
				t.Errorf("%s should be stripped, got %q", secret, e)
			}
		}
	}

	hasPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("PATH should be preserved (it's on the allowlist)")
	}
}

func TestTrimCodeFences_Integration(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain text", "plain text"},
		{"```\nhello\n```", "hello"},
		{"```json\n{\"x\":1}\n```", "{\"x\":1}"},
		{"```", ""},
		{"", ""},
		{"   \n  ", ""},
	}
	for _, tt := range tests {
		got := trimCodeFences(tt.in)
		if got != tt.want {
			t.Errorf("trimCodeFences(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestContainsAny_Integration(t *testing.T) {
	if !containsAny("hello world", "world", "missing") {
		t.Error("expected match for 'world'")
	}
	if containsAny("hello", "missing") {
		t.Error("expected no match")
	}
}

func contains(s []string, target string) bool {
	for _, x := range s {
		if x == target {
			return true
		}
	}
	return false
}
