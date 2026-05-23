---
title: Lesson 04 · Debugging a stuck story
tags: [px-dispatch, training, debugging]
---

# Lesson 04 · Debugging a stuck story

**Goal:** Diagnose and recover from the four most common stuck states. ~30
minutes; no LLM spend (the techniques are pure inspection).

## The stuck states, in rough order of frequency

| Symptom | Likely cause |
|---|---|
| Story respawns every ~90s, no progress | Spawn script failed before `.px-done` |
| Story stuck `in_progress`, `.px-done` exists | Poller missed the sentinel cycle |
| Story keeps failing review | Reviewer rejecting spec-noncompliance |
| Rebase aborts with "rebase-merge directory already exists" | Killed mid-rebase previously |

We'll work through each.

## Setup

```bash
export SANDBOX=/tmp/px-train-04
# Re-use the connect-4 or version sandbox from prior lessons, or set up
# a fresh one. For this lesson we mostly inspect — no need to run px.
```

## Case 1 — agent respawn loop

**Symptom:** `px events --limit 50` shows alternating `story.assigned` /
`agent.lost` cycles for the same story, every ~90 seconds.

**Diagnosis ladder:**

```bash
# 1. Is the tmux session even alive?
tmux ls 2>&1

# 2. What does the agent see?
tmux capture-pane -t px-<story-id> -p -S -200

# 3. Did the script even reach the sentinel touch?
ls -la /tmp/<state>/worktrees/<story-id>/.px-done

# 4. What did the script say?
cat /tmp/<state>/worktrees/<story-id>/PX_AGENT_TRANSCRIPT.log
```

**Root causes we've seen:**

- **Bad CLI flag** — claude exited immediately. Look for "unknown flag" /
  "unrecognised option" in the transcript. Fix: align
  `internal/runtime/<cli>.go::buildCommand` with the CLI's actual flags.
- **`status=$?` in zsh** — assignment errored before `touch .px-done`. We
  fixed this; if you see it again, check the runtime didn't regress.
- **Heredoc mismatch** — historical bug; new code uses a random per-spawn
  delimiter so this shouldn't recur.
- **Claude not authenticated** — try `echo hi | claude -p --output-format
  text` outside px. If that fails, fix the CLI auth first.

**Recovery:**

```bash
pkill -9 -f "claude --dangerously-skip-permissions"
tmux kill-server
px gc --kill-tmux
# Fix the underlying bug, rebuild px, then:
px resume <req-id> --godmode
```

## Case 2 — stuck `in_progress` with sentinel present

**Symptom:** `sqlite3 px.db 'SELECT status FROM stories WHERE id=X'` returns
`in_progress`, but `ls worktrees/X/.px-done` exists.

**Diagnosis:**

```bash
# Is the px process even still running?
pgrep -fl "px.*resume" | head -3

# Is the poller goroutine alive? Tail the structured log.
tail -F /tmp/<state>/logs/px.log | grep poller
```

**Root cause:** poller's last cycle missed the sentinel — usually because the
session was missing AND the `sentinelExists` check ran before the spawn
script reached `touch .px-done` (rare race; fixed by the sentinel mechanism
but the race window still exists during the first ~5 ms).

**Recovery:**

```bash
# Cleanest: just resume. The wave loop detects stories that are merged
# and skips them; for in_progress stories it dispatches a fresh agent.
px resume <req-id> --godmode
```

If the worktree already has the agent's work committed (`git -C worktrees/X
log --oneline`), the new agent will see the work and the autocommit/review
stages will run on it.

## Case 3 — review keeps rejecting

**Symptom:** Same story, multiple `story.review_failed` events. The agent
keeps adding code, the reviewer keeps saying no.

**Diagnosis:**

```bash
jq -r 'select(.type=="story.review_failed" and .story_id=="<id>") | .payload.summary' \
   "$SANDBOX-state/events.jsonl"
```

You'll see the reviewer's verdict. The three most common rejection reasons:

1. **Structural mismatch** — the spec listed `OwnedFiles` the agent didn't
   touch (e.g., agent inlined CSS into HTML when `styles.css` was owned).
   Fix: tighten the requirement to be more explicit, OR if the spec is
   genuinely wrong, edit the story and re-run.
2. **No tests** — TDD enforcement bit. Agent didn't write tests despite
   the prompt demanding them.
3. **Hardcoded values / secrets** — usually a junior model. Re-run with the
   senior or intermediate tier by lowering
   `routing.junior_max_complexity`.

**Recovery:** Most of the time, just `px resume` again — the `ReviewFeedback`
field is plumbed into the next agent's prompt and the agent fixes it. If
you've burned 3+ review cycles on the same story without progress,
intervene manually: edit the story's `description` in `px.db`, or break the
requirement down into smaller stories.

## Case 4 — rebase fails with "rebase-merge directory already exists"

**Symptom:** `pipeline error ... rebase already in progress`.

**Diagnosis:**

```bash
ls "$SANDBOX-state/worktrees/<story-id>/.git/rebase-merge" 2>&1
# Or for a worktree (.git is a file pointing elsewhere):
cat "$SANDBOX-state/worktrees/<story-id>/.git"
ls "$(cat ...gitdir)/rebase-merge" 2>&1
```

**Root cause:** A previous run was killed mid-rebase and left the marker
directory. The current code aborts stale rebases automatically (see
[[../05-Conflict-resolution-and-rebase-guard]]), so if you're seeing this
in practice the guard didn't fire.

**Recovery:**

```bash
cd "$SANDBOX-state/worktrees/<story-id>"
git rebase --abort 2>&1
cd -
# Then resume:
px resume <req-id> --godmode
```

If `git rebase --abort` itself fails, the rebase-merge directory is
corrupted. Nuclear:

```bash
rm -rf "$SANDBOX-state/worktrees/<story-id>"
px gc
px resume <req-id> --godmode    # px will re-create the worktree
```

## Exercises

### A — diagnose from events alone

Pull a real `events.jsonl` from a recent run and answer:
1. Which story had the most review cycles?
2. Which story took longest from `story.assigned` to `story.merged`?
3. Did any agent die without producing work?

### B — synthesise a stuck case

In a sandbox: start a `px resume`, immediately `kill -9` the px process
before any story makes progress. Then `px resume` again. Document what px
does. (Hint: it should pick up where it left off; the stale-rebase guard
kicks in if it was mid-rebase.)

### C — handcraft a recovery

Set up a sandbox where you've manually planted a `rebase-merge/` directory
in a worktree, then run `px resume` and verify the stale-rebase guard
actually aborts it (look for the "stale rebase detected in worktree" log
line).

## Symptom-to-action cheat sheet

| Symptom | First move | Second move |
|---|---|---|
| Respawn loop | `tmux capture-pane`, read transcript | `pkill`, fix code, `px gc --kill-tmux` |
| Stuck `in_progress`, sentinel present | `px resume` | check poller log |
| Review keeps rejecting | read `review_failed` payload | tighten requirement OR escalate |
| Rebase stuck | `git rebase --abort` in worktree | `rm -rf worktree && px resume` |
| Auto-merge stuck | `gh pr view <n>` (branch protection? CI gate?) | merge manually if safe |

Next: [[05-Cost-optimization]] — once recovery is muscle memory, learn to
not spend more than you need to in the first place.
