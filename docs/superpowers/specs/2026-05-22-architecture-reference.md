# Project X — Architecture Reference

> Canonical architecture doc. The README gives the elevator pitch; this is for
> **engineers who need to understand or extend the system**. Diagrams are
> rendered SVG (see `docs/diagrams/`) so they read the same on GitHub, in
> `gh repo view`, and in any markdown viewer.

---

## 1. Component layout

![Project X component architecture](../../diagrams/architecture.svg)

Each box is a Go package under `internal/`. Arrows are in-process function
calls. There are **no circular dependencies** — every package interaction goes
through an interface defined in the consumer package.

| Layer | Package(s) | Responsibility |
|-------|------------|----------------|
| Entry | `cmd/px`, `internal/cli` | Cobra commands, config loading, lifecycle |
| Orchestration | `planner`, `monitor`, `pipeline`, `runtime`, `cost`, `tmux` | Decompose, dispatch, drive each story |
| External I/O | `llm`, `git`, `modelswitch`, `logging` | LLMs, git/gh, structured logs |
| State | `state` (FileStore + Projector + SQLiteStore) | Event sourcing + projections |
| Read side | `dashboard` (TUI), `web` (REST + SSE + embedded SPA) | Queryable views |

### Why event sourcing?

Every state mutation is appended to `~/.px/events.jsonl` as an immutable event.
SQLite projections are derived asynchronously by a `Projector` running on a
buffered channel.

- **Recoverable** — replay JSONL to rebuild `px.db`.
- **Auditable** — full history is preserved.
- **Concurrency-safe** — projections are written by a single goroutine.

Trade-off: queries must hit the projection store (`SQLiteStore.List*`), never
the event log directly.

---

## 2. Requirement → merged PR sequence

![Project X sequence diagram](../../diagrams/sequence.svg)

Worth noting:

1. **Planning is two-pass.** Tech-lead LLM proposes stories, then a validator
   checks acceptance criteria + DAG sanity before any agent spawns.
2. **Waves are derived, not stored.** `graph.GroupByWave(dag)` computes them
   from completed-story state on every dispatch, so hand-merged stories or
   restarts still produce a correct next wave.
3. **Merge is serialized.** A process-wide mutex wraps
   `rebase → push → gh pr merge` so every story sees the latest `main`.
4. **Events fire at every boundary** (green arrows). The web dashboard
   subscribes via `GET /api/stream` (SSE) and refreshes only when something
   changed.

---

## 3. Pipeline stages

`internal/pipeline` composes seven stages behind a common `Stage` interface:

```go
type Stage interface {
    Name() string
    Run(ctx context.Context, sc StoryContext) (StageResult, error)
}
```

| # | Stage | Purpose | Failure mode |
|---|-------|---------|--------------|
| 1 | `autocommit` | Stage + commit all uncommitted work in the worktree | `StageSkipped` if nothing to commit |
| 2 | `diffcheck` | Confirm the agent changed `OwnedFiles` | `StageFailed` if diff empty or out-of-scope |
| 3 | `review` | LLM judge: pass / changes-requested | Retries per `pipeline.stages.review` |
| 4 | `qa` | Detect tech stack → `go vet`, `go test`, npm, etc. | Retries per `pipeline.stages.qa` |
| 5 | `rebase` | `git fetch` + `git rebase origin/main`; LLM-resolve conflicts ≤10 rounds | `pause_requirement` on exhaust |
| 6 | `merge` | `git push` + `gh pr create` + `gh pr merge --auto` | Serialized via process mutex |
| 7 | `cleanup` | `git worktree remove`, branch delete | Best-effort; logs failures |

Retry/escalation policy is declarative in `pipeline.stages.*` in `px.yaml`.

---

## 4. State model

```
~/.px/
├── events.jsonl    # append-only, source of truth (28 event types)
├── px.db           # SQLite projections (queryable)
└── logs/
    └── px.log      # slog JSON, rotated daily
```

`px.db` tables (all derived from events):

- `requirements`, `stories`, `agents`, `escalations`
- `token_usage` (per LLM call: model, in/out tokens, USD cost)
- `pipeline_runs` (one row per stage execution)
- `session_health` (last-known tmux pane status)

The projector channel is sized 256 — short-lived back-pressure is fine, but a
sustained burst will block the event publisher (intentional: dropping events
would corrupt the audit trail).

---

## 5. Cost protection

Three layers, smallest blast radius first:

1. **Ledger** (`cost.Ledger`) — records every LLM call.
2. **Circuit breaker** (`cost.BudgetBreaker`) — wraps the LLM client. Checks
   budgets *before* every call. Returns `BudgetExhaustedError` (classified
   `IsFatalAPIError`) when over limit.
3. **CLI visibility** — `px cost`, `px cost <req-id>`, dashboard cost panel.

Budgets are per-story / per-requirement / per-day, configured in `px.yaml`.
The breaker hard-stops by default; soft-stop requires `hard_stop: false`.

---

## 6. Runtime abstraction

Any AI coding CLI plugs in via `runtime.Runtime`:

```go
type Runtime interface {
    Name() string
    Spawn(runner CommandRunner, cfg SessionConfig) error
    Kill(runner CommandRunner, sessionName string) error
    DetectStatus(runner CommandRunner, sessionName string) (AgentStatus, error)
    ReadOutput(runner CommandRunner, sessionName string, lines int) (string, error)
    SendInput(runner CommandRunner, sessionName string, input string) error
    Capabilities() RuntimeCapabilities
}
```

Built-in: `ClaudeCodeRuntime`, `CodexRuntime`, `GeminiRuntime`. The smart router
(`runtime.Router`) picks the cheapest runtime per role that still satisfies the
requested capabilities.

---

## 7. Web dashboard API

| Endpoint | Returns |
|----------|---------|
| `GET /api/about` | Project metadata (name, tagline, version, description, features) |
| `GET /api/health` | `{status, uptime}` |
| `GET /api/requirements` | Non-archived requirements |
| `GET /api/stories?req_id=…&status=…&limit=…&offset=…` | Stories |
| `GET /api/agents?status=…` | Agents |
| `GET /api/events?type=…&limit=…` | Recent events |
| `GET /api/escalations` | Open + resolved escalations |
| `GET /api/cost?req_id=…&story_id=…` | Daily / req / story cost in USD |
| `GET /api/stream` | Server-Sent Events stream |

Everything except `/api/stream` is plain JSON. The SPA is embedded via
`//go:embed`, so the binary is self-contained.

---

## 8. Extension points

If you're adding a feature, these are the doors:

- **New CLI command** → add a `newFooCmd()` in `internal/cli/` and register in
  `root.go`.
- **New pipeline stage** → implement `pipeline.Stage`, append it to the slice
  in the runner constructor.
- **New runtime** → implement `runtime.Runtime`, register in
  `runtime.NewRegistry()`.
- **New API endpoint** → add a handler in `internal/web/handlers.go`, route in
  `server.go`, test in `handlers_test.go`. Update `aboutResponse` if it
  affects the project description.
- **New event type** → constant in `state/events.go`, payload struct, and
  projection logic in `state/sqlite.go`. Bump the projection schema test.

See the onboarding spec next:
[2026-05-22-onboarding.md](2026-05-22-onboarding.md).
