package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/git"
)

func TestCodexRuntime_Name(t *testing.T) {
	rt := NewCodexRuntime(false)
	if rt.Name() != "codex" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "codex")
	}
}

func TestCodexRuntime_Capabilities(t *testing.T) {
	rt := NewCodexRuntime(false)
	caps := rt.Capabilities()

	if caps.SupportsGodmode {
		t.Error("expected SupportsGodmode=false")
	}
	if caps.SupportsLogFile {
		t.Error("expected SupportsLogFile=false")
	}
	if caps.SupportsJsonOutput {
		t.Error("expected SupportsJsonOutput=false")
	}
	if len(caps.SupportsModel) == 0 {
		t.Error("expected at least one supported model")
	}
}

func TestCodexRuntime_Spawn(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))
	mock.AddResponse("", nil)

	rt := NewCodexRuntime(false)
	cfg := SessionConfig{
		SessionName: "px-codex-1",
		WorkDir:     "/tmp/work",
		Model:       "o3",
		Goal:        "implement feature Y",
	}

	err := rt.Spawn(mock, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if len(mock.Commands) < 2 {
		t.Fatalf("expected at least 2 commands, got %d", len(mock.Commands))
	}

	newCmd := mock.Commands[1]
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if !strings.Contains(lastArg, "codex") {
		t.Errorf("expected command to contain 'codex', got %q", lastArg)
	}
	if !strings.Contains(lastArg, "--model") {
		t.Errorf("expected --model flag, got %q", lastArg)
	}
}

func TestCodexRuntime_SpawnNoModel(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))
	mock.AddResponse("", nil)

	rt := NewCodexRuntime(false)
	cfg := SessionConfig{
		SessionName: "px-codex-1",
		WorkDir:     "/tmp/work",
		Goal:        "implement feature",
	}

	err := rt.Spawn(mock, cfg)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	newCmd := mock.Commands[1]
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if strings.Contains(lastArg, "--model") {
		t.Errorf("expected no --model flag when model is empty, got %q", lastArg)
	}
}

func TestCodexRuntime_Kill(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	rt := NewCodexRuntime(false)
	err := rt.Kill(mock, "px-codex-1")
	if err != nil {
		t.Fatalf("kill: %v", err)
	}
}

func TestCodexRuntime_ReadOutput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("codex output here", nil)

	rt := NewCodexRuntime(false)
	out, err := rt.ReadOutput(mock, "px-codex-1", 30)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "codex output here" {
		t.Errorf("expected 'codex output here', got %q", out)
	}
}

func TestCodexRuntime_SendInput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	rt := NewCodexRuntime(false)
	err := rt.SendInput(mock, "px-codex-1", "y")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestCodexRuntime_DetectStatus_Done(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))

	rt := NewCodexRuntime(false)
	status, err := rt.DetectStatus(mock, "px-codex-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusDone {
		t.Errorf("expected StatusDone, got %s", status)
	}
}

func TestCodexRuntime_DetectStatus_Working(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)
	mock.AddResponse("Running codex...", nil)

	rt := NewCodexRuntime(false)
	status, err := rt.DetectStatus(mock, "px-codex-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusWorking {
		t.Errorf("expected StatusWorking, got %s", status)
	}
}

func TestCodexRuntime_DetectStatus_PermissionPrompt(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"confirm action", "Some output\nConfirm action"},
		{"proceed y/n", "proceed? [y/n]"},
		{"allow this", "Allow this file write"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := git.NewMockRunner()
			mock.AddResponse("", nil)
			mock.AddResponse(tc.output, nil)

			rt := NewCodexRuntime(false)
			status, err := rt.DetectStatus(mock, "px-codex-1")
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if status != StatusPermissionPrompt {
				t.Errorf("expected StatusPermissionPrompt, got %s", status)
			}
		})
	}
}

func TestCodexRuntime_DetectStatus_Idle(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)
	mock.AddResponse("some output\n$", nil)

	rt := NewCodexRuntime(false)
	status, err := rt.DetectStatus(mock, "px-codex-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusIdle {
		t.Errorf("expected StatusIdle, got %s", status)
	}
}

func TestCodexRuntime_DetectStatus_ReadOutputError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                         // has-session succeeds
	mock.AddResponse("", fmt.Errorf("capture error")) // read output fails

	rt := NewCodexRuntime(false)
	status, err := rt.DetectStatus(mock, "px-codex-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	// When output read fails, should default to Working.
	if status != StatusWorking {
		t.Errorf("expected StatusWorking on read error, got %s", status)
	}
}

func TestCodexRuntime_BuildCommand(t *testing.T) {
	rt := NewCodexRuntime(false)

	tests := []struct {
		name      string
		cfg       SessionConfig
		wantParts []string
		noParts   []string
	}{
		{
			"with model",
			SessionConfig{Goal: "test", Model: "o3"},
			[]string{"codex", "--model", "o3"},
			nil,
		},
		{
			"no model",
			SessionConfig{Goal: "test goal"},
			[]string{"codex"},
			[]string{"--model"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := rt.buildCommand(tt.cfg)
			for _, part := range tt.wantParts {
				if !strings.Contains(cmd, part) {
					t.Errorf("buildCommand() missing %q in %q", part, cmd)
				}
			}
			for _, part := range tt.noParts {
				if strings.Contains(cmd, part) {
					t.Errorf("buildCommand() should not contain %q in %q", part, cmd)
				}
			}
			if !strings.Contains(cmd, "touch .px-done") {
				t.Errorf("buildCommand() must touch the .px-done completion sentinel: %q", cmd)
			}
			if !strings.Contains(cmd, "rm -f .px-done") {
				t.Errorf("buildCommand() must clear stale sentinel before run: %q", cmd)
			}
			if strings.Contains(cmd, "status=$?") {
				t.Errorf("must not assign to zsh read-only `status`; use `rc=$?`. cmd=%q", cmd)
			}
		})
	}
}
