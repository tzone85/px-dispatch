---
title: px-dispatch · Systematic Walkthrough
tags: [px-dispatch, index]
---

# px-dispatch · Systematic Walkthrough

**Repo:** [github.com/tzone85/px-dispatch](https://github.com/tzone85/px-dispatch)
**Local path:** `/Users/mncedimini/Sites/misc/project-x` (directory name still uses the old project-x label)

This vault is the top-to-bottom map of the system. Read in order, or jump.

## Reference path

1. [[01-What-px-dispatch-is]]
2. [[02-Architecture-at-a-glance]]
3. [[03-Agent-prompts-and-DDD-TDD-enforcement]]
4. [[04-Pipeline-stages-walkthrough]]
5. [[05-Conflict-resolution-and-rebase-guard]]
6. [[06-Cost-protection-and-budget-breakers]]
7. [[07-Runtime-adapters]]
8. [[08-Web-dashboard-and-API]]
9. [[09-Operating-the-system]]
10. [[10-Lessons-from-pilots]]
11. [[11-Open-questions]]

## Hands-on path

Eight lessons, ~15-45 min each. Start at the top if you're new to the system.

→ [[Training/00-Training-Index]]

1. [[Training/01-First-run-end-to-end]] — stand it up, watch a real PR land
2. [[Training/02-Writing-a-good-requirement]] — the single biggest lever on outcomes
3. [[Training/03-Reading-the-event-log]] — events.jsonl is the source of truth
4. [[Training/04-Debugging-a-stuck-story]] — diagnose and recover
5. [[Training/05-Cost-optimization]] — bills without surprises
6. [[Training/06-Adding-a-new-runtime]] — implement Runtime for a fourth CLI
7. [[Training/07-Tuning-the-review-stage]] — neither too strict nor too loose
8. [[Training/08-Self-recovery-patterns]] — what auto-recovers, what doesn't

## One-sentence mental model

`px` reads a requirement, asks an LLM tech-lead to decompose it into atomic
DDD-shaped stories, dispatches AI coding agents into isolated git worktrees,
and drives every story through a seven-stage pipeline until merged — with
cost budgets, health watchdogs, and a fire-and-forget cleanup at the end.

## Quick map

```
project-x/
├── cmd/px/                    # Cobra entry point
├── internal/
│   ├── agent/                 # SystemPrompt / GoalPrompt + diagnostic playbooks
│   ├── cli/                   # Cobra commands (plan, resume, dashboard, gc, …)
│   ├── config/                # px.yaml loader + Validate
│   ├── cost/                  # SQLiteLedger, budget breaker
│   ├── dashboard/             # Bubbletea TUI
│   ├── git/                   # CommandRunner, worktree, ops
│   ├── graph/                 # DAG + wave grouping
│   ├── llm/                   # Anthropic / Claude CLI / OpenAI / fallback chain
│   ├── logging/               # slog setup
│   ├── modelswitch/           # Approver interface for fallback prompts
│   ├── monitor/               # Dispatcher, Executor, Poller, Watchdog
│   ├── pipeline/              # autocommit → diffcheck → review → qa → rebase → merge → cleanup
│   ├── planner/               # LLM-driven decomposition + validation
│   ├── runtime/               # claude-code / codex / gemini tmux drivers
│   ├── state/                 # event store + SQLite projection
│   ├── tmux/                  # Session management primitives
│   └── web/                   # REST + SSE dashboard
├── docs/
│   ├── diagrams/              # rendered SVGs
│   ├── obsidian/              # ← you are here
│   │   └── training/          # ← hands-on lessons
│   └── superpowers/specs/     # canonical architecture references
└── px.yaml                    # config
```

## Cross-links

- Architecture deep-dive: [[02-Architecture-at-a-glance]] + the rendered SVGs in `docs/diagrams/`.
- For the canonical spec (PR-shaped, GitHub-rendered): [`docs/superpowers/specs/2026-05-22-architecture-reference.md`](../superpowers/specs/2026-05-22-architecture-reference.md).
- For the onboarding spec: [`docs/superpowers/specs/2026-05-22-onboarding.md`](../superpowers/specs/2026-05-22-onboarding.md).
- For shared learnings across sibling projects: `~/Sites/misc/SHARED_LEARNINGS.md`.
