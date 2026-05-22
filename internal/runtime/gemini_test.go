package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/git"
)

func TestGeminiRuntime_Name(t *testing.T) {
	rt := NewGeminiRuntime()
	if rt.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "gemini")
	}
}

func TestGeminiRuntime_Capabilities(t *testing.T) {
	rt := NewGeminiRuntime()
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

func TestGeminiRuntime_Spawn(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))
	mock.AddResponse("", nil)

	rt := NewGeminiRuntime()
	cfg := SessionConfig{
		SessionName: "px-gemini-1",
		WorkDir:     "/tmp/work",
		Model:       "gemini-2.5-pro",
		Goal:        "implement feature Z",
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
	if !strings.Contains(lastArg, "gemini") {
		t.Errorf("expected command to contain 'gemini', got %q", lastArg)
	}
}

func TestGeminiRuntime_Kill(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	rt := NewGeminiRuntime()
	err := rt.Kill(mock, "px-gemini-1")
	if err != nil {
		t.Fatalf("kill: %v", err)
	}
}

func TestGeminiRuntime_ReadOutput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("gemini output", nil)

	rt := NewGeminiRuntime()
	out, err := rt.ReadOutput(mock, "px-gemini-1", 30)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "gemini output" {
		t.Errorf("expected 'gemini output', got %q", out)
	}
}

func TestGeminiRuntime_SendInput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	rt := NewGeminiRuntime()
	err := rt.SendInput(mock, "px-gemini-1", "y")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestGeminiRuntime_DetectStatus_Done(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))

	rt := NewGeminiRuntime()
	status, err := rt.DetectStatus(mock, "px-gemini-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusDone {
		t.Errorf("expected StatusDone, got %s", status)
	}
}

func TestGeminiRuntime_DetectStatus_Working(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)
	mock.AddResponse("Processing...", nil)

	rt := NewGeminiRuntime()
	status, err := rt.DetectStatus(mock, "px-gemini-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusWorking {
		t.Errorf("expected StatusWorking, got %s", status)
	}
}

func TestGeminiRuntime_DetectStatus_PermissionPrompt(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"approve action", "Approve action for file write"},
		{"allow y/n", "Allow? (y/n)"},
		{"confirm execution", "Confirm execution of command"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := git.NewMockRunner()
			mock.AddResponse("", nil)
			mock.AddResponse(tc.output, nil)

			rt := NewGeminiRuntime()
			status, err := rt.DetectStatus(mock, "px-gemini-1")
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if status != StatusPermissionPrompt {
				t.Errorf("expected StatusPermissionPrompt, got %s", status)
			}
		})
	}
}

func TestGeminiRuntime_DetectStatus_Idle(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)
	mock.AddResponse("done\n$", nil)

	rt := NewGeminiRuntime()
	status, err := rt.DetectStatus(mock, "px-gemini-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusIdle {
		t.Errorf("expected StatusIdle, got %s", status)
	}
}

func TestGeminiRuntime_DetectStatus_ReadOutputError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)
	mock.AddResponse("", fmt.Errorf("capture error"))

	rt := NewGeminiRuntime()
	status, err := rt.DetectStatus(mock, "px-gemini-1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if status != StatusWorking {
		t.Errorf("expected StatusWorking on read error, got %s", status)
	}
}

func TestGeminiRuntime_BuildCommand(t *testing.T) {
	rt := NewGeminiRuntime()

	tests := []struct {
		name      string
		cfg       SessionConfig
		wantParts []string
		noParts   []string
	}{
		{
			"with model",
			SessionConfig{Goal: "test", Model: "gemini-2.5-pro"},
			[]string{"gemini", "--model", "gemini-2.5-pro"},
			nil,
		},
		{
			"no model",
			SessionConfig{Goal: "do something"},
			[]string{"gemini"},
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
