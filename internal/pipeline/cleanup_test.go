package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestCleanupStage_Name(t *testing.T) {
	stage := NewCleanupStage(git.NewMockRunner())
	if stage.Name() != "cleanup" {
		t.Errorf("expected name 'cleanup', got %q", stage.Name())
	}
}

func TestCleanupStage_AllSucceed(t *testing.T) {
	mock := git.NewMockRunner()
	// git worktree remove --force
	mock.AddResponse("", nil)
	// git branch -D
	mock.AddResponse("", nil)
	// git push origin --delete
	mock.AddResponse("", nil)

	stage := NewCleanupStage(mock)
	sc := StoryContext{
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
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

	// Verify worktree remove
	wtCmd := mock.Commands[0]
	wtArgs := strings.Join(wtCmd.Args, " ")
	if !strings.Contains(wtArgs, "worktree remove") || !strings.Contains(wtArgs, "/tmp/wt") {
		t.Errorf("expected 'worktree remove /tmp/wt', got %s", wtArgs)
	}

	// Verify branch delete
	branchCmd := mock.Commands[1]
	branchArgs := strings.Join(branchCmd.Args, " ")
	if !strings.Contains(branchArgs, "branch -D") || !strings.Contains(branchArgs, "feat/story-1") {
		t.Errorf("expected 'branch -D feat/story-1', got %s", branchArgs)
	}

	// Verify remote delete
	remoteCmd := mock.Commands[2]
	remoteArgs := strings.Join(remoteCmd.Args, " ")
	if !strings.Contains(remoteArgs, "push origin --delete") || !strings.Contains(remoteArgs, "feat/story-1") {
		t.Errorf("expected 'push origin --delete feat/story-1', got %s", remoteArgs)
	}
}

func TestCleanupStage_WorktreeRemoveInRepoDir(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // worktree remove
	mock.AddResponse("", nil) // branch -D
	mock.AddResponse("", nil) // push --delete

	stage := NewCleanupStage(mock)
	sc := StoryContext{
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
	}

	_, _ = stage.Execute(context.Background(), sc)

	// worktree and branch ops should run in repo dir
	for i := 0; i < 2; i++ {
		if mock.Commands[i].Dir != "/tmp/repo" {
			t.Errorf("command %d should run in repo dir, got %s", i, mock.Commands[i].Dir)
		}
	}
	// remote delete also runs in repo dir
	if mock.Commands[2].Dir != "/tmp/repo" {
		t.Errorf("remote delete should run in repo dir, got %s", mock.Commands[2].Dir)
	}
}

func TestCleanupStage_ErrorsAreLogged_NotFatal(t *testing.T) {
	mock := git.NewMockRunner()
	// All cleanup commands fail
	mock.AddResponse("", errors.New("worktree not found"))
	mock.AddResponse("", errors.New("branch not found"))
	mock.AddResponse("", errors.New("remote branch not found"))

	stage := NewCleanupStage(mock)
	sc := StoryContext{
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("cleanup errors should not propagate: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed even on errors, got %s", result)
	}

	// All 3 commands should still have been attempted
	if len(mock.Commands) != 3 {
		t.Errorf("expected all 3 cleanup commands to be attempted, got %d", len(mock.Commands))
	}
}

func TestCleanupStage_PartialFailure(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                         // worktree remove succeeds
	mock.AddResponse("", errors.New("branch in use")) // branch delete fails
	mock.AddResponse("", nil)                         // remote delete succeeds

	stage := NewCleanupStage(mock)
	sc := StoryContext{
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("cleanup errors should not propagate: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed even with partial failure, got %s", result)
	}
}
