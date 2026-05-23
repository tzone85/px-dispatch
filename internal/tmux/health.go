package tmux

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
)

// HealthStatus represents the health state of a tmux session.
type HealthStatus string

const (
	// Healthy indicates the session is running and producing output.
	Healthy HealthStatus = "healthy"
	// Stale indicates the session is alive but output has not changed.
	Stale HealthStatus = "stale"
	// Dead indicates the session pane process has exited.
	Dead HealthStatus = "dead"
	// Missing indicates the session does not exist.
	Missing HealthStatus = "missing"
)

// HealthResult holds the health check result plus metadata.
type HealthResult struct {
	Status     HealthStatus
	OutputHash string // hash of current output, for staleness comparison
	PanePID    string
}

// SessionHealth checks the health of a tmux session.
// previousHash is the output hash from the last check (empty on first check).
// When previousHash matches the current output hash, the session is considered Stale.
func SessionHealth(runner git.CommandRunner, name, previousHash string) HealthResult {
	// 1. Check session exists
	if !SessionExists(runner, name) {
		return HealthResult{Status: Missing}
	}

	// 2. Check pane process alive
	out, err := runner.Run("", "tmux", "list-panes", "-t", name,
		"-F", "#{pane_pid} #{pane_dead} #{pane_dead_status}")
	if err != nil {
		return HealthResult{Status: Missing}
	}

	parts := strings.Fields(strings.TrimSpace(out))

	pid := ""
	if len(parts) >= 1 {
		pid = parts[0]
	}

	if len(parts) >= 2 && parts[1] == "1" {
		return HealthResult{Status: Dead, PanePID: pid}
	}

	// 3. Check output for staleness
	output, err := ReadOutput(runner, name, 30)
	if err != nil {
		return HealthResult{Status: Healthy, PanePID: pid}
	}

	hash := hashOutput(output)
	if previousHash != "" && hash == previousHash {
		return HealthResult{Status: Stale, OutputHash: hash, PanePID: pid}
	}

	return HealthResult{Status: Healthy, OutputHash: hash, PanePID: pid}
}

// hashOutput returns a hex-encoded SHA-256 hash of the given string.
func hashOutput(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}
