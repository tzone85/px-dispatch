package runtime

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/tmux"
)

// Detection patterns for Codex CLI output.
var (
	codexPermissionRe = regexp.MustCompile(
		`(?i)(confirm\s+action|proceed\?\s*\[y/n\]|allow\s+this|do\s+you\s+trust\s+the\s+contents\s+of\s+this\s+directory|press\s+enter\s+to\s+continue)`,
	)
	codexIdleRe = regexp.MustCompile(
		`(?m)^\$\s*$`,
	)
)

// CodexRuntime implements the Runtime interface for the OpenAI Codex CLI.
type CodexRuntime struct {
	godmode bool
}

// NewCodexRuntime creates a CodexRuntime.
func NewCodexRuntime(godmode bool) *CodexRuntime {
	return &CodexRuntime{godmode: godmode}
}

// Name returns "codex".
func (c *CodexRuntime) Name() string {
	return "codex"
}

// Version detects the installed Codex CLI version.
func (c *CodexRuntime) Version(runner git.CommandRunner) (string, error) {
	out, err := runner.Run("", "codex", "--version")
	if err != nil {
		return "", fmt.Errorf("codex version: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// Health checks the health of a Codex session via tmux.
func (c *CodexRuntime) Health(runner git.CommandRunner, sessionName string) (tmux.HealthResult, error) {
	return tmux.SessionHealth(runner, sessionName, ""), nil
}

// Spawn starts a Codex session inside a new tmux session.
func (c *CodexRuntime) Spawn(runner git.CommandRunner, cfg SessionConfig) error {
	cmd := c.buildCommand(cfg)
	return tmux.CreateSession(runner, cfg.SessionName, cfg.WorkDir, cmd)
}

// Kill terminates the tmux session.
func (c *CodexRuntime) Kill(runner git.CommandRunner, sessionName string) error {
	return tmux.KillSession(runner, sessionName)
}

// DetectStatus reads pane output and classifies the agent state.
func (c *CodexRuntime) DetectStatus(runner git.CommandRunner, sessionName string) (AgentStatus, error) {
	if !tmux.SessionExists(runner, sessionName) {
		return StatusDone, nil
	}

	output, err := tmux.ReadOutput(runner, sessionName, 50)
	if err != nil {
		return StatusWorking, nil
	}

	return c.classifyOutput(output), nil
}

// ReadOutput returns the last N lines from the tmux pane.
func (c *CodexRuntime) ReadOutput(runner git.CommandRunner, sessionName string, lines int) (string, error) {
	return tmux.ReadOutput(runner, sessionName, lines)
}

// SendInput sends keystrokes to the tmux session.
func (c *CodexRuntime) SendInput(runner git.CommandRunner, sessionName string, input string) error {
	return tmux.SendKeys(runner, sessionName, input)
}

// Capabilities returns what Codex supports.
func (c *CodexRuntime) Capabilities() RuntimeCapabilities {
	return RuntimeCapabilities{
		SupportsModel: []string{
			"gpt-5.4",
			"gpt-5-codex",
			"gpt-5.2-codex",
			"o3",
			"o4-mini",
		},
		SupportsGodmode:    c.godmode,
		SupportsLogFile:    false,
		SupportsJsonOutput: false,
		MaxPromptLength:    0,
		CostTier:           CostTierAPI,
	}
}

// buildCommand constructs the codex CLI invocation string.
func (c *CodexRuntime) buildCommand(cfg SessionConfig) string {
	var parts []string
	parts = append(parts, "codex", "exec", "--color", "never", "--sandbox", "workspace-write")

	if c.godmode {
		parts = append(parts, "--dangerously-bypass-approvals-and-sandbox")
	} else {
		parts = append(parts, "--full-auto")
	}

	if cfg.Model != "" {
		parts = append(parts, "--model", shellQuote(cfg.Model))
	}

	parts = append(parts, "-")
	cmd := strings.Join(parts, " ")

	// `.px-done` is touched unconditionally after the CLI exits so the
	// pipeline poller can advance regardless of codex's exit code; pipeline
	// stages (diffcheck, qa) judge success based on the actual repo state.
	// Use `rc` not `status` — `status` is read-only in zsh.
	return "rm -f .px-done\n" +
		"cat <<'PX_EOF' | " + cmd + "\n" + cfg.Goal + "\nPX_EOF\n" +
		"rc=$?\n" +
		"printf '$\\n'\n" +
		"touch .px-done\n" +
		"sleep 30\n" +
		"exit $rc"
}

// classifyOutput matches Codex output against known patterns.
func (c *CodexRuntime) classifyOutput(output string) AgentStatus {
	if codexPermissionRe.MatchString(output) {
		return StatusPermissionPrompt
	}

	trimmed := strings.TrimRight(output, " \t\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 0 {
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if codexIdleRe.MatchString(lastLine) {
			return StatusIdle
		}
	}

	return StatusWorking
}
