# px-dispatch (px) — V2 Architecture Design Spec

**Date:** 2026-03-20
**Author:** Mncedimini + Claude
**Repo:** github.com/tzone85/px-dispatch
**Status:** Approved

## Overview

px-dispatch (`px`) is a clean-slate successor to Vortex Dispatch (VXD), an AI agent orchestration CLI that drives the full software development lifecycle — from natural-language requirements to merged PRs. px-dispatch takes VXD's proven architecture and addresses critical pain points around cost protection, session reliability, observability, pipeline resilience, and open-source readiness.

### Goals

- **Cost protection**: Never allow runaway API spend. Budget enforcement as a first-class concern.
- **Session reliability**: Detect and recover from stale/dead tmux sessions automatically.
- **Full observability**: Scrollable TUI dashboard + browser-based web dashboard with real-time updates.
- **Pipeline resilience**: Decomposed monitor with per-stage retry policies and clear failure modes.
- **Data layer correctness**: Proper migrations, indexes, async projections, typed payloads.
- **Planner intelligence**: Two-pass planning with validation to produce better stories.
- **Multi-runtime orchestration**: Clean runtime plugin SDK with smart cost-aware routing.
- **Open-source readiness**: Clean project structure, CI/CD, docs, packaging, contributor experience.

### Non-Goals

- Web-based agent IDE or code editor
- SaaS/hosted mode (px remains a local CLI tool)
- Custom LLM training or fine-tuning
- Support for non-CLI runtimes (e.g., API-only agents)
- VXD state/config migration (clean start; VXD users begin fresh)

---

## Cross-Cutting Concerns

### Concurrency & SQLite

SQLite operates in WAL mode with a single writer connection managed by the `ProjectionStore`. All writes go through this single connection — no connection pooling. The async projection goroutine (Section 5.4) is the sole writer; the poller, pipeline stages, and cost ledger enqueue events on the projection channel rather than writing directly. Dashboard and web reads use separate read-only connections (WAL mode allows concurrent readers). No external locking needed.

### Event Sourcing Model

The append-only JSONL file (`EventStore`) is the source of truth. SQLite projections are derived materialized views rebuilt by replaying events. On startup, if the SQLite projection is missing or corrupt, it can be rebuilt from the JSONL log. The JSONL store is append-only and never modified. The SQLite store is disposable and rebuildable.

### Graceful Shutdown

On SIGINT/SIGTERM:
1. Cancel the monitor context (stops polling, no new pipeline stages start).
2. Wait for in-flight pipeline stages to complete (with 30s timeout).
3. Drain the async projection channel fully.
4. Close cost ledger (flush pending writes).
5. Tmux sessions are NOT killed — they continue running. The user can reattach with `px resume`.
6. Close SQLite connections and web server.

### Structured Logging

