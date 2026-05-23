---
title: Lesson 01 · First run end-to-end
tags: [px-dispatch, training, getting-started]
---

# Lesson 01 · First run end-to-end

**Goal:** Stand up px-dispatch from scratch, hand it a tiny requirement, and
watch a real PR land. ~25 minutes wall-clock; ~$0.50 LLM spend if you're on
the subscription tier (free).

## Prerequisites

```bash
command -v tmux git gh claude   # all four must resolve
```

Plus a clean GitHub account you can create a throwaway repo against.

## Setup

```bash
# Sandbox dir.
export SANDBOX=/tmp/px-train-01
rm -rf "$SANDBOX" && mkdir -p "$SANDBOX" && cd "$SANDBOX"

# Bootstrap a private throwaway repo + push an empty README.
gh repo create tzone85/px-train-01 --private --description "px-dispatch training lesson 01"
git init -b main
git remote add origin https://github.com/tzone85/px-train-01.git
echo "# px-train-01" > README.md
git -c user.email=you@example.com -c user.name=You add README.md
git -c user.email=you@example.com -c user.name=You commit -q -m "chore: init"
git push -u origin main
```

## Step 1 — drop a `px.yaml`

```yaml
# /tmp/px-train-01/px.yaml
version: "1"
workspace:
  state_dir: "/tmp/px-train-01-state"
  backend: sqlite
  log_level: info
models:
  tech_lead:    { provider: anthropic, model: claude-sonnet-4-20250514, max_tokens: 16384 }
  senior:       { provider: anthropic, model: claude-sonnet-4-20250514, max_tokens: 16384 }
  intermediate: { provider: anthropic, model: claude-sonnet-4-20250514, max_tokens: 16384 }
  junior:       { provider: anthropic, model: claude-sonnet-4-20250514, max_tokens:  8192 }
  qa:           { provider: anthropic, model: claude-sonnet-4-20250514, max_tokens: 16384 }
  supervisor:   { provider: anthropic, model: claude-sonnet-4-20250514, max_tokens: 16384 }
routing:
  junior_max_complexity: 3
  intermediate_max_complexity: 5
  preferences:
    - { role: junior, prefer: claude-code }
    - { role: intermediate, prefer: claude-code }
    - { role: senior, prefer: claude-code }
    - { role: qa, prefer: claude-code }
merge: { auto_merge: true, base_branch: main }
fallback: { enabled: false }
budget:
  max_cost_per_day_usd: 5.0
  max_cost_per_requirement_usd: 2.0
  max_cost_per_story_usd: 1.0
  hard_stop: true
```

Tech-lead uses Claude CLI under your subscription, so the cost ledger will
record `$0.00` — the budget caps are a safety net only.

## Step 2 — write the requirement

```bash
cat > requirement.txt <<'REQ'
# Add a /version endpoint

Add a tiny Go HTTP server in main.go that exposes GET /version returning
JSON {"version": runtime.Version()} on port 8080. Other paths return 404.

## Owned files
- main.go
- main_test.go
- go.mod

## Acceptance criteria
- `go build ./...` succeeds.
- `go test ./...` passes.
- Server starts, hitting /version returns 200 + matching JSON shape.
REQ
```

## Step 3 — plan

```bash
px --config px.yaml migrate          # creates state DB on first run
px --config px.yaml plan requirement.txt
```

Expected output: a `Requirement: <ulid>` line and 2-3 stories — probably a
"Define module structure", "Implement /version handler", "Add table-driven
test" sequence.

## Step 4 — resume (the long-running step)

```bash
px --config px.yaml resume <REQ_ID> --godmode
```

`--godmode` lets the runtime use `--dangerously-skip-permissions`. Sandboxes
only — never on your day-job repo.

While it runs (5-15 min), open the dashboard in another terminal:

```bash
px --config px.yaml dashboard --web   # http://localhost:7890
```

The pipeline kanban will show stories sliding right: `planned` →
`in_progress` → `review` → `qa` → `merged`. When the last one merges,
`runResume` prints:

```
All N stories complete! Requirement <id> is done.
Cleanup complete: worktrees + tmux sessions for completed stories removed.
```

## Checks

```bash
# 1. The PR landed
gh pr list --repo tzone85/px-train-01 --state merged

# 2. The code actually does what was asked
cd /tmp/px-train-01
git pull
go build ./... && go test ./...
go run . &
sleep 1
curl -s http://localhost:8080/version
kill %1

# 3. Workspace is clean
ls /tmp/px-train-01-state/worktrees/   # empty
tmux ls 2>&1                            # no px-* sessions
```

## Variants

- **Different runtime:** set `routing.preferences[].prefer: codex` (needs
  `codex` CLI on PATH and an OpenAI key). Cost ledger will start showing
  non-zero figures.
- **Auto-merge off:** set `merge.auto_merge: false`. px creates the PR but
  leaves merging to you — useful when you're learning and want to inspect
  diffs.
- **Smaller stories:** lower `routing.intermediate_max_complexity` to 4 to
  push more work to the junior tier (faster, cheaper, less polished).

## Cleanup

```bash
rm -rf /tmp/px-train-01 /tmp/px-train-01-state
gh repo delete tzone85/px-train-01 --yes
```

Next: [[02-Writing-a-good-requirement]] — once you've seen one work, learn
what makes them work.
