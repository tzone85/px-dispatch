package tmux

import (
	"fmt"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
)

// Available reports whether the tmux binary is on PATH.
func Available(runner git.CommandRunner) bool {
	_, err := runner.Run("", "which", "tmux")
	return err == nil
}

// CreateSession starts a new detached tmux session.
// Kills any existing session with the same name first.
func CreateSession(runner git.CommandRunner, name, workDir, command string) error {
	if SessionExists(runner, name) {
		_ = KillSession(runner, name) // best effort
	}

	args := []string{"new-session", "-d", "-s", name, "-c", workDir}
	if command != "" {
		args = append(args, command)
	}

	_, err := runner.Run("", "tmux", args...)
	return err
}

// KillSession terminates a tmux session by name.
func KillSession(runner git.CommandRunner, name string) error {
	_, err := runner.Run("", "tmux", "kill-session", "-t", name)
	return err
}

// SessionExists checks whether a tmux session with the given name exists.
func SessionExists(runner git.CommandRunner, name string) bool {
	_, err := runner.Run("", "tmux", "has-session", "-t", name)
	return err == nil
}

// ListSessions returns the names of all active tmux sessions.
// Returns an empty slice (not an error) when no tmux server is running
// or when there are no sessions.
func ListSessions(runner git.CommandRunner) ([]string, error) {
	out, err := runner.Run("", "tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no server running") ||
			strings.Contains(errMsg, "no sessions") {
			return nil, nil
		}
		return nil, err
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}

	var sessions []string
	for _, line := range strings.Split(trimmed, "\n") {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// SendKeys sends keystrokes to a tmux session, followed by Enter.
func SendKeys(runner git.CommandRunner, name, keys string) error {
	_, err := runner.Run("", "tmux", "send-keys", "-t", name, keys, "Enter")
	return err
}

// ReadOutput captures the last N lines of output from a tmux session pane.
func ReadOutput(runner git.CommandRunner, name string, lines int) (string, error) {
	out, err := runner.Run("", "tmux", "capture-pane", "-t", name, "-p", "-S", fmt.Sprintf("-%d", lines))
	return out, err
}
