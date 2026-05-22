# Project X — Onboarding

A new engineer should be able to clone the repo, follow this doc, and ship a
real change inside an afternoon. No tribal knowledge required.

---

## 1. Mental model in one paragraph

`px` is a **CLI orchestrator**. You hand it a natural-language requirement; it
asks an LLM to break that requirement into atomic stories, builds a dependency
DAG, groups stories into parallel **waves**, spawns one AI coding agent per
story inside an isolated git worktree, then drives every story through a
seven-stage pipeline (autocommit → diffcheck → review → QA → rebase → merge →
cleanup). State is event-sourced (JSONL append + SQLite projection). Live
status streams to a TUI **and** a browser dashboard. Cost is enforced before
every LLM call.

See [architecture reference](2026-05-22-architecture-reference.md) for the
deep dive, with rendered SVG diagrams.

---

## 2. Tools you need installed

| Tool | Why | Install |
|------|-----|---------|
| Go ≥ 1.22 | Build `px` (uses 1.22 method-based routing) | [go.dev/dl](https://go.dev/dl/) |
| CGO toolchain | `go-sqlite3` requires it | macOS: Xcode CLT; Linux: `build-essential` |
| `tmux` | Agent sessions live in tmux panes | `brew install tmux` / `apt install tmux` |
| `git` ≥ 2.30 | Worktrees + rebase | preinstalled on most systems |
| `gh` | PR create + auto-merge | `brew install gh` |
| `claude` CLI | Default runtime | [claude.ai/download](https://claude.ai/download) |

Optional: `codex`, `gemini` (alternate runtimes), `golangci-lint` (lint),
[VHS](https://github.com/charmbracelet/vhs) (demo gif regen).

---

## 3. First build, first test, first run

```bash
git clone https://github.com/tzone85/project-x.git
cd project-x

# 1. Build
make build              # outputs ./px

# 2. Test (race + coverage)
make test               # writes coverage.out

# 3. (Optional) Lint
make lint               # requires golangci-lint

# 4. Initialize state
./px migrate            # creates ~/.px/{events.jsonl,px.db,logs/}

# 5. Smoke-test config
./px config show
```

If `make test` passes, the wiring is intact. If `make build` works but
`make test` fails with `cgo: C compiler "gcc" not found`, install the CGO
toolchain. The `modernc.org/sqlite` migration is on the roadmap to remove
this requirement.

---

## 4. Run the dashboards before you write code

You can browse the system as it exists today without ever planning a
requirement:

```bash
./px dashboard            # TUI
./px dashboard --web      # browser at http://localhost:7890
```

The browser dashboard has seven tabs. The **About** tab loads from
`GET /api/about` and is the single source of truth for the project
description — if you change features, update `projectAbout` in
`internal/web/handlers.go` and the test will keep README and dashboard in
sync.

---

## 5. The shortest path to a real change

You're going to add a single new field to `aboutResponse`. This forces you to
touch:

- a Go handler
- a unit test
- the frontend SPA
- the README

…which is the exact set of places most real features touch.

1. **Read first.** Open `internal/web/handlers.go` and find `aboutResponse`.
   Notice it's a plain struct with JSON tags and that the canonical instance
   `projectAbout` is package-level.
2. **Write the test first** (`internal/web/handlers_test.go`). Add an
   assertion that `result.Stars > 0` (or whatever field you're adding). Run
   `go test ./internal/web/...` — it should fail with a clear "field
   undefined" compile error.
3. **Add the field** to `aboutResponse` and `projectAbout`. Re-run the test
   — it should pass.
4. **Wire the SPA** (`web/index.html` About panel + `web/app.js` if needed).
5. **Update README** if the field is user-visible.
6. **Re-run everything**: `make lint && make test && make build`.

This is the same loop for any internal change. The test-first step is not
ceremonial — Go's compile errors are precise, and writing the test first
keeps the public shape honest.

---

## 6. Map of "where do I touch X?"

| Want to change | Touch |
|----------------|-------|
| A CLI command | `internal/cli/<cmd>.go` |
| How stories are scored | `internal/agent/complexity.go` |
| How a pipeline stage behaves | `internal/pipeline/<stage>.go` + its `_test.go` |
| Add a runtime (CLI tool) | `internal/runtime/` — implement `Runtime`, register in `registry.go` |
| Change cost limits or breakers | `internal/cost/` + `px.config.example.yaml` |
| Web API endpoint | `internal/web/handlers.go` + route in `server.go` + test |
| Browser SPA | `web/index.html`, `web/app.js`, `web/style.css` (embedded via go:embed) |
| TUI panel | `internal/dashboard/` |
| State schema (new event type) | `internal/state/events.go` + `state/sqlite.go` |
| Architecture docs | `docs/superpowers/specs/2026-05-22-architecture-reference.md` + SVGs in `docs/diagrams/` |

---

## 7. Conventions worth knowing on day one

- **Immutability everywhere.** Functions return new structs; we don't mutate
  inputs. The event log relies on this.
- **Many small files.** 200–400 lines per file is the norm. If you're past
  800, refactor.
- **Tests live next to code.** `foo.go` + `foo_test.go` in the same package.
  End-to-end tests in `test/e2e/`.
- **No `console.log` / no `fmt.Println` in production code.** Use `slog`
  for backend, `console.error` only in the SPA's catch-all error paths.
- **Validate at boundaries.** User input, LLM output, API responses. Internal
  code is trusted.
- **`StagePassed` / `StageFailed` / `StageSkipped` are the *only* legal
  return values for a pipeline stage.** Add new ones in `pipeline/stage.go`
  if you genuinely need them.

---

## 8. Day-one TDD walkthrough (concrete)

Pick something tiny that's actually been on the list and ship it:

> "Show the number of pipeline stages in the About panel."

The number is already in `projectAbout.PipelineStages`. The work is:

1. `handlers_test.go` — assert `len(result.PipelineStages) == 7`. **Done
   already** — see `TestGetAbout`.
2. `index.html` — display `about.pipeline_stages.length` in the About header.
3. Re-render: `go run ./cmd/px dashboard --web`.

Total touched: 1 frontend file, 0 backend. Total test runs: 2 (red, green).
That's the rhythm.

---

## 9. Going further

- Read `docs/superpowers/specs/2026-05-22-architecture-reference.md` for the
  full deep dive.
- Read `docs/superpowers/specs/2026-03-20-project-x-v2-architecture-design.md`
  for the original v2 design decisions.
- Browse `internal/state/events.go` to see every event the system can emit.
- Run `./px events --limit 100` after `px resume` to watch the audit trail.
