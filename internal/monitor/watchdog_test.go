package monitor

import (
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/runtime"
)

func TestWatchdog_PermissionBypass(t *testing.T) {
	mockRunner := git.NewMockRunner()
	// ReadOutput returns permission prompt text
	mockRunner.AddResponse("Do you want to allow this? (Y/n)", nil) // capture-pane
	mockRunner.AddResponse("", nil)                                 // send-keys "Y"

	rt := runtime.NewClaudeCodeRuntime(false)
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{StuckThresholdS: 120}, es)

	result := wd.Check(mockRunner, "test-session", rt)
	if result.Action != "permission_bypass" {
		t.Errorf("expected permission_bypass, got %s", result.Action)
	}
}

func TestWatchdog_CodexTrustPromptBypass(t *testing.T) {
	mockRunner := git.NewMockRunner()
	mockRunner.AddResponse("Do you trust the contents of this directory?\nPress enter to continue", nil)
	mockRunner.AddResponse("", nil) // send-keys "1"

	rt := runtime.NewCodexRuntime(false)
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{StuckThresholdS: 120}, es)

	result := wd.Check(mockRunner, "test-session", rt)
	if result.Action != "permission_bypass" {
		t.Fatalf("expected permission_bypass, got %s", result.Action)
	}

	if got := mockRunner.Commands[1].Args[3]; got != "1" {
		t.Fatalf("expected watchdog to answer Codex trust prompt with 1, got %q", got)
	}
}

func TestWatchdog_PlanModeEscape(t *testing.T) {
	mockRunner := git.NewMockRunner()
	mockRunner.AddResponse("Plan mode: reviewing changes...", nil) // capture-pane
	mockRunner.AddResponse("", nil)                                // send-keys Escape

	rt := runtime.NewClaudeCodeRuntime(false)
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{StuckThresholdS: 120}, es)

	result := wd.Check(mockRunner, "test-session", rt)
	if result.Action != "plan_escape" {
		t.Errorf("expected plan_escape, got %s", result.Action)
	}
}

func TestWatchdog_StuckDetection(t *testing.T) {
	mockRunner1 := git.NewMockRunner()
	mockRunner1.AddResponse("some output", nil) // first check: capture-pane for status
	mockRunner1.AddResponse("some output", nil) // fingerprint capture

	rt := runtime.NewClaudeCodeRuntime(false)
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{StuckThresholdS: 0}, es) // 0 = detect stuck immediately

	// First check — establishes fingerprint
	wd.Check(mockRunner1, "test-session", rt)

	mockRunner2 := git.NewMockRunner()
	mockRunner2.AddResponse("some output", nil) // same output
	mockRunner2.AddResponse("some output", nil) // same fingerprint

	// Second check with same output — should detect stuck
	result := wd.Check(mockRunner2, "test-session", rt)
	if result.Action != "stuck_detected" {
		t.Errorf("expected stuck_detected, got %s", result.Action)
	}
}

func TestWatchdog_WorkingNoAction(t *testing.T) {
	mockRunner := git.NewMockRunner()
	mockRunner.AddResponse("actively processing stuff...", nil) // capture-pane
	mockRunner.AddResponse("actively processing stuff...", nil) // fingerprint

	rt := runtime.NewClaudeCodeRuntime(false)
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{StuckThresholdS: 9999}, es)

	result := wd.Check(mockRunner, "test-session", rt)
	if result.Action != "none" {
		t.Errorf("expected none, got %s", result.Action)
	}
}

func TestWatchdog_ClearFingerprint(t *testing.T) {
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{}, es)
	wd.fingerprints["test"] = fingerprint{Hash: "abc"}

	wd.ClearFingerprint("test")

	if _, exists := wd.fingerprints["test"]; exists {
		t.Error("fingerprint should have been cleared")
	}
}

func TestWatchdog_DoneNoAction(t *testing.T) {
	// When agent is done, no action needed
	mockRunner := git.NewMockRunner()
	mockRunner.AddResponse("$ ", nil) // idle prompt = done

	rt := runtime.NewClaudeCodeRuntime(false)
	es := &mockEventStore{}
	wd := NewWatchdog(WatchdogConfig{}, es)

	result := wd.Check(mockRunner, "test-session", rt)
	if result.Action != "none" {
		t.Errorf("expected none for done session, got %s", result.Action)
	}
}
