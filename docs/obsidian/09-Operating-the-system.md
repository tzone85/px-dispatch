---
title: Operating the system
tags: [project-x, ops, how-to]
---

# Operating the system

Practical guide to using `px` day-to-day.

## First-time setup

```bash
go install github.com/tzone85/project-x/cmd/px@latest
px migrate
px config show
```

Required externals: `tmux`, `git` ≥ 2.30, `gh`, `claude` CLI (default
runtime). Optional: `codex`, `gemini`.

## Writing a good requirement

The single biggest predictor of pilot success is requirement quality.

✅ A good requirement specifies:
- **Goal** — one sentence, no jargon.
- **Functional behaviour** — bullets, testable.
- **Code quality bar** — DDD layers, TDD, lint, tsconfig strict, coverage.
- **Owned files** — explicit list. Reviewer uses this for the
  [[04-Pipeline-stages-walkthrough|structural check]].
- **Acceptance criteria** — `npm run build` passes, `npm test` passes, etc.

❌ A bad requirement:
- "Make me a CRUD app"
- No quality bar
- No file list
- "Whatever you think is best"

## Starting a run

```bash
cd <target-repo>
px plan requirement.txt
px plan --review <req-id>
px resume <req-id> --godmode
```

`--godmode` allows `--dangerously-skip-permissions`. Sandboxes only.

## During a run

```bash
px dashboard --web              # → http://localhost:7890
px events --limit 200           # event stream
```

If a story seems stuck:

```bash
tmux ls
tmux capture-pane -t px-<story-id> -p -S -100
ls ~/.px/worktrees/<story-id>/.px-done
sqlite3 ~/.px/px.db 'select status, agent_id from stories where id="<sid>"'
```

## Symptom → first move

| Symptom | Likely cause | First move |
|---|---|---|
| Respawns every ~90s, no progress | Spawn script error → `.px-done` never landed | capture-pane |
| Stuck "in_progress", sentinel exists | Poller missed cycle | `px gc` then `px resume` |
| Rebase fails: "rebase-merge already exists" | Killed mid-rebase | Now auto-aborts. See [[05-Conflict-resolution-and-rebase-guard]] |
| Review keeps rejecting | Agent not reading spec | Sharpen OwnedFiles |
| Auto-merge stuck pending | Branch protection / CI gate | `gh pr view <n>` |

## Cleanup

```bash
px gc                  # remove worktrees for archived/completed reqs
px gc --prune          # also delete stale px/* branches
px gc --kill-tmux      # also kill px-* tmux sessions

rm -rf ~/.px && px migrate   # nuclear
```

When a requirement completes via `px resume`, `autoCleanupAfterCompletion`
runs automatically.

## Budget watching

```yaml
budget:
  max_cost_per_day_usd: 10.0
  max_cost_per_requirement_usd: 5.0
  max_cost_per_story_usd: 2.0
  hard_stop: true
```

`px cost` shows where you stand.
