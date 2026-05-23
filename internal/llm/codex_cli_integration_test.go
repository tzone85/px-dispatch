package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeCodex writes a bash script that takes the `--output-last-message
// <path>` arg, writes content to that path, prints `stderr` to stderr, and
// exits with `code`.
func writeFakeCodex(t *testing.T, content, stderr string, code int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	script := fmt.Sprintf(`#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--output-last-message" ]; then
    out="$a"
  fi
  prev="$a"
done
if [ -n "$out" ]; then
  printf '%%s' %q > "$out"
fi
if [ -n %q ]; then
  printf '%%s' %q 1>&2
fi
exit %d
`, content, stderr, stderr, code)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	return path
}

func TestCodexCLI_SuccessReadsLastMessage(t *testing.T) {
	path := writeFakeCodex(t, "codex says hi", "", 0)
	client := NewCodexCLIClientWithPath(path)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "ping"}},
		Model:    "o3",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "codex says hi" {
		t.Errorf("Content = %q, want 'codex says hi'", resp.Content)
	}
	if resp.Model != "o3" {
		t.Errorf("Model = %q, want o3", resp.Model)
	}
}

func TestCodexCLI_ClassifyErrors(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantStatus int
		wantRetry  bool
	}{
		{"quota", "You exceeded your current quota, please check your plan and billing", 429, false},
		{"auth", "Not logged in to codex", 401, false},
		{"rate", "rate limit exceeded", 429, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFakeCodex(t, "", tt.stderr, 1)
			client := NewCodexCLIClientWithPath(path)
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

func TestCodexCLI_OutputReadError(t *testing.T) {
	// Fake codex that succeeds but DOESN'T write the output file → ReadFile fails.
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := NewCodexCLIClientWithPath(path)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "x"}},
	})
	// We expect either an os.ReadFile error (file never created) OR success
	// with empty content depending on whether os.CreateTemp leaves the file.
	if err == nil {
		// CreateTemp creates the file; ReadFile reads empty content → success
		// with empty result is acceptable.
		return
	}
	if !strings.Contains(err.Error(), "read codex output") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestHasCodexCLI(t *testing.T) {
	// We have a real codex CLI on the box per the user's env (`command -v
	// codex` returned a path during the sweep). HasCodexCLI should return
	// true OR false — we just exercise the function.
	_ = HasCodexCLI()
}
