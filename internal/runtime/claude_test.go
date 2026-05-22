package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/git"
)

func TestClaudeCodeRuntime_Name(t *testing.T) {
	rt := NewClaudeCodeRuntime(false)
	if rt.Name() != "claude-code" {
		t.Errorf("expected 'claude-code', got %s", rt.Name())
	}
}

func TestClaudeCodeRuntime_Capabilities(t *testing.T) {
	rt := NewClaudeCodeRuntime(true)
	caps := rt.Capabilities()

	if !caps.SupportsGodmode {
		t.Error("expected SupportsGodmode=true")
	}
	if !caps.SupportsJsonOutput {
		t.Error("expected SupportsJsonOutput=true")
	}
	if !caps.SupportsLogFile {
		t.Error("expected SupportsLogFile=true (transcript captured via tee)")
	}
	if caps.MaxPromptLength != 0 {
		t.Errorf("expected MaxPromptLength=0 (unlimited), got %d", caps.MaxPromptLength)
	}
	if len(caps.SupportsModel) == 0 {
		t.Error("expected at least one supported model")
	}
}

func TestClaudeCodeRuntime_CapabilitiesNoGodmode(t *testing.T) {
	rt := NewClaudeCodeRuntime(false)
	caps := rt.Capabilities()
	if caps.SupportsGodmode {
		t.Error("expected SupportsGodmode=false when godmode disabled")
	}
}

func TestClaudeCodeRuntime_Spawn(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails
	mock.AddResponse("", nil)                      // new-session succeeds

	rt := NewClaudeCodeRuntime(false)
	cfg := SessionConfig{
		SessionName: "px-story-1",
		WorkDir:     "/tmp/work",
		Model:       "opus",
		Goal:        "implement feature X",
	}

	err := rt.Spawn(mock, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Verify the tmux new-session command was called.
	if len(mock.Commands) < 2 {
		t.Fatalf("expected at least 2 commands, got %d", len(mock.Commands))
	}

	newCmd := mock.Commands[1]
	if newCmd.Name != "tmux" {
		t.Errorf("expected tmux command, got %s", newCmd.Name)
	}

	// The last arg should be the shell command string containing 'claude'.
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if !strings.Contains(lastArg, "claude") {
		t.Errorf("expected command to contain 'claude', got %q", lastArg)
	}
	if !strings.Contains(lastArg, "-p") {
		t.Errorf("expected command to contain '-p' flag, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "--model") {
		t.Errorf("expected command to contain '--model' flag, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "printf '$") {
		t.Errorf("expected completion marker in command, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "touch .px-done") {
		t.Errorf("expected .px-done sentinel touch, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "rm -f .px-done") {
		t.Errorf("expected stale sentinel cleanup, got %q", lastArg)
	}
	if strings.Contains(lastArg, "status=$?") {
		t.Errorf("must not assign to zsh read-only `status`; use `rc=$?`. cmd=%q", lastArg)
	}
}

func TestClaudeCodeRuntime_SpawnWithGodmode(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session
	mock.AddResponse("", nil)                      // new-session

	rt := NewClaudeCodeRuntime(true)
	cfg := SessionConfig{
		SessionName: "px-story-1",
		WorkDir:     "/tmp/work",
		Goal:        "implement feature X",
	}

	err := rt.Spawn(mock, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	newCmd := mock.Commands[1]
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if !strings.Contains(lastArg, "--dangerously-skip-permissions") {
		t.Errorf("expected godmode flag, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "printf '$") {
		t.Errorf("expected completion marker in command, got %q", lastArg)
	}
}

func TestClaudeCodeRuntime_SpawnWithSystemPrompt(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))
	mock.AddResponse("", nil)

	rt := NewClaudeCodeRuntime(false)
	cfg := SessionConfig{
		SessionName:  "px-story-1",
		WorkDir:      "/tmp/work",
		Goal:         "implement feature X",
		SystemPrompt: "You are a helpful assistant.",
	}

	err := rt.Spawn(mock, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	newCmd := mock.Commands[1]
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if !strings.Contains(lastArg, "--system-prompt") {
		t.Errorf("expected --system-prompt flag, got %q", lastArg)
	}
}

func TestClaudeCodeRuntime_SpawnWithLogFile(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))
	mock.AddResponse("", nil)

	rt := NewClaudeCodeRuntime(false)
	cfg := SessionConfig{
		SessionName: "px-story-1",
		WorkDir:     "/tmp/work",
		Goal:        "implement feature X",
		LogFile:     "/tmp/work/PX_AGENT_TRANSCRIPT.log",
	}

	err := rt.Spawn(mock, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	newCmd := mock.Commands[1]
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if strings.Contains(lastArg, "--output-file") {
		t.Errorf("--output-file is not a real claude flag; expected tee redirect instead, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "tee") {
		t.Errorf("expected tee redirect for transcript capture, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "PX_AGENT_TRANSCRIPT.log") {
		t.Errorf("expected transcript log path in command, got %q", lastArg)
	}
}

func TestClaudeCodeRuntime_Kill(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // kill-session succeeds

	rt := NewClaudeCodeRuntime(false)
	err := rt.Kill(mock, "px-story-1")
	if err != nil {
		t.Fatalf("kill: %v", err)
	}

	if len(mock.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.Commands))
	}
	cmd := mock.Commands[0]
	if cmd.Name != "tmux" {
		t.Errorf("expected tmux, got %s", cmd.Name)
	}
}

func TestClaudeCodeRuntime_ReadOutput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("some agent output", nil)

	rt := NewClaudeCodeRuntime(false)
	out, err := rt.ReadOutput(mock, "px-story-1", 30)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "some agent output" {
		t.Errorf("expected 'some agent output', got %q", out)
	}
}

func TestClaudeCodeRuntime_SendInput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	rt := NewClaudeCodeRuntime(false)
	err := rt.SendInput(mock, "px-story-1", "Y")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	cmd := mock.Commands[0]
	if cmd.Name != "tmux" {
		t.Errorf("expected tmux, got %s", cmd.Name)
	}
}

func TestClaudeCodeRuntime_DetectStatus_Working(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // has-session succeeds
	mock.AddResponse("Processing files...\nRunning tests...", nil)

	rt := NewClaudeCodeRuntime(false)
	status, err := rt.DetectStatus(mock, "px-story-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusWorking {
		t.Errorf("expected StatusWorking, got %s", status)
	}
}

func TestClaudeCodeRuntime_DetectStatus_PermissionPrompt(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"allow_pattern", "Some output\n  Allow this action? (y/n)"},
		{"yes_no_pattern", "Do you want to proceed?\n  Yes / No"},
		{"approve_pattern", "Please approve this change"},
		{"do_you_want", "Do you want to allow tool"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := git.NewMockRunner()
			mock.AddResponse("", nil) // has-session
			mock.AddResponse(tc.output, nil)

			rt := NewClaudeCodeRuntime(false)
			status, err := rt.DetectStatus(mock, "px-story-1")
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if status != StatusPermissionPrompt {
				t.Errorf("expected StatusPermissionPrompt for %q, got %s", tc.name, status)
			}
		})
	}
}

