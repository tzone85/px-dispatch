package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestMergeStage_Name(t *testing.T) {
	stage := NewMergeStage(git.NewMockRunner(), false)
	if stage.Name() != "merge" {
		t.Errorf("expected name 'merge', got %q", stage.Name())
	}
}

func TestMergeStage_PushAndCreatePR(t *testing.T) {
	mock := git.NewMockRunner()
	// git push -u origin branch
	mock.AddResponse("", nil)
	// gh pr create
	mock.AddResponse("https://github.com/owner/repo/pull/42", nil)

	stage := NewMergeStage(mock, false)
	sc := StoryContext{
		StoryID:      "STORY-1",
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	if len(mock.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(mock.Commands))
	}

	// Verify push
	pushCmd := mock.Commands[0]
	pushArgs := strings.Join(pushCmd.Args, " ")
	if !strings.Contains(pushArgs, "push") || !strings.Contains(pushArgs, "feat/story-1") {
		t.Errorf("expected push command with branch, got %s", pushArgs)
	}

	// Verify PR create
	prCmd := mock.Commands[1]
	if prCmd.Name != "gh" {
		t.Errorf("expected gh command, got %s", prCmd.Name)
	}
	prArgs := strings.Join(prCmd.Args, " ")
	if !strings.Contains(prArgs, "pr create") {
		t.Errorf("expected 'pr create' in args, got %s", prArgs)
	}
	if !strings.Contains(prArgs, "STORY-1") {
		t.Errorf("expected story ID in PR title, got %s", prArgs)
	}
}

func TestMergeStage_AutoMerge(t *testing.T) {
	mock := git.NewMockRunner()
	// git push
	mock.AddResponse("", nil)
	// gh pr create
	mock.AddResponse("https://github.com/owner/repo/pull/42", nil)
	// gh pr merge --squash --auto
	mock.AddResponse("", nil)

	stage := NewMergeStage(mock, true)
	sc := StoryContext{
		StoryID:      "STORY-1",
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	if len(mock.Commands) != 3 {
		t.Fatalf("expected 3 commands (push, pr create, pr merge), got %d", len(mock.Commands))
	}

	// Verify merge command
	mergeCmd := mock.Commands[2]
	mergeArgs := strings.Join(mergeCmd.Args, " ")
	if !strings.Contains(mergeArgs, "pr merge") {
		t.Errorf("expected 'pr merge' command, got %s", mergeArgs)
	}
	if !strings.Contains(mergeArgs, "--squash") || !strings.Contains(mergeArgs, "--auto") {
		t.Errorf("expected --squash --auto flags, got %s", mergeArgs)
	}
}

func TestMergeStage_PushError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", errors.New("push rejected"))

	stage := NewMergeStage(mock, false)
	sc := StoryContext{
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestMergeStage_PRCreateError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                              // push succeeds
	mock.AddResponse("", errors.New("PR creation failed")) // pr create fails

	stage := NewMergeStage(mock, false)
	sc := StoryContext{
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestMergeStage_MergeError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                                           // push
	mock.AddResponse("https://github.com/owner/repo/pull/42", nil)     // pr create
	mock.AddResponse("", errors.New("merge conflict or checks failed")) // pr merge fails

	stage := NewMergeStage(mock, true)
	sc := StoryContext{
		StoryID:      "STORY-1",
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestMergeStage_CommandsRunInWorktreeDir(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                                       // push
	mock.AddResponse("https://github.com/owner/repo/pull/1", nil)  // pr create

	stage := NewMergeStage(mock, false)
	sc := StoryContext{
		StoryID:      "STORY-1",
		Branch:       "feat/story-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	_, _ = stage.Execute(context.Background(), sc)

	for i, cmd := range mock.Commands {
		if cmd.Dir != "/tmp/wt" {
			t.Errorf("command %d: expected dir /tmp/wt, got %s", i, cmd.Dir)
		}
	}
}
