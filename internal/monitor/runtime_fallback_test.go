package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tzone85/px-dispatch/internal/agent"
	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/modelswitch"
	"github.com/tzone85/px-dispatch/internal/planner"
	"github.com/tzone85/px-dispatch/internal/runtime"
	"github.com/tzone85/px-dispatch/internal/state"
)

type fixedApprover struct {
	approved bool
}

func (a fixedApprover) ApproveSwitch(_ modelswitch.Request) (bool, error) {
	return a.approved, nil
}

func TestRuntimeFallbackManager_SwitchesToConfiguredRuntime(t *testing.T) {
	worktree := t.TempDir()
	transcriptPath := filepath.Join(worktree, transcriptFileName)
	if err := os.WriteFile(transcriptPath, []byte("planning\nediting\nlimit reached"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	runner := git.NewMockRunner()
	runner.AddResponse("feature/fallback", nil)      // git rev-parse --abbrev-ref HEAD
	runner.AddResponse("M README.md", nil)           // git status --short
	runner.AddResponse(" README.md | 2 +-", nil)     // git diff --stat
	runner.AddResponse("README.md", nil)             // git diff --name-only
	runner.AddResponse("", fmt.Errorf("no session")) // tmux has-session
	runner.AddResponse("", nil)                      // tmux new-session

	reg := runtime.NewRegistry()
	reg.Register("codex", runtime.NewCodexRuntime(false))

	es := &mockEventStore{}
	ps := &mockProjector{}

	manager := NewRuntimeFallbackManager(
		runner,
		reg,
		config.FallbackConfig{
			Enabled:            true,
			RequireApproval:    true,
			Runtime:            "codex",
			RuntimeModel:       "gpt-5.4",
			HandoffOutputLines: 40,
		},
		es,
		ps,
		fixedApprover{approved: true},
	)

	agentState := ActiveAgent{
		Assignment: Assignment{
			ReqID:       "req-1",
			StoryID:     "s-1",
			Role:        agent.RoleSenior,
			SessionName: "px-s-1",
		},
		WorktreePath:   worktree,
		RuntimeName:    "claude-code",
		Model:          "claude-sonnet-4-20250514",
		TranscriptPath: transcriptPath,
		Story: planner.PlannedStory{
			ID:                 "s-1",
			Title:              "Add fallback",
			Description:        "Implement model switch",
			AcceptanceCriteria: "Fallback works",
			Complexity:         5,
		},
	}

	updated, switched, err := manager.TrySwitch(context.Background(), agentState, "Error: usage limit reached\n$")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !switched {
		t.Fatal("expected runtime switch")
	}
	if updated.RuntimeName != "codex" {
		t.Fatalf("expected codex runtime, got %s", updated.RuntimeName)
	}

	handoffPath := filepath.Join(worktree, handoffFileName)
	if _, err := os.Stat(handoffPath); err != nil {
		t.Fatalf("expected handoff file to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, handoffJSONFileName)); err != nil {
		t.Fatalf("expected handoff json to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, transcriptSnapshotFileName)); err != nil {
		t.Fatalf("expected transcript snapshot to be written: %v", err)
	}

	foundProgress := false
	for _, evt := range es.events {
		if evt.Type == state.EventStoryProgress {
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Fatal("expected a story progress event for the runtime switch")
	}
}

func TestRuntimeFallbackManager_DeclinePausesRequirement(t *testing.T) {
	reg := runtime.NewRegistry()
	reg.Register("codex", runtime.NewCodexRuntime(false))

	es := &mockEventStore{}
	ps := &mockProjector{}

	manager := NewRuntimeFallbackManager(
		git.NewMockRunner(),
		reg,
		config.FallbackConfig{
			Enabled:            true,
			RequireApproval:    true,
			Runtime:            "codex",
			RuntimeModel:       "gpt-5.4",
			HandoffOutputLines: 40,
		},
		es,
		ps,
		fixedApprover{approved: false},
	)

	agentState := ActiveAgent{
		Assignment: Assignment{
			ReqID:       "req-1",
			StoryID:     "s-1",
			Role:        agent.RoleSenior,
			SessionName: "px-s-1",
		},
		RuntimeName: "claude-code",
		Story: planner.PlannedStory{
			ID:    "s-1",
			Title: "Add fallback",
		},
	}

	_, switched, err := manager.TrySwitch(context.Background(), agentState, "Error: credit balance exhausted\n$")
	if err == nil {
		t.Fatal("expected error when switch is declined")
	}
	if switched {
		t.Fatal("should not switch runtimes when declined")
	}

	foundPaused := false
	for _, evt := range es.events {
		if evt.Type == state.EventReqPaused {
			foundPaused = true
			break
		}
	}
	if !foundPaused {
		t.Fatal("expected requirement pause event after decline")
	}
}
