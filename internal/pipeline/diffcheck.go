package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
)

// DiffCheckStage verifies that the worktree has meaningful changes relative to
// the base branch. It fails if there are no changes or only trivial files
// (e.g. .gitignore) were modified.
type DiffCheckStage struct {
	runner git.CommandRunner
}

// NewDiffCheckStage creates a DiffCheckStage that uses the given runner.
func NewDiffCheckStage(runner git.CommandRunner) *DiffCheckStage {
	return &DiffCheckStage{runner: runner}
}

// Name returns the stage identifier.
func (s *DiffCheckStage) Name() string { return "diffcheck" }

// Execute checks for meaningful changes between HEAD and the base branch.
func (s *DiffCheckStage) Execute(_ context.Context, sc StoryContext) (StageResult, error) {
	baseBranch := sc.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	mergeBase, err := s.runner.Run(sc.WorktreePath, "git", "merge-base", "HEAD", "origin/"+baseBranch)
	if err != nil {
		return StageFailed, fmt.Errorf("finding merge-base: %w", err)
	}

	diffOut, err := s.runner.Run(sc.WorktreePath, "git", "diff", "--name-only", mergeBase, "HEAD")
	if err != nil {
		return StageFailed, fmt.Errorf("getting diff: %w", err)
	}

	files := parseFileList(diffOut)
	if len(files) == 0 {
		return StageFailed, fmt.Errorf("no changes found relative to %s", baseBranch)
	}

	if onlyTrivialFiles(files) {
		return StageFailed, fmt.Errorf("no real changes (only trivial files modified)")
	}

	return StagePassed, nil
}

// trivialFiles is the set of file names considered non-meaningful on their own.
var trivialFiles = map[string]bool{
	".gitignore": true,
}

// parseFileList splits newline-separated output into a slice, filtering blanks.
func parseFileList(output string) []string {
	if output == "" {
		return nil
	}
	var files []string
	for _, f := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

// onlyTrivialFiles returns true if every file in the list is in the trivial set.
func onlyTrivialFiles(files []string) bool {
	for _, f := range files {
		if !trivialFiles[f] {
			return false
		}
	}
	return true
}
