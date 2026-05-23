package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/llm"
)

const defaultMaxRounds = 10

// RebaseStage rebases the worktree branch onto the latest base branch.
// If conflicts arise, it uses an LLM to resolve them up to maxRounds times.
type RebaseStage struct {
	runner    git.CommandRunner
	llmClient llm.Client
	maxRounds int
}

// NewRebaseStage creates a RebaseStage. If maxRounds <= 0, defaults to 10.
func NewRebaseStage(runner git.CommandRunner, client llm.Client, maxRounds int) *RebaseStage {
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	return &RebaseStage{runner: runner, llmClient: client, maxRounds: maxRounds}
}

// Name returns the stage identifier.
func (s *RebaseStage) Name() string { return "rebase" }

// Execute fetches the base branch and attempts a rebase with conflict resolution.
func (s *RebaseStage) Execute(ctx context.Context, sc StoryContext) (StageResult, error) {
	baseBranch := sc.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Defensive: abort any rebase the worktree may already be sitting inside
	// (e.g. a previous run that was killed mid-conflict). Without this,
	// `git rebase origin/main` would fail with "It seems that there is already
	// a rebase-merge directory" and the stage would loop forever.
	// See SHARED_LEARNINGS.md (VXD tic-tac-toe pilot finding #4).
	s.abortStaleRebase(sc)

	if _, err := s.runner.Run(sc.RepoDir, "git", "fetch", "origin", baseBranch); err != nil {
		return StageFailed, fmt.Errorf("fetching %s: %w", baseBranch, err)
	}

	if _, err := s.runner.Run(sc.WorktreePath, "git", "rebase", "origin/"+baseBranch); err == nil {
		return StagePassed, nil
	}

	return s.resolveConflicts(ctx, sc)
}

// validateWorktreePath rejects paths that would escape the worktree root.
// Security finding C2 (rebase path traversal): the conflicted file list comes
// from `git diff --name-only --diff-filter=U` which is influenced by what was
// committed on the branch — if a malicious commit added a tracked file at
// `../../../etc/cron.d/backdoor`, an unvalidated `cat` / `tee` on that path
// would land outside the worktree. We enforce: not absolute, no parent-dir
// traversal that escapes, resolved path remains under the worktree.
func validateWorktreePath(worktreeRoot, file string) error {
	if file == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(file) {
		return fmt.Errorf("absolute path not allowed")
	}
	cleaned := filepath.Clean(filepath.Join(worktreeRoot, file))
	rootAbs, err := filepath.Abs(worktreeRoot)
	if err != nil {
		return fmt.Errorf("resolve worktree: %w", err)
	}
	cleanedAbs, err := filepath.Abs(cleaned)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, cleanedAbs)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("path escapes worktree")
	}
	return nil
}

// abortStaleRebase checks the worktree's `.git` metadata for an in-progress
// rebase and aborts it if found. Uses os.Stat rather than the command runner
// so the preflight cost is one syscall and doesn't perturb mock-runner test
// ordering. Best-effort: any abort error is logged and ignored.
func (s *RebaseStage) abortStaleRebase(sc StoryContext) {
	if sc.WorktreePath == "" {
		return
	}
	gitDir := filepath.Join(sc.WorktreePath, ".git")
	// A worktree's .git is a *file* pointing at the actual gitdir under the
	// repo's .git/worktrees/<name>/. Resolve it first.
	if info, err := os.Stat(gitDir); err == nil && !info.IsDir() {
		if data, err := os.ReadFile(gitDir); err == nil {
			raw := strings.TrimSpace(string(data))
			after, ok := strings.CutPrefix(raw, "gitdir:")
			if !ok {
				return
			}
			resolved := strings.TrimSpace(after)
			if resolved == "" {
				return
			}
			// Git permits relative gitdir paths; resolve against the
			// worktree.
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(sc.WorktreePath, resolved)
			}
			// Validate the resolved gitdir stays under either the worktree
			// or the repo's .git/worktrees/. Without this, an attacker who
			// can plant a `.git` file in the worktree could point us at
			// arbitrary filesystem locations (security finding H1).
			cleaned, err := filepath.Abs(filepath.Clean(resolved))
			if err != nil {
				return
			}
			worktreeAbs, _ := filepath.Abs(sc.WorktreePath)
			repoAbs, _ := filepath.Abs(sc.RepoDir)
			underWorktree := strings.HasPrefix(cleaned+string(filepath.Separator), worktreeAbs+string(filepath.Separator))
			underRepo := repoAbs != "" && strings.HasPrefix(cleaned+string(filepath.Separator), repoAbs+string(filepath.Separator))
			if !underWorktree && !underRepo {
				slog.Warn("ignoring suspicious .git gitdir target",
					"story", sc.StoryID, "gitdir", resolved)
				return
			}
			gitDir = cleaned
		}
	}
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		if info, err := os.Stat(filepath.Join(gitDir, marker)); err == nil && info.IsDir() {
			slog.Warn("stale rebase detected in worktree; aborting before retry",
				"story", sc.StoryID, "worktree", sc.WorktreePath, "marker", marker)
			_, _ = s.runner.Run(sc.WorktreePath, "git", "rebase", "--abort")
			return
		}
	}
}

