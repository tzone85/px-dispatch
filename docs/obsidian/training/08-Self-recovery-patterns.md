---
title: Lesson 08 · Self-recovery patterns
tags: [px-dispatch, training, resilience]
---

# Lesson 08 · Self-recovery patterns

**Goal:** Understand what px does automatically when something goes wrong,
and where the gaps are. ~25 minutes; partly analysis, partly design.

This lesson is half tutorial, half open-question. The recovery story is
**incomplete by design** — full LLM-driven re-planning is a deferred
feature (see [[../11-Open-questions]] §"Resilience"). What's in place today
is a foundation; this lesson shows you what it covers and what it doesn't.

## What recovers automatically today

### Spawn-level

| Failure | Recovery |
|---|---|
| Spawn script errors before `.px-done` | Poller sees session missing without sentinel → emits `agent.lost` → wave dispatches a fresh agent |
| Claude/Codex/Gemini exits non-zero mid-run | Same as above; the sentinel is touched UNCONDITIONALLY after the CLI exits so the pipeline triggers either way |
| tmux session ends naturally (work done) | Sentinel triggers pipeline; no recovery needed |

### Pipeline-level

| Failure | Recovery |
|---|---|
| `StageFailed` from any stage | Retry up to `pipeline.stages.<stage>.max_retries` (default 2) |
| `max_retries` exhausted | `on_exhaust` policy: `pause` (default) or `escalate` |
| `StageFatal` (e.g., budget exhausted, fatal API error) | Requirement pauses immediately, escalation event emitted |
| Stale rebase from a killed previous run | `abortStaleRebase` runs before every rebase attempt |

### Workspace-level

| Failure | Recovery |
|---|---|
| All stories merge | `autoCleanupAfterCompletion` removes worktrees + tmux sessions |
| User runs `px gc` | Removes worktrees for archived/completed reqs; with `--prune` also stale branches |

## What does NOT recover automatically (yet)

### No tech-lead re-plan

When a story exhausts retries, the requirement pauses and emits an
`escalation` event. **There is no automatic loop that goes back to the
tech-lead with "this story is too big, break it down further" and
dispatches a fresh wave.** The escalation chain is logged, not driven.

The shape this would take:

```
story.review_failed (cycle 3)
  ↓
escalation.created (reason: "review_max_retries")
  ↓
[NEW] tech_lead.replan_requested  (with story context + feedback)
  ↓
[NEW] story.split_into(new_story_ids)
  ↓
wave dispatches the smaller stories
```

The mechanism doesn't exist yet. If you hit this state in practice, manual
intervention is needed:

1. Read the rejection reasons from the events log.
2. Either edit the story's description in `px.db` to scope it down, OR
   `px archive <req-id>` and `px plan` with a refined requirement.

### No stuck detection beyond output fingerprinting

The watchdog (`internal/monitor/watchdog.go`) compares pane output
fingerprints over time. Same hash for `StuckThresholdS` seconds → emit
`agent.stuck`. But it cannot tell:

- agent is genuinely thinking (no output yet, but working internally)
- agent crashed but the pane shows its last message (looks stuck)
- agent is waiting for user input that will never come

Heuristic improvements live in [[../11-Open-questions]].

### No automatic conflict avoidance

If two stories own overlapping files, the dispatcher will assign them to
the same wave (parallel) and the merger will see conflicts. The conflict
resolver handles them, but at LLM cost. The right fix is at planning time:
the tech-lead prompt could be tightened to refuse overlapping `OwnedFiles`.

Today, you can see this as: `OwnedFiles` overlap is a planning-quality
signal, not a runtime issue.

## Designing your own recovery

A few patterns worth experimenting with — all are local to `px.yaml`:

### Escalation = pause

```yaml
pipeline:
  stages:
    review:    { max_retries: 2, on_exhaust: pause }
    qa:        { max_retries: 3, on_exhaust: pause }
    rebase:    { max_retries: 1, on_exhaust: pause }
    merge:     { max_retries: 1, on_exhaust: escalate }   # bail to manual
```

`pause` lets you `px resume` after manual intervention. `escalate` (today
identical to pause + escalation event) lands a clear signal but doesn't
otherwise change behaviour.

### Aggressive retries on transient stages

```yaml
pipeline:
  stages:
    qa:        { max_retries: 5 }    # CI/network flakes
    rebase:    { max_retries: 1 }    # conflicts are real; don't loop
    review:    { max_retries: 3 }    # LLM noise; up to a point
```

### Hard caps via budget

If a runaway story is the symptom, `max_cost_per_story_usd` is the bound.
Pipeline pauses cleanly when the breaker fires.

## A worked example

Sandbox: hand px a requirement that's deliberately under-specified.

```
# Bad requirement on purpose
Build a chess engine.
```

Tech-lead plans ~12 stories (good). Each is vague. Junior agents produce
diffs that miss the spec because there's no spec. Reviewer rejects. Cycle.
Cycle. Pause.

**What you should observe:**

- After cycle 2 of story 1: `story.review_failed × 2`
- Then `story.paused` (synthesised from `on_exhaust`)
- Then `escalation.created`
- Then **nothing**. Px does not re-plan.

**What a fully autonomous system would do here:**

- Re-invoke the tech-lead with the rejection reasons:  
  *"Story s-1 failed review 2x. Reasons: X, Y, Z. Break it into smaller
  stories or rewrite the description."*
- Tech-lead returns new sub-stories (with smaller `OwnedFiles` each).
- Old story is archived; new sub-stories are dispatched.

This loop is the next major resilience feature. Today, you'd do it by hand:
read the rejections, edit the story, `px resume`.

## Exercises

### A — measure recovery rate

Across your last 10 runs, count:
- Total stories
- Stories that needed >1 attempt
- Stories that auto-recovered (≤max_retries cycles)
- Stories that required manual intervention

The recovery rate is the fraction that finished without you stepping in.
Above 90% = px is doing its job; below 70% = your specs need work.

### B — design the tech-lead replan loop

In a markdown doc, sketch:
- Where in the pipeline does the trigger fire?
- What context does the tech-lead need beyond the original requirement?
- How do new sub-stories get dependencies wired to existing completed
  stories?
- What protects against an infinite replan loop?

Submit your design as an issue / PR on the repo. This is real open
territory.

### C — escalation event handler

Today, `escalation.created` events are written but have no consumer beyond
the dashboard. Write a tiny external script that tails `events.jsonl` and
pages you (slack webhook, email, etc.) on every new escalation. This is
how you'd run px unattended over a weekend without flying blind.

## Where to go from here

You've reached the end of the training path. Where next:

- [[../09-Operating-the-system]] — keep this open during runs.
- [[../10-Lessons-from-pilots]] — append your own findings here as you run.
- [[../11-Open-questions]] — pick something and chip away at it.

The system is intentionally a workbench, not a finished product. Each
gap above is an invitation.
