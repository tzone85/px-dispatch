package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ClaudeCLIClient implements Client by invoking the Claude Code CLI.
// Routes completions through the user's Claude subscription rather than API credits.
type ClaudeCLIClient struct {
	cliPath   string
	skipPerms bool
}

// NewClaudeCLIClient creates a client using the default "claude" binary on PATH.
func NewClaudeCLIClient() *ClaudeCLIClient {
	return &ClaudeCLIClient{cliPath: "claude", skipPerms: false}
}

// NewClaudeCLIClientWithPath creates a client using an explicit CLI binary path.
func NewClaudeCLIClientWithPath(cliPath string) *ClaudeCLIClient {
	return &ClaudeCLIClient{cliPath: cliPath, skipPerms: false}
}

// WithSkipPermissions returns a new copy with --dangerously-skip-permissions enabled.
// The original client is not modified (immutability).
func (c *ClaudeCLIClient) WithSkipPermissions() *ClaudeCLIClient {
	return &ClaudeCLIClient{cliPath: c.cliPath, skipPerms: true}
}

// Compile-time interface check.
var _ Client = (*ClaudeCLIClient)(nil)

// cliEnvelope is the JSON output format produced by "claude --output-format json".
type cliEnvelope struct {
	Result    string `json:"result"`
	IsError   bool   `json:"is_error"`
	SessionID string `json:"session_id"`
	Usage     struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Complete invokes the Claude Code CLI to perform a single-turn completion.
// It pipes the prompt via stdin, strips ANTHROPIC_API_KEY from the environment
// (forcing subscription usage), and parses the JSON envelope response.
func (c *ClaudeCLIClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	args := c.buildArgs(req)
	prompt := buildCLIPrompt(req)

	cmd := exec.CommandContext(ctx, c.cliPath, args...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = buildStrippedEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		combined := append(stdout.Bytes(), stderr.Bytes()...)
		return CompletionResponse{}, classifyCLIError(err, combined)
	}

	return parseOutput(stdout.Bytes(), req.Model)
}

// buildArgs constructs the CLI argument list.
func (c *ClaudeCLIClient) buildArgs(req CompletionRequest) []string {
	var args []string

	if c.skipPerms {
		args = append(args, "--dangerously-skip-permissions")
	}

	// -p - means read prompt from stdin
	args = append(args, "-p", "-")
	args = append(args, "--output-format", "json")
	args = append(args, "--max-turns", "1")

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	return args
}

// buildCLIPrompt concatenates the system prompt and user messages into a single
// text block for the CLI. Assistant messages are excluded because the CLI
// operates in single-turn mode.
func buildCLIPrompt(req CompletionRequest) string {
	var parts []string

	if req.System != "" {
		parts = append(parts, req.System)
	}

	for _, msg := range req.Messages {
		if msg.Role == RoleUser {
			parts = append(parts, msg.Content)
		}
	}

	return strings.Join(parts, "\n\n")
}

// envAllowlist is the set of environment variables that survive into the
// claude CLI subprocess. Everything else is stripped to prevent secrets
// (OPENAI_API_KEY, GH_TOKEN, GEMINI_API_KEY, etc.) from being inherited and
// then echoed into transcripts or surfaced through the dashboard logs API.
// PATH is required so claude can find git / npm / etc. when the agent runs
// shell tools. HOME / USER / SHELL / TMPDIR / TERM / LANG / LC_* support
// normal CLI behaviour. CI=true is preserved for CI environments that gate
// behaviour on it.
var envAllowlist = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
	"SHELL": true, "TMPDIR": true, "TERM": true,
	"LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"PWD": true, "OLDPWD": true,
	"CI": true,
	// Subscription auth — Claude CLI reads its own config from ~/.claude/,
	// not from env, so no API key needs to pass.
}

// buildStrippedEnv returns the environment to pass to the claude subprocess.
// It uses an allowlist (not a denylist) so any future secret added to the
// operator's shell environment cannot accidentally leak through.
// LC_* variables are preserved by prefix match because the locale set varies
// by host.
func buildStrippedEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		key := e
		if eq := strings.IndexByte(e, '='); eq >= 0 {
			key = e[:eq]
		}
		if envAllowlist[key] {
			filtered = append(filtered, e)
			continue
		}
		if strings.HasPrefix(key, "LC_") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// parseOutput attempts to parse the CLI JSON envelope. If JSON parsing fails,
// it falls back to using the raw output as the content.
func parseOutput(raw []byte, requestModel string) (CompletionResponse, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return CompletionResponse{}, fmt.Errorf("claude CLI returned empty output")
	}

	var envelope cliEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		// Fall back to raw output if not valid JSON.
		return CompletionResponse{
			Content: trimCodeFences(trimmed),
			Model:   requestModel,
		}, nil
	}

	if envelope.IsError {
		return CompletionResponse{}, &APIError{
			StatusCode: 500,
			Message:    envelope.Result,
			Retryable:  false,
		}
	}

	return CompletionResponse{
		Content:      trimCodeFences(envelope.Result),
		Model:        requestModel,
		InputTokens:  envelope.Usage.InputTokens,
		OutputTokens: envelope.Usage.OutputTokens,
	}, nil
}

// trimCodeFences strips markdown code fences from the string.
// Handles ```lang\n...\n``` and ```\n...\n``` formats.
func trimCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if !strings.HasPrefix(s, "```") {
		return s
	}

	// Remove opening fence line (```lang or ```)
	firstNewline := strings.Index(s, "\n")
	if firstNewline < 0 {
		// Only a fence marker with no content
		return ""
	}
	s = s[firstNewline+1:]

	// Remove closing fence if present
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}

	return strings.TrimSpace(s)
}

// classifyCLIError examines CLI output to produce a typed error when possible.
// Pattern-matches for billing, authentication, rate limit, and overload errors.
// Returns a generic wrapped error for unrecognized failures.
func classifyCLIError(originalErr error, output []byte) error {
	text := strings.ToLower(string(output))

	switch {
	case containsAny(text, "credit balance", "billing", "payment", "quota exceeded", "out of extra usage", "extra usage"):
		return &APIError{
			StatusCode: 400,
			Message:    strings.TrimSpace(string(output)),
			Retryable:  false,
		}

	case containsAny(text, "authentication", "unauthorized", "invalid api key", "permission denied"):
		return &APIError{
			StatusCode: 401,
			Message:    strings.TrimSpace(string(output)),
			Retryable:  false,
		}

	case containsAny(text, "rate limit", "too many requests", "throttl"):
		return &APIError{
			StatusCode: 429,
			Message:    strings.TrimSpace(string(output)),
			Retryable:  true,
		}

	case containsAny(text, "overloaded", "capacity", "503"):
		return &APIError{
			StatusCode: 529,
			Message:    strings.TrimSpace(string(output)),
			Retryable:  true,
		}
	}

	return fmt.Errorf("claude CLI failed: %w: %s", originalErr, strings.TrimSpace(string(output)))
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
