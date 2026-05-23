package monitor

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/runtime"
	"github.com/tzone85/px-dispatch/internal/state"
)

// outputClassificationLines is the number of output lines to read for status classification.
const outputClassificationLines = 50

// fingerprintLines is the number of output lines to read for stuck-detection fingerprinting.
const fingerprintLines = 30

// Classification patterns for detecting agent states from pane output.
var (
	watchdogPermissionRe = regexp.MustCompile(
		`(?i)(Allow\s+.*\?\s*\(y/n\)|Yes\s*/\s*No|approve\s+this|Do you want to allow|Do you trust the contents of this directory|Press enter to continue)`,
	)
	watchdogPlanModeRe = regexp.MustCompile(
		`(?i)(plan\s*mode|Plan:\s+)`,
	)
	watchdogIdleRe = regexp.MustCompile(
		`(?m)^\$\s*$`,
	)
	watchdogCodexTrustRe = regexp.MustCompile(
		`(?i)(Do you trust the contents of this directory|Press enter to continue)`,
	)
)

// fingerprint records the hash and timestamp of a pane capture for stuck detection.
type fingerprint struct {
	Hash      string
	Timestamp time.Time
}

// WatchdogConfig holds the configuration for a Watchdog instance.
type WatchdogConfig struct {
	// StuckThresholdS is the number of seconds of unchanged output before
	// an agent is considered stuck. Zero means flag immediately on second
	// consecutive identical fingerprint.
	StuckThresholdS int
}

// CheckResult captures the outcome of a single watchdog check.
type CheckResult struct {
	SessionName string
	Status      runtime.AgentStatus
	// Action is one of: "none", "permission_bypass", "plan_escape", "stuck_detected".
	Action string
}

// Watchdog monitors agent sessions for stuck states, permission prompts,
// and plan mode — taking corrective action automatically.
type Watchdog struct {
	config       WatchdogConfig
	eventStore   state.EventStore
	fingerprints map[string]fingerprint
}

// NewWatchdog creates a Watchdog with the given config and event store.
func NewWatchdog(cfg WatchdogConfig, es state.EventStore) *Watchdog {
	return &Watchdog{
		config:       cfg,
		eventStore:   es,
		fingerprints: make(map[string]fingerprint),
	}
}

// Check reads the current pane output for sessionName, classifies the agent
// state, and takes corrective action when necessary. It returns a CheckResult
// describing the action taken (if any).
//
// The method calls rt.ReadOutput directly (rather than rt.DetectStatus) so
// that a single tmux capture-pane invocation is used for status classification.
// For working sessions the pane is captured a second time to produce a stable
// fingerprint for stuck detection.
func (w *Watchdog) Check(runner git.CommandRunner, sessionName string, rt runtime.Runtime) CheckResult {
	result := CheckResult{SessionName: sessionName, Action: "none"}

	// Read pane output for status classification.
	output, err := rt.ReadOutput(runner, sessionName, outputClassificationLines)
	if err != nil {
		result.Status = runtime.StatusWorking
		return result
	}

	status := w.classifyOutput(output)
	result.Status = status

	switch status {
	case runtime.StatusPermissionPrompt:
		input := "Y"
		if watchdogCodexTrustRe.MatchString(output) {
			input = "1"
		}
		_ = rt.SendInput(runner, sessionName, input)
		result.Action = "permission_bypass"

	case runtime.StatusPlanMode:
		_ = rt.SendInput(runner, sessionName, "Escape")
		result.Action = "plan_escape"

	case runtime.StatusDone, runtime.StatusIdle, runtime.StatusTerminated:
		// Session is idle or finished — no corrective action needed.
		return result

	case runtime.StatusWorking:
		// Capture a second snapshot for fingerprinting.
		fp, err := rt.ReadOutput(runner, sessionName, fingerprintLines)
		if err != nil {
			return result
		}

		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fp)))
		now := time.Now()

		prev, exists := w.fingerprints[sessionName]
		// Always update the stored fingerprint with the latest snapshot.
		w.fingerprints[sessionName] = fingerprint{Hash: hash, Timestamp: now}

		if exists && prev.Hash == hash {
			elapsed := now.Sub(prev.Timestamp)
			if elapsed.Seconds() >= float64(w.config.StuckThresholdS) {
				result.Action = "stuck_detected"
				_ = w.eventStore.Append(state.NewEvent(
					state.EventAgentStuck, "", "", map[string]any{
						"session_name": sessionName,
						"stuck_for_s":  int(elapsed.Seconds()),
					},
				))
			}
		}
	}

	return result
}

// ClearFingerprint removes the stored fingerprint for a session, e.g. when
// the poller restarts an agent after a stuck event.
func (w *Watchdog) ClearFingerprint(sessionName string) {
	delete(w.fingerprints, sessionName)
}

// classifyOutput maps pane output to an AgentStatus using pattern matching.
// It mirrors the classification logic in ClaudeCodeRuntime so that the
// watchdog can work without an extra tmux has-session call.
func (w *Watchdog) classifyOutput(output string) runtime.AgentStatus {
	if watchdogPermissionRe.MatchString(output) {
		return runtime.StatusPermissionPrompt
	}
	if watchdogPlanModeRe.MatchString(output) {
		return runtime.StatusPlanMode
	}

	// A bare shell prompt at the end of the pane means the agent has exited.
	trimmed := strings.TrimRight(output, " \t\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 0 {
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if watchdogIdleRe.MatchString(lastLine) {
			return runtime.StatusIdle
		}
	}

	return runtime.StatusWorking
}
