---
title: Conflict resolution and rebase guard
tags: [project-x, rebase, conflicts]
---

# Conflict resolution and rebase guard

When two stories touch the same file, the rebase stage produces conflicts.
Project X resolves them with an LLM rather than aborting back to the user.

## Flow

```
git rebase origin/main
  └─ conflict?
       ├─ no  → StagePassed
       └─ yes → resolveConflicts (up to N rounds)
                  ├─ list conflicted files
                  ├─ for each file: read content, ask LLM, write back, `git add`
                  └─ `git rebase --continue`
```

The LLM is told the file MUST be: free of conflict markers, syntactically
valid for its language, a minimal merge that preserves both branches.

## Max-rounds policy

`pipeline.stages.rebase.max_rounds` (default `10`) caps the loop. On
exhaust:

- `git rebase --abort`.
- Stage returns `StageFailed`.
- Retry/escalation policy takes over.

## The stale-rebase guard

A killed run leaves `.git/rebase-merge/` or `.git/rebase-apply/` in the
worktree. The next `git rebase origin/main` would fail with:

```
fatal: It seems that there is already a rebase-merge directory, …
```

Fix:

```go
func (s *RebaseStage) abortStaleRebase(sc StoryContext) {
    gitDir := resolveGitDir(sc.WorktreePath)  // worktree's .git is a *file*
    for _, marker := range []string{"rebase-merge", "rebase-apply"} {
        if info, err := os.Stat(filepath.Join(gitDir, marker)); err == nil && info.IsDir() {
            _, _ = s.runner.Run(sc.WorktreePath, "git", "rebase", "--abort")
            return
        }
    }
}
```

Uses `os.Stat` (not the runner) so it doesn't perturb mock-runner test
ordering.

## When the resolver should NOT be used

- Old generated files (lock files, bundled JS) — regenerate after the merge.
- Binary files — the resolver refuses these explicitly.

For both, mark the file in `pipeline.stages.rebase.skip_files` to bypass the
LLM and pause immediately.

## Lineage

Came from VXD tic-tac-toe pilot finding #4 (SHARED_LEARNINGS.md): in two
parallel stories that both touched `index.html`, the merger left the second
story stuck in a half-applied rebase and px exited "successfully",
abandoning an unmergeable PR.
