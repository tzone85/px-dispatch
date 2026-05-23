// Package pipeline provides a composable stage-based pipeline for post-execution
// processing of stories (review, QA, rebase, merge). Each stage gets independent
// retry budgets and configurable exhaustion policies.
package pipeline

import "context"

// StageResult represents the outcome of a pipeline stage.
type StageResult int

const (
	// StagePassed indicates the stage succeeded; advance to the next stage.
	StagePassed StageResult = iota
	// StageFailed indicates the stage failed; retry up to per-stage limit,
	// then apply the on_exhaust policy.
	StageFailed
	// StageFatal indicates a non-recoverable error; pause the requirement immediately.
	StageFatal
)

// String returns a human-readable representation of the stage result.
func (r StageResult) String() string {
	switch r {
	case StagePassed:
		return "passed"
	case StageFailed:
		return "failed"
	case StageFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// StoryContext carries the state needed for pipeline stages to operate on a story.
// This struct is immutable by convention; stages must not modify it.
type StoryContext struct {
	StoryID            string
	ReqID              string
	Branch             string
	WorktreePath       string
	RepoDir            string
	AgentID            string
	RuntimeName        string
	BaseBranch         string
	StoryTitle         string   // Optional: passed through to the review stage.
	StoryDescription   string   // Optional: lets reviewers judge intent.
	AcceptanceCriteria string   // Optional: enforces the spec-compliance gate.
	OwnedFiles         []string // Files the spec asked the agent to produce/own.
}

// Stage is a single step in the post-execution pipeline.
// Implementations must be safe for concurrent use if the same stage instance
// is shared across multiple runners.
type Stage interface {
	// Name returns a unique identifier for this stage (e.g. "review", "merge").
	Name() string
	// Execute runs the stage logic for the given story context.
	// It returns a StageResult indicating success, failure (retryable), or fatal error.
	Execute(ctx context.Context, sc StoryContext) (StageResult, error)
}
