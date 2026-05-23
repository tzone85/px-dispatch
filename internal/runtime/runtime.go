// Package runtime defines the plugin interface for AI coding CLI runtimes
// and provides built-in implementations for Claude Code, Codex, and Gemini.
package runtime

import (
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/tmux"
)

// AgentStatus represents the detected state of an agent session.
type AgentStatus int

const (
	StatusWorking AgentStatus = iota
	StatusDone
	StatusTerminated
	StatusPermissionPrompt
	StatusPlanMode
	StatusStuck
	StatusIdle
)

// String returns the human-readable name of an AgentStatus.
func (s AgentStatus) String() string {
	switch s {
	case StatusWorking:
		return "working"
	case StatusDone:
		return "done"
	case StatusTerminated:
		return "terminated"
	case StatusPermissionPrompt:
		return "permission_prompt"
	case StatusPlanMode:
		return "plan_mode"
	case StatusStuck:
		return "stuck"
	case StatusIdle:
		return "idle"
	default:
		return "unknown"
	}
}

// SessionConfig holds the parameters for spawning an agent session.
type SessionConfig struct {
	SessionName  string
	WorkDir      string
	Model        string
	Goal         string
	SystemPrompt string
	LogFile      string
}

// CostTier classifies how a runtime is billed.
type CostTier int

const (
	// CostTierSubscription indicates a flat-rate or subscription-based runtime.
	CostTierSubscription CostTier = iota
	// CostTierAPI indicates a per-token/per-request metered runtime.
	CostTierAPI
)

// RuntimeCapabilities describes what a runtime supports.
type RuntimeCapabilities struct {
	SupportsModel      []string
	SupportsGodmode    bool
	SupportsLogFile    bool
	SupportsJsonOutput bool
	MaxPromptLength    int      // 0 = unlimited
	CostTier           CostTier // subscription vs API-metered billing
}

// Runtime is the interface for AI coding CLI runtimes.
// Each implementation encapsulates the CLI arguments, output patterns,
// and detection logic for a specific agent (e.g. Claude Code, Codex, Gemini).
type Runtime interface {
	// Name returns the unique identifier for this runtime.
	Name() string

	// Version detects and returns the CLI tool version string.
	Version(runner git.CommandRunner) (string, error)

	// Spawn starts a new agent session inside a tmux session.
	Spawn(runner git.CommandRunner, cfg SessionConfig) error

	// Kill terminates an agent session by its tmux session name.
	Kill(runner git.CommandRunner, sessionName string) error

	// DetectStatus reads the tmux pane output and classifies the agent state.
	DetectStatus(runner git.CommandRunner, sessionName string) (AgentStatus, error)

	// ReadOutput returns the last N lines of output from the agent session.
	ReadOutput(runner git.CommandRunner, sessionName string, lines int) (string, error)

	// Health checks the health of a running session via tmux.
	Health(runner git.CommandRunner, sessionName string) (tmux.HealthResult, error)

	// SendInput sends keystrokes to the agent session (e.g. to approve a prompt).
	SendInput(runner git.CommandRunner, sessionName string, input string) error

	// Capabilities returns what this runtime supports.
	Capabilities() RuntimeCapabilities
}
