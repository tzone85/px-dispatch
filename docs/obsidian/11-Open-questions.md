---
title: Open questions and future work
tags: [project-x, future]
---

# Open questions and future work

Honest list of what's not solved yet.

## Resilience

- **No tech-lead replan on persistent block.** Today, when a story
  exhausts retries, the requirement pauses and emits an `escalation`
  event. There's no automatic loop that goes back to the tech-lead with
  "this story is too big, break it down further" and dispatches a fresh
  wave. The escalation chain is logged, not driven.

- **Stuck detection is naive.** The watchdog looks for output-stable
  agents over a fixed window. Can't tell "agent is thinking" from
  "agent is hung."

## Cost

- **Subscription Claude pricing is `0.00` in the ledger.** Correct in the
  API-cost sense but misleading for wall-time efficiency. A "rough effort"
  metric (tokens × subscription proxy rate) would be honest.

- **No per-story projection of remaining budget.**

## Knowledge retrieval (MemPalace-lite)

- **No memory retrieval for agents.** `vortex-dispatch` has MemPalace
  (Python CLI vector-searching wing/room structure). Project X could
  expose this by reading SHARED_LEARNINGS.md and the obsidian vault as a
  context blob, but the right answer is the retrieval layer.

- **Where to bake it:** the agent's system prompt grows long. A pre-flight
  call that asks "give me 3 retrieved snippets relevant to this story"
  would keep prompts lean.

## UX

- **Plan review is read-only.** `px plan --review` shows the plan but
  can't be edited without re-running plan. `--refine` only takes new
  feedback. Story-level surgical edits (rename, re-split) would help.

- **No web-side plan editor.** The web dashboard is read-only.

## Security

- **Secret-detection is reactive.** The reviewer prompt mentions it, but
  there's no static pre-commit hook in the pipeline.

- **`--godmode` is global.** Should be per-runtime.

## DDD enforcement

- **No architectural fitness functions.** Prompt says "domain logic should
  not import frameworks." We don't check that. Simple `grep` rule in QA
  (`grep -r "from 'express'" src/domain/` → fail) would catch cheap
  violations.

## Tooling

- **No `px replay <req-id>` command.** Useful for debugging or re-running
  the planner step with the same input.

- **TUI isn't keyboard-accessible from start.** First keystroke must be
  tab/number; focus indicator could be louder.

## Cross-link

- [[10-Lessons-from-pilots]] — pilots feed this list.
- `SHARED_LEARNINGS.md` for items shared with sibling projects.
