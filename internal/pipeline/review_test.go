package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/llm"
)

func TestReviewStage_Name(t *testing.T) {
	stage := NewReviewStage(git.NewMockRunner(), llm.NewReplayClient())
	if stage.Name() != "review" {
		t.Errorf("expected name 'review', got %q", stage.Name())
	}
}

func TestReviewStage_Passed(t *testing.T) {
	mock := git.NewMockRunner()
	// merge-base
	mock.AddResponse("abc123", nil)
	// git diff abc123
	mock.AddResponse("diff --git a/main.go\n+code", nil)
	// git ls-files
	mock.AddResponse("main.go\ngo.mod", nil)

	replay := llm.NewReplayClient(llm.CompletionResponse{
		Content: `{"passed": true, "summary": "Code looks good.", "comments": []}`,
	})

	stage := NewReviewStage(mock, replay)
	sc := StoryContext{
		StoryID:      "STORY-1",
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

func TestReviewStage_Failed(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)
	mock.AddResponse("diff --git a/main.go\n+bad code", nil)
	mock.AddResponse("main.go", nil)

	replay := llm.NewReplayClient(llm.CompletionResponse{
		Content: `{"passed": false, "summary": "Missing error handling.", "comments": ["line 10: no error check"]}`,
	})

	stage := NewReviewStage(mock, replay)
	sc := StoryContext{
		StoryID:      "STORY-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
	// err contains the review feedback for retry/logging
	if err == nil {
		t.Error("expected descriptive error with review feedback")
	}
}

func TestReviewStage_LLMFatalError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)
	mock.AddResponse("diff content", nil)
	mock.AddResponse("main.go", nil)

	// Use a mock LLM client that returns a fatal API error.
	fatalClient := &errorLLMClient{err: &llm.APIError{StatusCode: 401, Message: "unauthorized", Retryable: false}}

	stage := NewReviewStage(mock, fatalClient)
	sc := StoryContext{
		StoryID:      "STORY-1",
		WorktreePath: "/tmp/wt",
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

func TestReviewStage_LLMRetryableError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)
	mock.AddResponse("diff content", nil)
	mock.AddResponse("main.go", nil)

	retryClient := &errorLLMClient{err: &llm.APIError{StatusCode: 529, Message: "overloaded", Retryable: true}}

	stage := NewReviewStage(mock, retryClient)
	sc := StoryContext{
		StoryID:      "STORY-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed for retryable error, got %s", result)
	}
}

func TestReviewStage_DiffError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", errors.New("merge-base failed"))

	stage := NewReviewStage(mock, llm.NewReplayClient())
	sc := StoryContext{WorktreePath: "/tmp/wt", BaseBranch: "main"}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed, got %s", result)
	}
}

func TestReviewStage_InvalidJSON(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("abc123", nil)
	mock.AddResponse("diff content", nil)
	mock.AddResponse("main.go", nil)

	replay := llm.NewReplayClient(llm.CompletionResponse{
		Content: "this is not json",
	})

	stage := NewReviewStage(mock, replay)
	sc := StoryContext{
		StoryID:      "STORY-1",
		WorktreePath: "/tmp/wt",
		BaseBranch:   "main",
	}

	result, err := stage.Execute(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if result != StageFailed {
		t.Errorf("expected StageFailed for parse error, got %s", result)
	}
}

// errorLLMClient always returns a configured error.
type errorLLMClient struct {
	err error
}

func (c *errorLLMClient) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, c.err
}