All components use `slog` (Go's structured logging stdlib) with JSON output:
- Log levels: debug, info, warn, error (configurable via `workspace.log_level`).
- Each log entry includes: timestamp, level, component (monitor, pipeline, watchdog, etc.), story_id (when applicable).
- Output to: stderr (always), log file (`~/.px/logs/px.log`, rotated daily).
- The dashboard Logs panel reads from the same log file via tail-follow.

### Web Dashboard Security

The web server binds to `127.0.0.1` only (localhost). No authentication needed for local-only binding. If the user explicitly binds to `0.0.0.0` via `--bind`, a warning is printed. Future: optional bearer token auth for non-localhost binds.

### Model Pricing Table

Token cost computation uses a config-driven pricing table:
```yaml
pricing:
  anthropic/claude-opus-4-20250514: { input_per_1m: 15.00, output_per_1m: 75.00 }
  anthropic/claude-sonnet-4-20250514: { input_per_1m: 3.00, output_per_1m: 15.00 }
  openai/gpt-4o-mini: { input_per_1m: 0.15, output_per_1m: 0.60 }
```
Ships with defaults. Users override in config. `px cost --update-prices` fetches latest from a maintained JSON endpoint (future).

### Waves

A **wave** is a set of stories dispatched for parallel execution in a single batch. Stories within a wave have no dependencies on each other (all dependencies are on stories from prior waves). Wave boundaries are determined by topological sort of the story dependency DAG: wave 1 contains all root nodes, wave 2 contains nodes whose dependencies are all in wave 1, etc.

## Section 1: Cost Protection & Budget Enforcement

### Problem

No token budget enforcement exists in VXD. Agents stuck in loops burn API credits silently. The `IsFatalAPIError` check is reactive — it catches billing exhaustion after credits are gone.

### Design

#### 1.1 Cost Accounting Subsystem (`internal/cost/`)

- `Ledger` interface tracks token usage per story, per wave, per requirement.
- Every `CompletionResponse` extended with `InputTokens`, `OutputTokens`, `CostUSD` (computed from a model pricing table).
- Token usage persisted to a `token_usage` SQLite table.

#### 1.2 Budget Limits (config-driven)

```yaml
budget:
  max_cost_per_story_usd: 2.00
  max_cost_per_requirement_usd: 20.00
  max_cost_per_day_usd: 50.00
  warning_threshold_pct: 80
  hard_stop: true
```

#### 1.3 Circuit Breaker (`internal/cost/breaker.go`)

- Wraps any `llm.Client` with pre-call budget checks.
- Before each LLM call: check story budget, requirement budget, daily budget.
- On breach: return `BudgetExhaustedError` (non-retryable, triggers requirement pause).
- At 80% threshold: emit `EventBudgetWarning` for dashboard visibility.

#### 1.4 CLI Cost Tracking

- CLI-routed calls (Claude Code subscription): track estimated token counts for visibility and loop detection.
- `px cost` command: show spending by story, requirement, and day.

---

## Section 2: Tmux Session Health & Resilience

### Problem

Tmux sessions go stale silently. VXD's tmux package has no health checking — `SessionExists` only checks session name existence, not process liveness. Zombie sessions produce no output change and are misclassified as "stuck" rather than "dead."

### Design

#### 2.1 Session Health Monitor (`internal/tmux/health.go`)

- `SessionHealth(name) -> HealthStatus` checks:
  - Session exists (`tmux has-session`)
  - Pane process alive (`tmux list-panes -F "#{pane_pid} #{pane_dead}"`)
  - Process exit code if dead (`#{pane_dead_status}`)
  - Output flowing (last N bytes changed since last check)
- Returns `HealthStatus` enum: `Healthy`, `Stale`, `Dead`, `Missing`.

#### 2.2 Watchdog Integration

- Watchdog calls `SessionHealth` before fingerprinting.
- `Dead` → emit `EventAgentDied`, re-dispatch or escalate.
- `Stale` beyond threshold → emit `EventAgentStale`, attempt restart or kill-and-redispatch.
- `Missing` → emit `EventAgentLost`, clean up tracking state.

#### 2.3 Session Recovery (config-driven)

```yaml
sessions:
  stale_threshold_s: 180
  on_dead: redispatch
  on_stale: restart
  max_recovery_attempts: 2
```

#### 2.4 Dashboard Visibility

- Agent panel shows health status alongside agent status.
- Color-coded: green (healthy), yellow (stale), red (dead).

---

## Section 3: Dashboard Overhaul

### Problem

VXD's dashboard truncates content with "..." when stories exceed screen height. No scroll state, no keyboard navigation within panels. Story-to-agent mapping invisible when lists are long.

### Design

#### 3.1 Scrollable Viewports (Bubbletea `viewport` component)

- Each panel gets independent `viewport.Model` with scroll state.
- `j/k` or arrows scroll active panel, `g/G` jump top/bottom, Page Up/Down.
- Scroll indicator: `[3/15]` in panel border.

#### 3.2 Enhanced Pipeline Panel

- Stories grouped by requirement when multiple active.
- Story cards show: ID, title, complexity, agent, runtime, wave, elapsed time.
- Color-coded status progression.
- Paused stories highlighted in red with reason.

#### 3.3 Enhanced Agent Panel

- Tmux health status column.
- Token usage per agent.
- Current story title alongside ID.
- Live output preview (last line of tmux output).

#### 3.4 New Panels

- **Panel 5: Cost** — Real-time cost breakdown, budget bars.
- **Panel 6: Logs** — Streamed structured logs, filterable by component.

#### 3.5 Web Dashboard

**Stack:** Embedded HTTP server + Alpine.js + Tailwind CSS (zero external dependencies).

**API Layer** (`internal/web/`):
- REST endpoints: `/api/requirements`, `/api/stories`, `/api/agents`, `/api/events`, `/api/cost`, `/api/health`
- Server-Sent Events on `/api/stream` for real-time updates.
- Static assets embedded via `embed.FS`.

**Launch:**
```bash
px dashboard          # TUI (default)
px dashboard --web    # Browser on localhost:7890
px dashboard --web --port 8080
```

Both dashboards share the same Go data layer. Zero business logic duplication.

#### 3.6 Status Bar

- Active wave progress: `Wave 3: 4/7 stories merged`
- Budget usage: `$12.40 / $50.00 daily`
- Scroll position for active panel.

---

## Section 4: Monitor Decomposition & Pipeline Resilience

### Problem

VXD's `monitor.go` (755 lines) handles agent polling, event emission, review, QA, merge serialization, auto-resume, conflict resolution, retry logic, and requirement pausing in one struct. A bug here blocks all pipelines.

### Design

#### 4.1 Pipeline Stage Pattern (`internal/pipeline/`)

```go
type Stage interface {
    Name() string
    Execute(ctx context.Context, story StoryContext) (StageResult, error)
}
```

Stages: `AutoCommitStage`, `DiffCheckStage`, `ReviewStage`, `QAStage`, `RebaseStage`, `MergeStage`, `CleanupStage`.

Each returns `StageResult{Passed, Failed, Fatal}`:
- `Passed` — advance to the next stage.
- `Failed` — retry this stage (up to per-stage limit), then apply `on_exhaust` policy.
- `Fatal` — non-recoverable error; pause the requirement immediately.

#### 4.2 Pipeline Runner (`internal/pipeline/runner.go`)

- Executes stages sequentially per story.
- Per-stage retry budgets (configurable).
- Integrates with cost breaker — checks budget before each stage.
- `Fatal` → pause requirement. `Failed` → apply retry policy.

#### 4.3 Agent Poller (`internal/monitor/poller.go`)

Slim (~150 lines), single responsibility:
- Poll active sessions at intervals.
- Check watchdog/health.
- Agent finishes → hand off to pipeline runner.
- All done → trigger next wave dispatch.

#### 4.4 Per-Stage Retry Policy

```yaml
pipeline:
  stages:
    review:
      max_retries: 2
      on_exhaust: escalate  # re-assign story to a senior agent role with higher-capability model
    qa:
      max_retries: 3
      on_exhaust: pause_requirement
    rebase:
      max_retries: 2
      on_exhaust: pause_requirement
    merge:
      max_retries: 1
      on_exhaust: pause_requirement
```

---

## Section 5: State Layer Optimization & Schema Redesign

### Problem

SQLite layer has no indexes, ad-hoc migrations, synchronous projections blocking the event path, no pagination, and loose `map[string]any` payload extraction.

### Design

#### 5.1 Migration System

- Numbered SQL files in top-level `migrations/`: `001_init.sql`, `002_add_indexes.sql`, etc.
- Embedded into the Go binary via `embed.FS`.
- Migration runner (`internal/state/migrator.go`) tracks applied versions in a `schema_migrations` table.
- `px migrate` command, auto-run on startup.
- Forward-only migrations (no rollback — local CLI tool, keep it simple).

#### 5.2 Indexes

```sql
CREATE INDEX idx_stories_req_id ON stories(req_id);
CREATE INDEX idx_stories_status ON stories(status);
CREATE INDEX idx_stories_req_status ON stories(req_id, status);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_escalations_story_id ON escalations(story_id);
CREATE INDEX idx_token_usage_story_id ON token_usage(story_id);
CREATE INDEX idx_token_usage_req_id ON token_usage(req_id);
CREATE INDEX idx_token_usage_date ON token_usage(created_at);
```

#### 5.3 New Tables

- `token_usage` — cost tracking (req_id, story_id, agent_id, model, input/output tokens, cost_usd, stage)
- `session_health` — tmux health (session_name, status, pane_pid, last_output_hash, recovery_attempts)
- `pipeline_runs` — stage tracking (story_id, stage, status, attempt, error, timing)

#### 5.4 Async Projection

- Buffered channel between event append and projection.
- Background goroutine drains channel, applies projections in batches.
- Unblocks critical path. Drain fully on shutdown.

#### 5.5 Paginated Queries

- All `List*` methods accept `Limit` and `Offset`.
- Dashboard fetches page-sized chunks.
- Event stream uses cursor-based pagination.

#### 5.6 Typed Event Payloads

Each event type gets a payload struct (e.g., `StoryCreatedPayload`). Schema mismatches caught at decode time.

---

## Section 6: Planner Intelligence & Story Quality

### Problem

Planner sometimes produces poor decompositions — stories too large, vague, missing acceptance criteria, or nonsensical dependency chains.

### Design

#### 6.1 Two-Pass Planning

- Pass 1: Decomposition (Tech Lead LLM).
- Pass 2: Validation (separate LLM call reviews against quality criteria).
- If validation fails: feed critique back for second attempt.
- Max 2 rounds, then proceed with best plan + quality warnings.

#### 6.2 Story Quality Criteria

```yaml
planning:
  required_fields: [title, description, acceptance_criteria, owned_files, complexity, depends_on]
  max_story_complexity: 8
  max_stories_per_requirement: 15
  enforce_file_ownership: true
```

#### 6.3 Tech Stack Detection

Feed detected framework, language, test runner, linter, build tool, directory layout, and test patterns into planner prompt for context-aware decomposition.

#### 6.4 Plan-Then-Dispatch Workflow

```bash
px plan <requirement-file>     # plan only
px plan --review <req-id>      # inspect plan
px plan --refine <req-id>      # re-plan with feedback
px resume <req-id>             # dispatch approved plan
```

Separates planning from execution. Bad plans caught before agents spawn.

---

## Section 7: Runtime Plugin SDK & Multi-Runtime Orchestration

### Problem

VXD's runtime system uses shared regex-based detection for all runtimes. Adding a new runtime is fragile. No smart routing between runtimes.

### Design

#### 7.1 Runtime Interface

```go
type Runtime interface {
    Name() string
    Version() (string, error)
    Spawn(cfg SessionConfig) error
    Kill(sessionName string) error
    DetectStatus(sessionName string) (AgentStatus, error)
    ReadOutput(sessionName string, lines int) (string, error)
    Health(sessionName string) (HealthStatus, error)
    SendInput(sessionName string, input string) error
    Capabilities() RuntimeCapabilities
}

type RuntimeCapabilities struct {
    SupportsModel      []string
    SupportsGodmode    bool
    SupportsLogFile    bool
    SupportsJsonOutput bool
    MaxPromptLength    int
}
```

#### 7.2 Built-in Runtimes

- `ClaudeCodeRuntime` — Claude Code CLI
- `CodexRuntime` — OpenAI Codex CLI
- `GeminiRuntime` — Google Gemini CLI

Each encapsulates its own detection, argument formatting, and output parsing.

#### 7.3 Smart Runtime Router (`internal/runtime/router.go`)

- Cost-aware routing: prefer subscription-based runtimes (Claude Code CLI) over API-based clients.
- Capability matching: route large-context stories to runtimes that support them.
- Fallback chain: if preferred runtime is unhealthy, fall back to next available.

```yaml
routing:
  strategy: cost_optimized  # cost_optimized | performance
  preferences:
    - role: junior
      prefer: codex
      fallback: claude-code
    - role: senior
      prefer: claude-code
      fallback: gemini
```

Note: Direct Anthropic/OpenAI API calls are handled by the `llm` client layer, not the runtime layer. Runtimes are CLI tools that manage tmux sessions. The `llm` layer is used for planning, review, and conflict resolution — internal operations that don't need a tmux session.

#### 7.4 Runtime Health Integration

`Health()` feeds into session health system, dashboard, and router fallback logic.

---

## Section 8: Project Structure & Open-Source Readiness

### Project Layout

```
px-dispatch/
├── cmd/px/main.go
├── internal/
│   ├── agent/          # Roles, prompts, scoring
│   ├── cli/            # Cobra commands
│   ├── config/         # Config types, loading, validation
│   ├── cost/           # Ledger, breaker, pricing
│   ├── dashboard/      # Bubbletea TUI
│   ├── git/            # Git, worktree, GitHub ops
│   ├── graph/          # DAG, topological sort
│   ├── llm/            # Client interface + implementations
│   ├── monitor/        # Agent poller (slim)
│   ├── pipeline/       # Stage interface + implementations
│   ├── planner/        # Decomposition, validation
│   ├── runtime/        # Interface, router, built-in runtimes
│   ├── state/          # Event store, projections, migrations
│   ├── tmux/           # Session mgmt, health
│   └── web/            # HTTP server, SSE, embedded SPA
├── migrations/         # SQL files
├── web/                # Frontend assets (Alpine.js + Tailwind)
├── test/e2e/           # E2E tests
├── test/fixtures/      # LLM replay fixtures
├── docs/               # Architecture, contributing, config reference
├── .github/workflows/  # CI + release
├── Makefile
├── go.mod
├── px.config.example.yaml
├── README.md
├── LICENSE (Apache 2.0)
└── CHANGELOG.md
```

### Architectural Principles

- One responsibility per package.
- `internal/` prevents external import of unstable internals.
- Interfaces at package boundaries.
- No circular dependencies: `cli → monitor → pipeline → {runtime, llm, state, cost, git}`.
- Dependency injection everywhere.

### CI/CD

- `go vet`, `golangci-lint`, `go test -race`, 80% coverage gate.
- GoReleaser for cross-platform binary releases.
- Homebrew tap: `brew install tzone85/tap/px`.

### Open-Source Essentials

- README with quick start, demo GIF, architecture diagram, feature matrix.
- CONTRIBUTING guide with dev setup, code style, PR process.
- CHANGELOG in Keep a Changelog format.
- Issue templates: bug, feature, runtime plugin proposal.
- Well-commented example config.

---

## Dependency Graph

```
cli
 ├── monitor
 │    ├── pipeline
 │    │    ├── runtime (Stage execution uses runtimes)
 │    │    ├── llm     (Review, conflict resolution)
 │    │    ├── state   (Event emission, projections)
 │    │    ├── cost    (Budget checks per stage)
 │    │    └── git     (Rebase, merge, worktree)
 │    ├── tmux         (Session health)
 │    └── state        (Agent tracking)
 ├── planner
 │    ├── llm
 │    ├── state
 │    ├── graph
 │    └── git          (Tech stack detection)
 ├── dashboard
 │    └── state        (Read-only queries, including token_usage for cost panel)
 ├── web
 │    └── state        (Read-only queries + SSE, including token_usage for cost API)
 ├── config
 └── cost
      ├── llm          (Wraps clients)
      └── state        (Token usage persistence)
```

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Pipeline stage decomposition introduces regressions | Medium | High | Comprehensive tests for each stage; integration tests for full pipeline |
| Async projection loses events on crash | Low | High | Flush channel on graceful shutdown; JSONL is source of truth for replay |
| Web dashboard SSE connection instability | Medium | Low | Auto-reconnect on client side; TUI always available as fallback |
| LLM validation pass doubles planning cost | Medium | Low | Skip validation for simple requirements (<3 stories); budget-aware |
| Runtime plugin interface too rigid for new CLIs | Medium | Medium | Keep interface minimal; use `Capabilities()` for optional features |

---

## Success Criteria

1. Zero runaway cost incidents — budget breaker halts before overspend.
2. Stale tmux sessions detected within 2 poll cycles and automatically recovered.
3. Dashboard scrollable with full story-to-agent visibility regardless of pipeline size.
4. Monitor poller under 200 lines; each pipeline stage under 150 lines.
5. All queries use indexes; async projection doesn't block event emission.
6. Two-pass planning produces measurably better story quality (acceptance criteria present, complexity within bounds).
7. New runtime addable by implementing one interface — no config regex.
8. CI green, 80%+ coverage, installable via `go install` and Homebrew.
