package runtime

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/tmux"
)

// Detection patterns for Gemini CLI output.
var (
	geminiPermissionRe = regexp.MustCompile(
		`(?i)(approve\s+action|allow\?\s*\(y/n\)|confirm\s+execution)`,
	)
	geminiIdleRe = regexp.MustCompile(
		`(?m)^\$\s*$`,
	)
)

// GeminiRuntime implements the Runtime interface for the Google Gemini CLI.
type GeminiRuntime struct{}

// NewGeminiRuntime creates a GeminiRuntime.
func NewGeminiRuntime() *GeminiRuntime {
	return &GeminiRuntime{}
}

// Name returns "gemini".
func (c *GeminiRuntime) Name() string {
	return "gemini"
}

// Version detects the installed Gemini CLI version.
func (c *GeminiRuntime) Version(runner git.CommandRunner) (string, error) {
	out, err := runner.Run("", "gemini", "--version")
	if err != nil {
		return "", fmt.Errorf("gemini version: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// Health checks the health of a Gemini session via tmux.
func (c *GeminiRuntime) Health(runner git.CommandRunner, sessionName string) (tmux.HealthResult, error) {
	return tmux.SessionHealth(runner, sessionName, ""), nil
}

// Spawn starts a Gemini session inside a new tmux session.
func (c *GeminiRuntime) Spawn(runner git.CommandRunner, cfg SessionConfig) error {
	cmd := c.buildCommand(cfg)
	return tmux.CreateSession(runner, cfg.SessionName, cfg.WorkDir, cmd)
}

// Kill terminates the tmux session.
func (c *GeminiRuntime) Kill(runner git.CommandRunner, sessionName string) error {
	return tmux.KillSession(runner, sessionName)
}

// DetectStatus reads pane output and classifies the agent state.
func (c *GeminiRuntime) DetectStatus(runner git.CommandRunner, sessionName string) (AgentStatus, error) {
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
func (c *GeminiRuntime) ReadOutput(runner git.CommandRunner, sessionName string, lines int) (string, error) {
	return tmux.ReadOutput(runner, sessionName, lines)
}

// SendInput sends keystrokes to the tmux session.
func (c *GeminiRuntime) SendInput(runner git.CommandRunner, sessionName string, input string) error {
	return tmux.SendKeys(runner, sessionName, input)
}

// Capabilities returns what Gemini supports.
func (c *GeminiRuntime) Capabilities() RuntimeCapabilities {
	return RuntimeCapabilities{
		SupportsModel: []string{
			"gemini-2.5-pro",
			"gemini-2.5-flash",
		},
		SupportsGodmode:    false,
		SupportsLogFile:    false,
		SupportsJsonOutput: false,
		MaxPromptLength:    0,
		CostTier:           CostTierAPI,
	}
}

// buildCommand constructs the gemini CLI invocation string.
func (c *GeminiRuntime) buildCommand(cfg SessionConfig) string {
	var parts []string
	parts = append(parts, "gemini")

	if cfg.Model != "" {
		parts = append(parts, "--model", cfg.Model)
	}

	parts = append(parts, shellQuote(cfg.Goal))

	cmd := strings.Join(parts, " ")
	// `.px-done` is touched unconditionally after the CLI exits so the
	// pipeline poller can advance regardless of gemini's exit code; pipeline
	// stages (diffcheck, qa) judge success based on the actual repo state.
	// Use `rc` not `status` — `status` is read-only in zsh.
	return "rm -f .px-done\n" +
		cmd + "\n" +
		"rc=$?\n" +
		"printf '$\\n'\n" +
		"touch .px-done\n" +
		"sleep 30\n" +
		"exit $rc"
}

// classifyOutput matches Gemini output against known patterns.
func (c *GeminiRuntime) classifyOutput(output string) AgentStatus {
	if geminiPermissionRe.MatchString(output) {
		return StatusPermissionPrompt
	}

	trimmed := strings.TrimRight(output, " \t\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 0 {
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if geminiIdleRe.MatchString(lastLine) {
			return StatusIdle
		}
	}

	return StatusWorking
}
