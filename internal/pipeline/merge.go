package pipeline

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
)

// MergeStage pushes the branch and creates a GitHub pull request.
// If autoMerge is true, it also enables auto-merge with squash strategy.
type MergeStage struct {
	runner    git.CommandRunner
	autoMerge bool
}

// NewMergeStage creates a MergeStage with the given runner and auto-merge setting.
func NewMergeStage(runner git.CommandRunner, autoMerge bool) *MergeStage {
	return &MergeStage{runner: runner, autoMerge: autoMerge}
}

// Name returns the stage identifier.
func (s *MergeStage) Name() string { return "merge" }

// Execute pushes the branch, creates a PR, and optionally enables auto-merge.
func (s *MergeStage) Execute(_ context.Context, sc StoryContext) (StageResult, error) {
	baseBranch := sc.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Push branch to remote.
	if _, err := s.runner.Run(sc.WorktreePath, "git", "push", "-u", "origin", sc.Branch); err != nil {
		return StageFailed, fmt.Errorf("pushing branch %s: %w", sc.Branch, err)
	}

	// Create pull request.
	title := fmt.Sprintf("%s: automated changes", sc.StoryID)
	body := fmt.Sprintf("Automated PR for story %s", sc.StoryID)

	prURL, err := s.runner.Run(sc.WorktreePath, "gh",
		"pr", "create",
		"--head", sc.Branch,
		"--base", baseBranch,
		"--title", title,
		"--body", body,
	)
	if err != nil {
		return StageFailed, fmt.Errorf("creating PR: %w", err)
	}

	if !s.autoMerge {
		return StagePassed, nil
	}

	// Enable auto-merge.
	prNumber, err := extractPRNumber(prURL)
	if err != nil {
		return StageFailed, fmt.Errorf("parsing PR number: %w", err)
	}

	if _, err := s.runner.Run(sc.WorktreePath, "gh",
		"pr", "merge", strconv.Itoa(prNumber),
		"--squash", "--auto",
	); err != nil {
		return StageFailed, fmt.Errorf("enabling auto-merge: %w", err)
	}

	return StagePassed, nil
}

// extractPRNumber parses a PR number from a GitHub PR URL.
// Expected format: https://github.com/owner/repo/pull/<number>
func extractPRNumber(url string) (int, error) {
	url = strings.TrimSpace(url)
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid PR URL: %s", url)
	}

	last := parts[len(parts)-1]
	num, err := strconv.Atoi(last)
	if err != nil {
		return 0, fmt.Errorf("invalid PR number in URL %s: %w", url, err)
	}

	return num, nil
}
