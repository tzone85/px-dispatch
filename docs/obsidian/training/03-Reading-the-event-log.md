---
title: Lesson 03 · Reading the event log
tags: [px-dispatch, training, events]
---

# Lesson 03 · Reading the event log

**Goal:** Use `events.jsonl` and the SQLite projection to figure out exactly
what px is doing at any point in time. ~15 minutes; no LLM spend.

## Why this matters

The event log is the source of truth. The dashboard, the cost ledger, even
the stories table are projections of it. When something looks wrong, the
event log will always tell you what really happened.

## Two surfaces

- **`<state_dir>/events.jsonl`** — append-only JSON lines, one event per
  line. Strict ordering. Never rewritten.
- **`<state_dir>/px.db`** — SQLite projection. Rebuildable from the JSONL.
  Read-optimised for the dashboard and `px status` / `px events` commands.

## Hands-on

Reuse the sandbox from [[01-First-run-end-to-end]] or set up a tiny new one.
Use whichever requirement you have lying around — even one mid-run.

### 1. Count event types

```bash
jq -r '.type' "$SANDBOX-state/events.jsonl" | sort | uniq -c | sort -rn
```

Typical greenfield output:

```
  18 story.progress
   9 story.created
   9 story.started
   9 story.assigned
   9 story.merged
   3 story.review_passed
   3 story.review_failed
   1 req.submitted
   1 req.planned
   1 req.completed
```

You can already see the shape: 9 stories, 3 review re-runs (story 5 was
reviewed twice), one completed requirement.

### 2. Follow one story end-to-end

```bash
jq -r 'select(.story_id=="01ABC-s-3") | "\(.timestamp)  \(.type)  \(.agent_id)"' \
   "$SANDBOX-state/events.jsonl"
```

You should see the sequence:

```
2026-05-22T19:34:24Z  story.created     planner
2026-05-22T19:34:30Z  story.assigned    a-junior-3
2026-05-22T19:34:30Z  story.started     a-junior-3
2026-05-22T19:36:01Z  story.completed   a-junior-3
2026-05-22T19:36:05Z  story.review_passed monitor
2026-05-22T19:36:50Z  story.qa_passed   monitor
2026-05-22T19:37:30Z  story.pr_created  monitor
2026-05-22T19:38:00Z  story.merged      monitor
```

If a stage is missing, that's where the story is stuck.

### 3. Find the slowest story

```bash
jq -r 'select(.type=="story.created" or .type=="story.merged") | "\(.story_id) \(.type) \(.timestamp)"' \
   "$SANDBOX-state/events.jsonl" | sort
```

Pair created/merged by story_id and subtract the timestamps.

### 4. Inspect a single event's payload

```bash
jq 'select(.type=="story.review_failed") | .payload' "$SANDBOX-state/events.jsonl"
```

The reviewer's structured verdict (passed: false, summary, comments[]) is
right there. This is what feeds back into the next agent run as
`ReviewFeedback`.

### 5. Query the projection

```bash
sqlite3 "$SANDBOX-state/px.db" 'SELECT id, status FROM stories;'
sqlite3 "$SANDBOX-state/px.db" 'SELECT status, COUNT(*) FROM stories GROUP BY status;'
sqlite3 "$SANDBOX-state/px.db" 'SELECT * FROM agents;'
```

The projection always reflects the LAST event that affected each row. If a
story has bounced through review twice, you'll see `status='in_progress'`
in the projection even though the events log shows two completed cycles.

## Replay

Because the JSONL is authoritative, you can rebuild the projection from
scratch:

```bash
mv "$SANDBOX-state/px.db" "$SANDBOX-state/px.db.bak"
px --config "$SANDBOX/px.yaml" migrate  # creates an empty schema
# Today there's no `px replay` command (open question #7 in the vault),
# so you'd have to feed events through Project() manually if you ever
# need this. It's listed as a follow-up.
```

This is why every state change MUST go through `state.NewEvent(...)` → event
store → projector. Bypass the projector and your dashboard lies; bypass the
event store and there's nothing to replay from.

## Stream live

```bash
tail -F "$SANDBOX-state/events.jsonl" | jq -c '{ts: .timestamp, type, story: .story_id}'
```

Or use the dashboard SSE endpoint:

```bash
curl -N http://localhost:7890/api/stream
```

Same data; the SSE just JSON-deserialises and sends only the new lines.

## Common diagnostic snippets

```bash
# Last 20 events for a requirement
jq -r 'select(.payload.req_id=="01ABC...") | "\(.timestamp) \(.type)"' \
   "$SANDBOX-state/events.jsonl" | tail -20

# Agents that died vs spawned
jq -r 'select(.type=="agent.spawned" or .type=="agent.died") | "\(.type) \(.agent_id)"' \
   "$SANDBOX-state/events.jsonl" | sort | uniq -c

# Did anything cost money?
jq -r 'select(.type=="budget.warning" or .type=="budget.exhausted") | .payload' \
   "$SANDBOX-state/events.jsonl"
```

## Checks

- You can answer "where is story X right now?" without opening the dashboard.
- You can rebuild a guess at total wall time without timestamping anything
  yourself.
- You know which events do NOT produce projection updates (`story.progress`,
  `req.analyzed`, `story.estimated`, budget events) — see
  `internal/state/sqlite.go::Project`.

Next: [[04-Debugging-a-stuck-story]] — when the log shows something odd,
this is how you act on it.
