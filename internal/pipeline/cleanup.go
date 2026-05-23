package pipeline

import (
	"context"
	"log/slog"

	"github.com/tzone85/px-dispatch/internal/git"
)

// CleanupStage removes the worktree, local branch, and remote branch.
// Cleanup errors are logged but do not fail the stage — best-effort only.
type CleanupStage struct {
	runner git.CommandRunner
}

// NewCleanupStage creates a CleanupStage that uses the given runner.
func NewCleanupStage(runner git.CommandRunner) *CleanupStage {
	return &CleanupStage{runner: runner}
}

// Name returns the stage identifier.
func (s *CleanupStage) Name() string { return "cleanup" }

// Execute removes the worktree, local branch, and remote branch.
// All errors are logged but never cause stage failure.
func (s *CleanupStage) Execute(_ context.Context, sc StoryContext) (StageResult, error) {
	// Remove worktree.
	if _, err := s.runner.Run(sc.RepoDir, "git", "worktree", "remove", sc.WorktreePath, "--force"); err != nil {
		slog.Warn("cleanup: failed to remove worktree",
			"path", sc.WorktreePath,
			"error", err,
		)
	}

	// Delete local branch.
	if _, err := s.runner.Run(sc.RepoDir, "git", "branch", "-D", sc.Branch); err != nil {
		slog.Warn("cleanup: failed to delete local branch",
			"branch", sc.Branch,
			"error", err,
		)
	}

	// Delete remote branch.
	if _, err := s.runner.Run(sc.RepoDir, "git", "push", "origin", "--delete", sc.Branch); err != nil {
		slog.Warn("cleanup: failed to delete remote branch",
			"branch", sc.Branch,
			"error", err,
		)
	}

	return StagePassed, nil
}