func TestClaudeCodeRuntime_DetectStatus_Done(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails

	rt := NewClaudeCodeRuntime(false)
	status, err := rt.DetectStatus(mock, "px-story-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusDone {
		t.Errorf("expected StatusDone, got %s", status)
	}
}

func TestClaudeCodeRuntime_DetectStatus_Idle(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // has-session
	mock.AddResponse("some output\n$", nil)

	rt := NewClaudeCodeRuntime(false)
	status, err := rt.DetectStatus(mock, "px-story-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusIdle {
		t.Errorf("expected StatusIdle, got %s", status)
	}
}

func TestClaudeCodeRuntime_DetectStatus_PlanMode(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // has-session
	mock.AddResponse("Entering plan mode\nPlan: step 1, step 2", nil)

	rt := NewClaudeCodeRuntime(false)
	status, err := rt.DetectStatus(mock, "px-story-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusPlanMode {
		t.Errorf("expected StatusPlanMode, got %s", status)
	}
}

func TestClaudeCodeRuntime_Version(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("claude-code 1.2.3\n", nil)

	rt := NewClaudeCodeRuntime(false)
	ver, err := rt.Version(mock)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if ver != "claude-code 1.2.3" {
		t.Errorf("expected 'claude-code 1.2.3', got %q", ver)
	}

	cmd := mock.Commands[0]
	if cmd.Name != "claude" {
		t.Errorf("expected 'claude' command, got %s", cmd.Name)
	}
	if len(cmd.Args) != 1 || cmd.Args[0] != "--version" {
		t.Errorf("expected ['--version'] args, got %v", cmd.Args)
	}
}

func TestClaudeCodeRuntime_VersionError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("command not found"))

	rt := NewClaudeCodeRuntime(false)
	_, err := rt.Version(mock)
	if err == nil {
		t.Error("expected error when claude CLI not found")
	}
}

func TestClaudeCodeRuntime_Health_Healthy(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                     // has-session
	mock.AddResponse("12345 0 0", nil)             // list-panes
	mock.AddResponse("some output being done", nil) // capture-pane

	rt := NewClaudeCodeRuntime(false)
	result, err := rt.Health(mock, "px-story-1")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if result.Status != "healthy" {
		t.Errorf("expected 'healthy', got %q", result.Status)
	}
}

func TestClaudeCodeRuntime_Health_Missing(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails

	rt := NewClaudeCodeRuntime(false)
	result, err := rt.Health(mock, "px-story-1")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if result.Status != "missing" {
		t.Errorf("expected 'missing', got %q", result.Status)
	}
}

func TestClaudeCodeRuntime_CostTier(t *testing.T) {
	rt := NewClaudeCodeRuntime(false)
	caps := rt.Capabilities()
	if caps.CostTier != CostTierSubscription {
		t.Errorf("expected CostTierSubscription, got %d", caps.CostTier)
	}
}
