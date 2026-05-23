package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/llm"
)

func TestRebaseStage_Name(t *testing.T) {
	stage := NewRebaseStage(git.NewMockRunner(), llm.NewReplayClient(), 10)
	if stage.Name() != "rebase" {
		t.Errorf("expected name 'rebase', got %q", stage.Name())
	}
}

func TestRebaseStage_CleanRebase(t *testing.T) {
	mock := git.NewMockRunner()
	// git fetch origin main
	mock.AddResponse("", nil)
	// git rebase origin/main (succeeds)
	mock.AddResponse("", nil)

	stage := NewRebaseStage(mock, llm.NewReplayClient(), 10)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	// Verify fetch in repo dir, rebase in worktree dir
	fetchCmd := mock.Commands[0]
	if fetchCmd.Dir != "/tmp/repo" {
		t.Errorf("fetch should run in repo dir, got %s", fetchCmd.Dir)
	}
	rebaseCmd := mock.Commands[1]
	if rebaseCmd.Dir != "/tmp/wt" {
		t.Errorf("rebase should run in worktree dir, got %s", rebaseCmd.Dir)
	}
}

func TestRebaseStage_ConflictResolved(t *testing.T) {
	mock := git.NewMockRunner()
	// git fetch origin main
	mock.AddResponse("", nil)
	// git rebase origin/main (fails with conflict)
	mock.AddResponse("", errors.New("CONFLICT"))
	// git diff --name-only --diff-filter=U (conflicted files)
	mock.AddResponse("main.go", nil)
	// cat main.go (read conflicted file)
	mock.AddResponse("<<<<<<< HEAD\nour code\n=======\ntheir code\n>>>>>>> origin/main", nil)
	// Write resolved file: tee main.go (write back resolved)
	mock.AddResponse("", nil)
	// git add main.go
	mock.AddResponse("", nil)
	// GIT_EDITOR=true git rebase --continue (succeeds)
	mock.AddResponse("", nil)

	replay := llm.NewReplayClient(llm.CompletionResponse{
		Content: "our code\ntheir code merged",
	})

	stage := NewRebaseStage(mock, replay, 10)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected StagePassed, got %s", result)
	}

	// Verify the LLM was called to resolve the conflict, and add + continue were run
	var foundAdd, foundContinue bool
	for _, cmd := range mock.Commands {
		args := strings.Join(cmd.Args, " ")
		if cmd.Name == "git" && strings.HasPrefix(args, "add ") {
			foundAdd = true
		}
		if cmd.Name == "git" && strings.Contains(args, "rebase --continue") {
			foundContinue = true
		}
	}
	if !foundAdd {
		t.Error("expected git add for resolved file")
	}
	if !foundContinue {
		t.Error("expected git rebase --continue")
	}
}

func TestRebaseStage_UnresolvableConflict(t *testing.T) {
	mock := git.NewMockRunner()
	// git fetch origin main
	mock.AddResponse("", nil)
	// git rebase origin/main (conflict)
	mock.AddResponse("", errors.New("CONFLICT"))

	// Round 1: conflict detection and resolution attempt
	mock.AddResponse("main.go", nil)                        // diff --name-only --diff-filter=U
	mock.AddResponse("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>>", nil) // cat main.go
	mock.AddResponse("", nil)                                // tee (write resolved)
	mock.AddResponse("", nil)                                // git add main.go
	mock.AddResponse("", errors.New("CONFLICT"))             // rebase --continue fails again

	// Round 2: still conflicting
	mock.AddResponse("main.go", nil)
	mock.AddResponse("<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>>", nil)
	mock.AddResponse("", nil)
	mock.AddResponse("", nil)
	mock.AddResponse("", errors.New("CONFLICT"))

	// After max rounds: git rebase --abort
	mock.AddResponse("", nil)

	replay := llm.NewReplayClient(llm.CompletionResponse{
		Content: "resolved content",
	})

	stage := NewRebaseStage(mock, replay, 2) // only 2 rounds
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}

	// Verify rebase --abort was called
	lastCmd := mock.Commands[len(mock.Commands)-1]
	if !strings.Contains(strings.Join(lastCmd.Args, " "), "rebase --abort") {
		t.Errorf("expected last command to be 'git rebase --abort', got %s %s",
			lastCmd.Name, strings.Join(lastCmd.Args, " "))
	}
}

func TestRebaseStage_FetchError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", errors.New("network error"))

	stage := NewRebaseStage(mock, llm.NewReplayClient(), 10)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
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

func TestRebaseStage_DefaultMaxRounds(t *testing.T) {
	stage := NewRebaseStage(git.NewMockRunner(), llm.NewReplayClient(), 0)
	if stage.maxRounds != defaultMaxRounds {
		t.Errorf("expected default max rounds %d, got %d", defaultMaxRounds, stage.maxRounds)
	}
}

func TestRebaseStage_LLMFatalError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                    // fetch
	mock.AddResponse("", errors.New("CONFLICT")) // rebase
	mock.AddResponse("main.go", nil)             // diff --name-only
	mock.AddResponse("conflict content", nil)    // cat file

	fatalClient := &errorLLMClient{err: &llm.APIError{StatusCode: 401, Message: "unauthorized", Retryable: false}}

	// After fatal LLM error, should abort the rebase
	mock.AddResponse("", nil) // rebase --abort

	stage := NewRebaseStage(mock, fatalClient, 10)
	sc := StoryContext{
		WorktreePath: "/tmp/wt",
		RepoDir:      "/tmp/repo",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFatal {
		t.Errorf("expected StageFatal for fatal API error, got %s", result)
	}
}
