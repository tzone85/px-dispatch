---
title: Training · px-dispatch
tags: [px-dispatch, training, index]
---

# Training · px-dispatch

Hands-on walkthroughs. Each lesson is a self-contained 15-30 minute exercise.
Do them in order if you're new; jump around if you have a specific need.

## Path

1. [[01-First-run-end-to-end]] — clone, configure, plan, resume, watch a merge land. 25 min.
2. [[02-Writing-a-good-requirement]] — worked examples of good vs bad requirements. 20 min.
3. [[03-Reading-the-event-log]] — `events.jsonl`, the projector, the dashboard. 15 min.
4. [[04-Debugging-a-stuck-story]] — diagnose + recover from agent-lost, review-failed, rebase-stuck. 30 min.
5. [[05-Cost-optimization]] — budgets, ledger, picking models per role. 20 min.
6. [[06-Adding-a-new-runtime]] — implement `runtime.Runtime` for a fourth CLI. 45 min.
7. [[07-Tuning-the-review-stage]] — when LLM judge rejects too aggressively or too loosely. 20 min.
8. [[08-Self-recovery-patterns]] — escalation events, breakdown-on-blocker (open territory). 25 min.

## How to use this

Each lesson has:
- **Goal** — what you'll be able to do at the end.
- **Setup** — exact commands to get a clean baseline.
- **Steps** — numbered, with expected output.
- **Checks** — assertions you can run to confirm you got it.
- **Variants** — what changes if you swap a setting / runtime / target.

Lessons share a common sandbox: `/tmp/px-train-<lesson>` so each can be wiped
and redone without disturbing your real work.

## Audience

- **Engineer onboarding** — go top to bottom over a half-day.
- **Operator** — lessons 3, 4, 5 are the day-to-day muscle.
- **Contributor** — lessons 6, 7, 8 are where the system's seams are.

See also: [[../09-Operating-the-system]] for ops reference (not training).
