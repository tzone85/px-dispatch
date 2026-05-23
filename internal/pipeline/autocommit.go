package pipeline

import (
	"context"
	"fmt"

	"github.com/tzone85/px-dispatch/internal/git"
)

// AutoCommitStage stages and commits any uncommitted changes in the worktree.
// If no changes are detected, it passes without creating a commit.
type AutoCommitStage struct {
	runner git.CommandRunner
}

// NewAutoCommitStage creates an AutoCommitStage that uses the given runner
// for all git operations.
func NewAutoCommitStage(runner git.CommandRunner) *AutoCommitStage {
	return &AutoCommitStage{runner: runner}
}

// Name returns the stage identifier.
func (s *AutoCommitStage) Name() string { return "autocommit" }

// Execute checks for uncommitted changes and commits them if found.
func (s *AutoCommitStage) Execute(_ context.Context, sc StoryContext) (StageResult, error) {
	status, err := s.runner.Run(sc.WorktreePath, "git", "status", "--porcelain")
	if err != nil {
		return StageFailed, fmt.Errorf("checking git status: %w", err)
	}

	if status == "" {
		return StagePassed, nil
	}

	if _, err := s.runner.Run(sc.WorktreePath, "git", "add", "-A"); err != nil {
		return StageFailed, fmt.Errorf("staging changes: %w", err)
	}

	msg := fmt.Sprintf("feat(%s): auto-commit agent work", sc.StoryID)
	if _, err := s.runner.Run(sc.WorktreePath, "git", "commit", "-m", msg); err != nil {
		return StageFailed, fmt.Errorf("committing changes: %w", err)
	}

	return StagePassed, nil
}
