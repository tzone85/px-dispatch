package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newGCCmd() *cobra.Command {
	var prune bool
	var killTmux bool
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Garbage collect old worktrees, stale branches, and tmux sessions",
		Long: "Removes worktrees and branches for archived/completed requirements " +
			"so the system stays fire-and-forget. Use --prune to also delete stale " +
			"px/* local branches that no longer have a worktree. Use --kill-tmux " +
			"to terminate every px-* tmux session.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGC(prune, killTmux)
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "also delete stale px/* local branches")
	cmd.Flags().BoolVar(&killTmux, "kill-tmux", false, "kill all px-* tmux sessions")
	return cmd
}

// runGC removes worktrees under the state dir for archived or completed
// requirements, optionally prunes stale local branches and tmux sessions.
func runGC(prune, killTmux bool) error {
	reqs, err := app.projStore.ListRequirements(state.ReqFilter{})
	if err != nil {
		return fmt.Errorf("list requirements: %w", err)
	}

	worktreesDir := filepath.Join(app.stateDir, "worktrees")
	removedWorktrees, err := gcWorktrees(worktreesDir, reqs)
	if err != nil {
		return err
	}

	removedBranches := 0
	if prune {
		removedBranches = gcStaleBranches(worktreesDir)
	}

	killedSessions := 0
	if killTmux {
		killedSessions = gcTmuxSessions()
	}

	fmt.Printf("Garbage collection complete: %d worktrees, %d branches, %d tmux sessions removed.\n",
		removedWorktrees, removedBranches, killedSessions)
	return nil
}

func gcWorktrees(worktreesDir string, reqs []state.Requirement) (int, error) {
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read worktrees dir: %w", err)
	}

	// Build set of active story IDs (non-archived, non-completed). Worktree
	// names are typically <reqID>-s-<n> but we match either the req prefix or
	// the full story id, leaving conservative behaviour for unknown names.
	activeReqIDs := make(map[string]bool, len(reqs))
	for _, req := range reqs {
		if req.Status != "archived" && req.Status != "completed" {
			activeReqIDs[req.ID] = true
		}
	}

	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isActiveWorktree(name, activeReqIDs) {
			continue
		}
		target := filepath.Join(worktreesDir, name)
		if err := os.RemoveAll(target); err != nil {
			fmt.Printf("Failed to remove worktree %s: %v\n", target, err)
			continue
		}
		fmt.Printf("Removed worktree: %s\n", target)
		removed++
	}
	return removed, nil
}

// isActiveWorktree returns true when the worktree name corresponds to a
// requirement that has not been archived or completed. Accepts either the
// full requirement id ("01ABC") or any story id prefixed with it ("01ABC-s-1").
func isActiveWorktree(name string, activeReqIDs map[string]bool) bool {
	if activeReqIDs[name] {
		return true
	}
	for reqID := range activeReqIDs {
		if strings.HasPrefix(name, reqID) {
			return true
		}
	}
	return false
}

// gcStaleBranches deletes px/* branches that no longer have a corresponding
// worktree on disk. Best-effort.
func gcStaleBranches(worktreesDir string) int {
	repoDir, _ := os.Getwd()
	if repoDir == "" {
		return 0
	}
	branches, err := listLocalPxBranches(repoDir)
	if err != nil {
		fmt.Printf("Listing branches failed (skipping): %v\n", err)
		return 0
	}
	removed := 0
	for _, branch := range branches {
		storyID := strings.TrimPrefix(branch, "px/")
		if _, err := os.Stat(filepath.Join(worktreesDir, storyID)); err == nil {
			continue // worktree still exists → keep branch
		}
		if err := runGit(repoDir, "branch", "-D", branch); err != nil {
			fmt.Printf("Skipped branch %s: %v\n", branch, err)
			continue
		}
		fmt.Printf("Removed branch: %s\n", branch)
		removed++
	}
	return removed
}

// gcTmuxSessions kills every tmux session whose name starts with "px-". The
// monitor's poller no longer needs them once a requirement is complete or
// abandoned. Best-effort.
func gcTmuxSessions() int {
	out, err := runShellCapture("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return 0
	}
	removed := 0
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(name, "px-") {
			continue
		}
		if err := runShellQuiet("tmux", "kill-session", "-t", name); err == nil {
			fmt.Printf("Killed tmux session: %s\n", name)
			removed++
		}
	}
	return removed
}
