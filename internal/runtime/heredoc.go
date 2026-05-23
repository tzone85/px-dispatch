package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// randomHeredocDelimiter returns a one-shot heredoc delimiter — 16 random hex
// chars prefixed with PX_END_ — that is statistically impossible to appear in
// an LLM-generated goal. This blocks the heredoc-escape attack where a
// crafted prompt closes the heredoc early and lands subsequent lines as
// shell commands.
//
// If randomness fails (extraordinarily unlikely on a healthy host), we fall
// back to a fixed sentinel and still run sanitizeHeredocBody on the input as
// a second line of defence.
func randomHeredocDelimiter() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "PX_END_FALLBACK_DO_NOT_USE_IN_PROMPTS"
	}
	return "PX_END_" + hex.EncodeToString(b[:])
}

// sanitizeHeredocBody belt-and-braces in case the (astronomical) collision
// between the random delimiter and the goal body ever happens. If a line in
// the body is exactly the delimiter, replace it with a benign visible string
// — the body is a prompt, not source code, so swapping a single line is
// acceptable.
func sanitizeHeredocBody(body, delim string) string {
	if !strings.Contains(body, delim) {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == delim {
			lines[i] = "[heredoc delimiter collision sanitised by px]"
		}
	}
	return strings.Join(lines, "\n")
}
