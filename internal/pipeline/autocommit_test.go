package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestAutoCommitStage_Name(t *testing.T) {
	stage := NewAutoCommitStage(git.NewMockRunner())
	if stage.Name() != "autocommit" {
		t.Errorf("expected name 'autocommit', got %q", stage.Name())
	}
}

func TestAutoCommitStage_NoChanges(t *testing.T) {
	mock := git.NewMockRunner()
	// git status --porcelain returns empty (no changes)
	mock.AddResponse("", nil)

	stage := NewAutoCommitStage(mock)
	sc := StoryContext{
		StoryID:      "STORY-1",
		WorktreePath: "/tmp/wt",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}
	// Should only have called git status --porcelain
	if len(mock.Commands) != 1 {
		t.Errorf("expected 1 command (status), got %d", len(mock.Commands))
	}
}

func TestAutoCommitStage_WithChanges(t *testing.T) {
	mock := git.NewMockRunner()
	// git status --porcelain returns changed files
	mock.AddResponse("M  main.go\n?? new_file.go", nil)
	// git add -A
	mock.AddResponse("", nil)
	// git commit
	mock.AddResponse("", nil)

	stage := NewAutoCommitStage(mock)
	sc := StoryContext{
		StoryID:      "STORY-42",
		WorktreePath: "/tmp/wt",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	if len(mock.Commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(mock.Commands))
	}

	// Verify git add -A
	addCmd := mock.Commands[1]
	if addCmd.Name != "git" || strings.Join(addCmd.Args, " ") != "add -A" {
		t.Errorf("expected 'git add -A', got %s %s", addCmd.Name, strings.Join(addCmd.Args, " "))
	}

	// Verify commit message includes story ID
	commitCmd := mock.Commands[2]
	if commitCmd.Name != "git" {
		t.Errorf("expected git command, got %s", commitCmd.Name)
	}
	commitArgs := strings.Join(commitCmd.Args, " ")
	if !strings.Contains(commitArgs, "STORY-42") {
		t.Errorf("commit message should contain story ID, got %q", commitArgs)
	}

	// All commands should run in worktree dir
	for i, cmd := range mock.Commands {
		if cmd.Dir != "/tmp/wt" {
			t.Errorf("command %d: expected dir /tmp/wt, got %s", i, cmd.Dir)
		}
	}
}

func TestAutoCommitStage_StatusError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", errors.New("git status failed"))

	stage := NewAutoCommitStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestAutoCommitStage_AddError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("M file.go", nil)           // status shows changes
	mock.AddResponse("", errors.New("add error")) // git add fails

	stage := NewAutoCommitStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestAutoCommitStage_CommitError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("M file.go", nil)              // status
	mock.AddResponse("", nil)                        // add
	mock.AddResponse("", errors.New("commit error")) // commit fails

	stage := NewAutoCommitStage(mock)
	sc := StoryContext{WorktreePath: "/tmp/wt"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}