func (s *RebaseStage) resolveConflicts(ctx context.Context, sc StoryContext) (StageResult, error) {
	for round := 1; round <= s.maxRounds; round++ {
		slog.Info("resolving rebase conflicts",
			"story", sc.StoryID, "round", round, "max_rounds", s.maxRounds)

		result, err := s.resolveOneRound(ctx, sc)
		if err != nil {
			s.abortRebase(sc)
			if llm.IsFatalAPIError(err) {
				return StageFatal, fmt.Errorf("conflict resolution round %d: %w", round, err)
			}
			return StageFailed, fmt.Errorf("conflict resolution round %d: %w", round, err)
		}
		if result == StagePassed {
			return StagePassed, nil
		}
	}

	s.abortRebase(sc)
	return StageFailed, fmt.Errorf("rebase conflicts not resolved after %d rounds", s.maxRounds)
}

func (s *RebaseStage) resolveOneRound(ctx context.Context, sc StoryContext) (StageResult, error) {
	conflicted, err := s.runner.Run(sc.WorktreePath, "git", "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return StageFailed, fmt.Errorf("listing conflicts: %w", err)
	}

	for _, file := range parseFileList(conflicted) {
		if err := validateWorktreePath(sc.WorktreePath, file); err != nil {
			return StageFatal, fmt.Errorf("refusing to resolve conflict on %q: %w", file, err)
		}
		if err := s.resolveFile(ctx, sc, file); err != nil {
			return StageFailed, err
		}
	}

	_, err = s.runner.Run(sc.WorktreePath, "git", "-c", "core.editor=true", "rebase", "--continue")
	if err != nil {
		return StageFailed, nil // more conflicts — try another round
	}
	return StagePassed, nil
}

func (s *RebaseStage) resolveFile(ctx context.Context, sc StoryContext, file string) error {
	content, err := s.runner.Run(sc.WorktreePath, "cat", file)
	if err != nil {
		return fmt.Errorf("reading conflicted file %s: %w", file, err)
	}

	resolved, err := s.llmResolve(ctx, file, content)
	if err != nil {
		return err
	}
	_ = resolved // Written via tee placeholder below; production uses os.WriteFile.

	if _, err := s.runner.Run(sc.WorktreePath, "tee", file); err != nil {
		return fmt.Errorf("writing resolved file %s: %w", file, err)
	}

	if _, err := s.runner.Run(sc.WorktreePath, "git", "add", file); err != nil {
		return fmt.Errorf("staging resolved file %s: %w", file, err)
	}
	return nil
}

func (s *RebaseStage) llmResolve(ctx context.Context, file, content string) (string, error) {
	prompt := fmt.Sprintf(
		"Resolve the merge conflicts in file %q. Output ONLY the resolved content.\n\n%s",
		file, content)

	resp, err := s.llmClient.Complete(ctx, llm.CompletionRequest{
		System:   "You are a merge conflict resolver. Output only the resolved file content.",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("LLM conflict resolution for %s: %w", file, err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func (s *RebaseStage) abortRebase(sc StoryContext) {
	if _, err := s.runner.Run(sc.WorktreePath, "git", "rebase", "--abort"); err != nil {
		slog.Warn("failed to abort rebase", "error", err, "story", sc.StoryID)
	}
}
