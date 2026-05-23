---
title: Lesson 05 · Cost optimization
tags: [px-dispatch, training, cost]
---

# Lesson 05 · Cost optimization

**Goal:** Run px on real work without surprise bills. ~20 minutes;
analysis-only, no spend.

## The shape of cost

For a typical 8-story requirement on Anthropic Sonnet:

| Stage | Calls | Tokens per call (input/output) | Typical $ |
|---|---|---|---|
| Planning (tech-lead) | 1 | 2k / 4k | $0.20 |
| Per-story agent work | ~12 (across 8 stories + retries) | 6k / 3k | $1.40 |
| Per-story review | ~10 (incl. retries) | 8k / 0.5k | $0.30 |
| Conflict resolution | 0-3 | 4k / 2k | $0.10 |
| Total | ~26 LLM calls | — | **~$2.00** |

That's per-token API. On Claude subscription (the `claude` CLI signed into
Claude Pro/Max), all these calls are $0 from px's perspective.

## Lever 1 — pick subscription where you can

```yaml
routing:
  preferences:
    - { role: junior, prefer: claude-code }       # subscription
    - { role: intermediate, prefer: claude-code }
    - { role: senior, prefer: claude-code }
    - { role: qa, prefer: claude-code }
```

The runtime adapter for `claude-code` runs the `claude` CLI under your
subscription. The ledger records the call but `cost_usd = 0.00`. Daily
budgets are about API spend; they don't gate subscription effort.

When subscription isn't an option (you've hit the daily message cap, or
the model isn't available on subscription), `codex` or `gemini` are
per-token but cheaper for small stories.

## Lever 2 — match model to role

```yaml
models:
  tech_lead:    { model: claude-opus-4-…    }   # most expensive, best reasoning
  senior:       { model: claude-sonnet-4-…  }
  intermediate: { model: claude-sonnet-4-…  }
  junior:       { model: claude-haiku-…     }   # cheap, fast, deterministic
  qa:           { model: claude-haiku-…     }   # mechanical check; haiku fine
  supervisor:   { model: claude-sonnet-4-…  }
```

The router picks based on story complexity:

- Complexity 1-3 → junior (haiku)
- Complexity 4-5 → intermediate (sonnet)
- Complexity 6+ → senior (sonnet/opus)

A mis-sized story (complexity 8 for "rename a variable") costs 3-5x more.
Push back on the tech-lead's complexity scoring during plan review.

## Lever 3 — budgets as safety nets

```yaml
budget:
  max_cost_per_day_usd: 10.0
  max_cost_per_requirement_usd: 5.0
  max_cost_per_story_usd: 2.0
  hard_stop: true
```

`BudgetBreaker` checks these BEFORE every LLM call. On exhaustion it returns
a `BudgetExhaustedError` classified as fatal — pipeline pauses, no further
spend. `hard_stop: false` would just emit a warning event and let the call
through (use only if you're actively watching).

The order of checks: story → requirement → daily. Story-level catches
runaway agents; daily catches misconfiguration.

## Lever 4 — short prompts where they help

The review stage prompt grows linearly with the diff size. A 5000-line
scaffold diff costs 50x more to review than a 100-line bugfix. The 10-minute
LLM timeout we added is partly cost protection — a runaway prompt won't
chew tokens for an hour.

For very large diffs, the deferred follow-up is per-file review. Until
then, **prefer requirements that produce many small stories over a few big
ones.** That's also a DDD virtue, so it aligns with the other levers.

## Lever 5 — turn off what you don't use

```yaml
fallback:
  enabled: false        # don't pay the per-call switching overhead
monitor:
  poll_interval_ms: 10000   # default is fine; lowering doesn't save money
pipeline:
  stages:
    review:
      max_retries: 1    # default 2; 1 retry is usually enough
```

## Reading the data

```bash
# Today's total
px cost

# Per-requirement breakdown
px cost <req-id>

# Raw ledger query (when you need a specific stage)
sqlite3 ~/.px/px.db '
  SELECT model, stage, COUNT(*) AS calls, SUM(input_tokens) AS in_tok, SUM(output_tokens) AS out_tok, ROUND(SUM(cost_usd), 4) AS usd
  FROM token_usage
  WHERE req_id = "01ABC..."
  GROUP BY model, stage
  ORDER BY usd DESC;
'
```

The most expensive stages are usually `review` (long prompts) and
`resolve_conflict` (multi-round, large context). If those dominate your
spend, look hard at requirement quality (Lesson 02) — most rejection cycles
come from spec ambiguity.

## Exercises

### A — predict before measuring

Pick an upcoming requirement. Estimate, in advance, how many LLM calls and
$USD it'll cost. Run it. Compare. Iterate until your prediction is within
20%. This is the single best skill for running large requirements without
budget anxiety.

### B — model substitution

Take a small requirement (3-4 stories) and run it twice in fresh sandboxes:
once with all roles on Sonnet, once with junior+qa on Haiku. Compare
- total cost
- total wall time
- review-cycle count

Often Haiku for junior+QA is 4x cheaper at the cost of one extra review
cycle on the trickier story. Worth it nine times out of ten.

### C — exhaust a budget on purpose

Set `max_cost_per_requirement_usd: 0.50` and run a real requirement. Watch
what happens when the breaker fires:
- which event appears in `events.jsonl`?
- does the pipeline pause cleanly or does it crash?
- when you raise the cap and `px resume`, does it pick up where it left off?

## Common mistakes

- **Sonnet everywhere "just to be safe"** — burns 3-5x what Haiku would.
- **No `hard_stop`** — runaway costs go unnoticed for hours.
- **Subscription on a shared machine** — the CLI uses whoever's signed in.
  If you're sharing a host, consider per-user state dirs.

Next: [[06-Adding-a-new-runtime]] — when none of the existing CLIs are
quite right for your work.
