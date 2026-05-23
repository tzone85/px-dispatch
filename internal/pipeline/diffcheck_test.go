package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func TestDiffCheckStage_Name(t *testing.T) {
	stage := NewDiffCheckStage(git.NewMockRunner())
	if stage.Name() != "diffcheck" {
		t.Errorf("expected name 'diffcheck', got %q", stage.Name())
	}
}

func TestDiffCheckStage_RealChanges(t *testing.T) {
	mock := git.NewMockRunner()
	// git merge-base HEAD origin/main
	mock.AddResponse("abc123", nil)
	// git diff --name-only abc123 HEAD
	mock.AddResponse("main.go\ninternal/app.go", nil)

	stage := NewDiffCheckStage(mock)
	sc := StoryContext{
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
}

func TestDiffCheckStage_EmptyDiff(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil) // merge-base
	mock.AddResponse("", nil)       // empty diff

	stage := NewDiffCheckStage(mock)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if result != StageFailed {
		t.Errorf("expected StageFailed for empty diff, got %s", result)
	}
	if err == nil {
		t.Error("expected descriptive error for empty diff")
	}
}

func TestDiffCheckStage_OnlyGitignore(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)   // merge-base
	mock.AddResponse(".gitignore", nil) // only .gitignore changed

	stage := NewDiffCheckStage(mock)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if result != StageFailed {
		t.Errorf("expected StageFailed for gitignore-only, got %s", result)
	}
	if err == nil {
		t.Error("expected descriptive error for trivial-only changes")
	}
}

func TestDiffCheckStage_GitignorePlusOther(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)               // merge-base
	mock.AddResponse(".gitignore\nmain.go", nil)   // gitignore plus real file

	stage := NewDiffCheckStage(mock)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed when real files changed, got %s", result)
	}
}

func TestDiffCheckStage_MergeBaseError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", errors.New("no merge base"))

	stage := NewDiffCheckStage(mock)
	sc := StoryContext{
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

func TestDiffCheckStage_DiffError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)
	mock.AddResponse("", errors.New("diff failed"))

	stage := NewDiffCheckStage(mock)
	sc := StoryContext{
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
