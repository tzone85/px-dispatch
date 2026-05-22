---
title: Pipeline stages walkthrough
tags: [project-x, pipeline]
---

# Pipeline stages walkthrough

The pipeline is a slice of `pipeline.Stage` constructed in
`internal/cli/resume.go`. Each stage returns `StagePassed`, `StageFailed`
(retryable), or `StageFatal` (pauses the requirement).

```
autocommit → diffcheck → review → qa → rebase → merge → cleanup
```

## The handoff: filesystem sentinel

```sh
rm -f .px-done
cat <<'PX_EOF' | claude … | tee transcript
{goal}
PX_EOF
rc=$?
printf '$\n'
touch .px-done
sleep 30
exit $rc
```

Three earlier bugs we walked into:

1. `--output-file` isn't a real claude flag → claude exited immediately →
   tmux died in <10 s.
2. The poller treated missing sessions as `agent.lost` even when the agent
   finished cleanly.
3. `status=$?` is a *read-only* variable in zsh (macOS tmux default-shell);
   the assignment errored and the sentinel never landed.

`rc=$?` + `touch .px-done` + the sentinel check in the poller is the fix.

## Stage 1 — autocommit

Stages a single commit with all the agent's tracked + untracked changes.
Empty diff → `StageSkipped`; the next stage (`diffcheck`) will fail it.

## Stage 2 — diffcheck

Confirms the agent actually changed files in `OwnedFiles`. Files outside
the declared ownership trigger a fail with a message back to the agent.

## Stage 3 — review (LLM judge)

Sends diff + spec to an LLM with the
[[03-Agent-prompts-and-DDD-TDD-enforcement|review prompt]]. Six explicit
failure conditions:

1. **Structural mismatch** — OwnedFiles not touched.
2. **Unfulfilled acceptance criteria**.
3. **No tests** (or tests assert implementation details).
4. **Unhandled errors / silent failures**.
5. **Security issue** — hardcoded secrets, injection vectors, missing
   validation.
6. **Broken invariant** — removed tests, weakened public contract.

Response as JSON `{"passed": bool, "summary": "…", "comments": [...]}`.

## Stage 4 — qa

Detects tech stack (`go`, `npm`, `python`, etc.) and runs lint/test/build.
"No test files found" is treated as a non-failure (early scaffolding
stories may have no tests yet).

## Stage 5 — rebase

`git fetch origin <base>` → `git rebase origin/<base>`. On conflicts, the
LLM-powered [[05-Conflict-resolution-and-rebase-guard|ConflictResolver]]
runs up to N rounds.

**Stale-rebase guard** (new): aborts any leftover in-progress rebase
before starting. Closes the "rebase-merge directory already exists" hang
that VXD finding #4 surfaced.

## Stage 6 — merge

`git push` → `gh pr create` → `gh pr merge --auto` (or immediate if
`merge.auto_merge: true`). Serialised by `monitor.poller.mergeMu`.

## Stage 7 — cleanup

Removes the worktree, deletes the local branch, deletes the remote branch.
Best-effort.

## When a stage fails

| Result | Effect |
|---|---|
| `StagePassed` | advance |
| `StageFailed` | retry up to `pipeline.stages.<stage>.max_retries`, then `on_exhaust` policy |
| `StageFatal` | pause requirement; emit escalation event |
