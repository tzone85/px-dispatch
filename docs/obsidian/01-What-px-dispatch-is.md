---
title: What px-dispatch is
tags: [px-dispatch, overview]
---

# What px-dispatch is

px-dispatch (binary: `px`) is an autonomous coding team you can hand a
requirement to. It plans, dispatches AI agents, reviews their output, rebases,
and merges. You start it and walk away.

## The problem it solves

A single human developer-with-AI works one tool at a time, in one branch at a
time, with one context window. px-dispatch parallelises that work the way a
real team does: many agents, many worktrees, many tmux sessions, with a
tech-lead gate at planning and a code reviewer + QA gate at the back.

## What you give it

```bash
px plan requirement.txt          # natural-language description
px resume <req-id> --godmode     # let it run
```

The requirement can be greenfield (build a 4-in-a-row game) or revamp
(modernise a legacy React Native app). The
[[03-Agent-prompts-and-DDD-TDD-enforcement|tech-lead prompt]] handles both
via the `IsExistingCodebase` flag.

## What it gives back

- Merged PRs against `main` (or any base branch).
- An event log (`~/.px/events.jsonl`) — source of truth for every state
  transition. See [[02-Architecture-at-a-glance]] for the projection flow.
- A queryable cost ledger broken down by story / requirement / day.
- A clean workspace — worktrees, tmux sessions, and local `px/*` branches
  garbage-collected once a requirement completes. See
  [[09-Operating-the-system]].

## Key design choices

- **Event sourcing.** Every state change is append-only. SQLite projection
  is rebuildable from the JSONL log.
- **Pipeline-as-data.** Stages are values in `internal/pipeline.Stage`.
  Reorder or replace them at construction time.
- **Runtime adapters.** claude-code, codex, and gemini all implement
  `runtime.Runtime`. The router picks the cheapest one that satisfies the
  role's needs.
- **Filesystem sentinel for handoff.** The spawn script touches `.px-done`
  after the CLI exits. The poller checks for that sentinel so the pipeline
  triggers even if the tmux session has already ended. See
  [[04-Pipeline-stages-walkthrough]].

## When not to use it

- One-file fix — just use Claude Code directly.
- Throwaway script — overkill.

For a feature that touches five modules and needs tests, use `px`.
