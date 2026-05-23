# px-dispatch (px) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build px-dispatch (`px`), a production-grade CLI that orchestrates autonomous AI agents through the full SDLC — from natural-language requirements to merged PRs — with cost protection, session resilience, multi-runtime support, and full observability.

**Architecture:** Event-sourced state (append-only JSONL + SQLite projections), pipeline-stage pattern for post-execution processing, DAG-based wave dispatch, pluggable runtime SDK for AI CLIs (Claude Code, Codex, Gemini), with both TUI and browser-based dashboards.

**Tech Stack:** Go 1.22+, SQLite (WAL mode), Cobra (CLI), Bubbletea (TUI), Alpine.js + Tailwind (web dashboard), slog (structured logging), tmux (session management).

**Spec:** `docs/superpowers/specs/2026-03-20-px-dispatch-v2-architecture-design.md`

---

## File Structure

```
px-dispatch/
├── cmd/px/
│   └── main.go                       # Entry point, wires Cobra root
├── internal/
│   ├── agent/
│   │   ├── roles.go                  # Role enum, complexity routing, ModelConfig lookup
│   │   ├── roles_test.go
│   │   ├── prompts.go                # System/goal prompt generation per role
│   │   └── prompts_test.go
│   ├── cli/
│   │   ├── root.go                   # Cobra root command, global flags, config loading
│   │   ├── plan.go                   # px plan <file>, px plan --review, px plan --refine
│   │   ├── resume.go                 # px resume <req-id>
│   │   ├── status.go                 # px status [req-id]
│   │   ├── cost.go                   # px cost [req-id]
│   │   ├── dashboard.go              # px dashboard [--web] [--port]
│   │   ├── agents.go                 # px agents
│   │   ├── events.go                 # px events [--limit]
│   │   ├── config_cmd.go             # px config show/validate
│   │   ├── migrate.go                # px migrate
│   │   ├── gc.go                     # px gc (garbage collect)
│   │   └── archive.go                # px archive <req-id>
│   ├── config/
│   │   ├── config.go                 # All config types (Config, BudgetConfig, etc.)
│   │   ├── config_test.go            # Validation tests
│   │   ├── loader.go                 # YAML loading, defaults, env expansion
│   │   └── loader_test.go
│   ├── cost/
│   │   ├── ledger.go                 # Ledger interface + SQLite implementation
│   │   ├── ledger_test.go
│   │   ├── breaker.go                # Circuit breaker wrapping llm.Client
│   │   ├── breaker_test.go
│   │   └── pricing.go               # Model pricing table + cost computation
│   ├── dashboard/
│   │   ├── app.go                    # Bubbletea Model, Init, Update, View
│   │   ├── viewport.go              # Scrollable viewport wrapper per panel
│   │   ├── pipeline.go              # Pipeline panel (kanban columns)
│   │   ├── agents.go                # Agent status panel
│   │   ├── activity.go              # Event activity panel
│   │   ├── escalations.go           # Escalations panel
│   │   ├── cost_panel.go            # Cost breakdown panel
│   │   ├── logs_panel.go            # Structured log viewer panel
│   │   └── styles.go                # lipgloss styles
│   ├── git/
│   │   ├── ops.go                    # fetch, rebase, diff, merge-base, branch ops
│   │   ├── ops_test.go
│   │   ├── worktree.go              # Create/remove worktrees
│   │   ├── worktree_test.go
│   │   ├── github.go                # PR create, merge, auto-merge (via gh CLI)
│   │   ├── github_test.go
│   │   └── scan.go                  # Tech stack detection via marker files
│   ├── graph/
│   │   ├── dag.go                    # DAG struct, AddNode, AddEdge
│   │   ├── dag_test.go
│   │   └── topo.go                  # Topological sort, ReadyNodes, wave grouping
│   ├── llm/
│   │   ├── client.go                # Client interface, CompletionRequest/Response, Message
│   │   ├── claude_cli.go            # Claude Code CLI client (stdin piping)
│   │   ├── claude_cli_test.go
│   │   ├── anthropic.go             # Direct Anthropic API client
│   │   ├── anthropic_test.go
│   │   ├── openai.go                # OpenAI API client
│   │   ├── openai_test.go
│   │   ├── retry.go                 # Retry wrapper with exponential backoff
│   │   ├── retry_test.go
│   │   ├── replay.go                # Deterministic test fixture client
│   │   └── errors.go                # APIError, IsFatalAPIError, BudgetExhaustedError
│   ├── monitor/
│   │   ├── poller.go                # Slim agent poller (~150 lines)
│   │   ├── poller_test.go
│   │   ├── dispatcher.go            # Wave dispatch from DAG
│   │   ├── dispatcher_test.go
│   │   ├── executor.go              # Worktree creation, runtime spawn
│   │   ├── watchdog.go              # Stuck detection, permission bypass
│   │   └── watchdog_test.go
│   ├── pipeline/
│   │   ├── stage.go                 # Stage interface, StageResult, StoryContext
│   │   ├── runner.go                # Sequential stage executor with retry policy
│   │   ├── runner_test.go
│   │   ├── autocommit.go           # Auto-commit uncommitted agent work
│   │   ├── diffcheck.go            # Verify agent produced real changes
│   │   ├── review.go               # Code review stage (LLM-based)
│   │   ├── review_test.go
│   │   ├── qa.go                    # Lint/build/test pipeline
│   │   ├── qa_test.go
│   │   ├── rebase.go               # Rebase with conflict resolution
│   │   ├── rebase_test.go
│   │   ├── merge.go                # Push, PR, auto-merge
│   │   ├── merge_test.go
│   │   └── cleanup.go              # Worktree/branch pruning
│   ├── planner/
│   │   ├── planner.go              # Two-pass requirement decomposition
│   │   ├── planner_test.go
│   │   ├── validator.go            # Plan quality validation (pass 2)
│   │   ├── validator_test.go
│   │   └── techstack.go            # Enhanced tech stack detection for prompts
│   ├── runtime/
│   │   ├── runtime.go              # Runtime interface, Capabilities, SessionConfig
│   │   ├── registry.go             # Runtime registry (name -> Runtime)
│   │   ├── registry_test.go
│   │   ├── router.go               # Cost-aware runtime selection
│   │   ├── router_test.go
│   │   ├── claude.go               # ClaudeCodeRuntime implementation
│   │   ├── claude_test.go
│   │   ├── codex.go                # CodexRuntime implementation
│   │   └── gemini.go               # GeminiRuntime implementation
│   ├── state/
│   │   ├── models.go               # Domain models: Requirement, Story, Agent, etc.
│   │   ├── events.go               # Event struct, EventType constants, typed payloads
│   │   ├── events_test.go
│   │   ├── store.go                # EventStore + ProjectionStore interfaces
│   │   ├── filestore.go            # Append-only JSONL event store
│   │   ├── filestore_test.go
│   │   ├── sqlite.go               # SQLite ProjectionStore implementation
│   │   ├── sqlite_test.go
│   │   ├── projector.go            # Async projection goroutine (channel-based)
│   │   ├── projector_test.go
│   │   ├── migrator.go             # Forward-only migration runner
│   │   └── migrator_test.go
│   ├── tmux/
│   │   ├── session.go              # Create, kill, list, send-keys, read-output
│   │   ├── session_test.go
│   │   ├── health.go               # SessionHealth, HealthStatus enum
│   │   └── health_test.go
│   └── web/
│       ├── server.go               # HTTP server, route setup, localhost binding
│       ├── server_test.go
│       ├── handlers.go             # REST API handlers
│       ├── handlers_test.go
│       ├── sse.go                  # Server-Sent Events stream
│       └── embed.go               # embed.FS for static assets
├── migrations/
│   ├── 001_init.sql                # Core tables
│   ├── 002_indexes.sql             # Performance indexes
│   ├── 003_token_usage.sql         # Cost tracking table
│   ├── 004_session_health.sql      # Tmux health tracking
│   └── 005_pipeline_runs.sql       # Pipeline stage tracking
├── web/
│   ├── index.html                  # SPA shell
│   ├── app.js                      # Alpine.js app
│   └── style.css                   # Tailwind CSS
├── test/
│   ├── e2e/
│   │   └── pipeline_test.go        # Full pipeline integration test
│   └── fixtures/
│       └── replay/                 # LLM replay JSON fixtures
├── go.mod
├── Makefile
├── px.config.example.yaml
├── .goreleaser.yml
├── .github/workflows/
│   ├── ci.yml
│   └── release.yml
├── README.md
├── LICENSE
└── CHANGELOG.md
```

---

## Phase 1: Foundation & State Layer

Everything depends on this. Produces: a `px` CLI that loads config, runs migrations, and can store/query events and projections.

### Task 1.1: Go Module & Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `cmd/px/main.go`
- Create: `internal/cli/root.go`
- Create: `Makefile`
- Create: `LICENSE`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/mncedimini/Sites/hackathon
go mod init github.com/tzone85/px-dispatch
```

- [ ] **Step 2: Create entry point**

Create `cmd/px/main.go`:
```go
package main

import (
	"os"

	"github.com/tzone85/px-dispatch/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Create root command**

Create `internal/cli/root.go`:
```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	version = "dev"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "px",
		Short: "px-dispatch — AI agent orchestration for the full SDLC",
		Long:  "Orchestrate autonomous AI agents from requirements to merged PRs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./px.yaml)")
	cmd.AddCommand(newVersionCmd())
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("px %s\n", version)
		},
	}
}

func Execute() error {
	return NewRootCmd().Execute()
}
```

- [ ] **Step 4: Create Makefile**

Create `Makefile`:
```makefile
BINARY := px
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/tzone85/px-dispatch/internal/cli.version=$(VERSION)"

.PHONY: build test lint clean install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/px

test:
	go test ./... -race -coverprofile=coverage.out

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY) coverage.out

install: build
	cp $(BINARY) $(GOPATH)/bin/
```

- [ ] **Step 5: Create LICENSE (Apache 2.0)**

Create `LICENSE` with Apache 2.0 license text, copyright `2026 tzone85`.

- [ ] **Step 6: Install Cobra dependency and verify build**

```bash
go get github.com/spf13/cobra
go build ./cmd/px
./px version
```

Expected: `px dev`

- [ ] **Step 7: Commit**

```bash
git add cmd/ internal/cli/ go.mod go.sum Makefile LICENSE
git commit -m "feat: project scaffold with cobra CLI and build system"
```

---

### Task 1.2: Configuration System

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/config/loader.go`
- Create: `internal/config/loader_test.go`
- Create: `px.config.example.yaml`

- [ ] **Step 1: Write config validation tests**

Create `internal/config/config_test.go`:
```go
package config

import "testing"

func TestValidate_ValidConfig(t *testing.T) {
	cfg := Defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidate_InvalidBackend(t *testing.T) {
	cfg := Defaults()
	cfg.Workspace.Backend = "mysql"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid backend")
	}
}

func TestValidate_JuniorComplexityTooHigh(t *testing.T) {
	cfg := Defaults()
	cfg.Routing.JuniorMaxComplexity = 20
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for out-of-range complexity")
	}
}

func TestValidate_IntermediateBelowJunior(t *testing.T) {
	cfg := Defaults()
	cfg.Routing.JuniorMaxComplexity = 5
	cfg.Routing.IntermediateMaxComplexity = 3
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when intermediate < junior")
	}
}

func TestValidate_BudgetLimits(t *testing.T) {
	cfg := Defaults()
	cfg.Budget.MaxCostPerStoryUSD = -1.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative budget")
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
go test ./internal/config/... -v
```

Expected: FAIL — `config` package doesn't exist yet.

- [ ] **Step 3: Write Config types and Defaults**

Create `internal/config/config.go` with all config structs from the spec:
- `Config` (top-level)
- `WorkspaceConfig`, `ModelsConfig`, `ModelConfig`
- `RoutingConfig`, `MonitorConfig`, `CleanupConfig`, `MergeConfig`
- `PlanningConfig`, `BudgetConfig`, `PricingEntry`
- `SessionsConfig`, `PipelineConfig`, `StageConfig`
- `RuntimeConfig`, `RuntimeDetection`
- `Defaults()` function returning sensible defaults
- `Validate()` method checking all constraints

Key defaults:
```go
func Defaults() Config {
	return Config{
		Version: "1",
		Workspace: WorkspaceConfig{
			StateDir: "~/.px",
			Backend:  "sqlite",
			LogLevel: "info",
		},
		Budget: BudgetConfig{
			MaxCostPerStoryUSD:       2.0,
			MaxCostPerRequirementUSD: 20.0,
			MaxCostPerDayUSD:         50.0,
			WarningThresholdPct:      80,
			HardStop:                 true,
		},
		Routing: RoutingConfig{
			JuniorMaxComplexity:        3,
			IntermediateMaxComplexity:  5,
			MaxRetriesBeforeEscalation: 3,
		},
		Monitor: MonitorConfig{
			PollIntervalMs:         10000,
			StuckThresholdS:        120,
			ContextFreshnessTokens: 150000,
		},
		Sessions: SessionsConfig{
			StaleThresholdS:     180,
			OnDead:              "redispatch",
			OnStale:             "restart",
			MaxRecoveryAttempts: 2,
		},
		Merge: MergeConfig{
			AutoMerge:  true,
			BaseBranch: "main",
		},
		Cleanup: CleanupConfig{
			WorktreePrune:       "immediate",
			BranchRetentionDays: 7,
		},
		Planning: PlanningConfig{
			MaxStoryComplexity:       8,
			MaxStoriesPerRequirement: 15,
			EnforceFileOwnership:     true,
		},
	}
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test ./internal/config/... -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Write loader tests**

Create `internal/config/loader_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("expected defaults when no file: %v", err)
	}
	if cfg.Workspace.Backend != "sqlite" {
		t.Errorf("expected sqlite backend, got %q", cfg.Workspace.Backend)
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "px.yaml")
	os.WriteFile(path, []byte(`
workspace:
  state_dir: /tmp/px-test
  backend: sqlite
  log_level: debug
routing:
  junior_max_complexity: 2
  intermediate_max_complexity: 5
`), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if cfg.Workspace.LogLevel != "debug" {
		t.Errorf("expected debug log level, got %q", cfg.Workspace.LogLevel)
	}
	if cfg.Routing.JuniorMaxComplexity != 2 {
		t.Errorf("expected junior complexity 2, got %d", cfg.Routing.JuniorMaxComplexity)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "px.yaml")
	os.WriteFile(path, []byte(`{invalid yaml`), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
```

- [ ] **Step 6: Implement loader**

Create `internal/config/loader.go`:
- `Load(path string) (Config, error)` — loads YAML, merges over defaults, validates
- `FindConfigFile()` — searches `./px.yaml`, `./px.config.yaml`, `~/.px/config.yaml`
- `expandHome(path string) string` — replaces `~` with home dir

- [ ] **Step 7: Run all config tests**

```bash
go test ./internal/config/... -v
```

Expected: all tests PASS.

- [ ] **Step 8: Create example config**

Create `px.config.example.yaml` with all options documented inline with comments.

- [ ] **Step 9: Commit**

```bash
git add internal/config/ px.config.example.yaml
git commit -m "feat: configuration system with types, defaults, validation, and YAML loader"
```

---

### Task 1.3: Domain Models & Event Types

**Files:**
- Create: `internal/state/models.go`
- Create: `internal/state/events.go`
- Create: `internal/state/events_test.go`
- Create: `internal/state/store.go`

- [ ] **Step 1: Write event creation tests**

Create `internal/state/events_test.go`:
```go
package state

import (
	"encoding/json"
	"testing"
)

func TestNewEvent_HasULID(t *testing.T) {
	evt := NewEvent(EventReqSubmitted, "agent-1", "", map[string]any{"id": "req-1"})
	if evt.ID == "" {
		t.Fatal("event ID should not be empty")
	}
	if evt.Type != EventReqSubmitted {
		t.Errorf("expected %s, got %s", EventReqSubmitted, evt.Type)
	}
}

func TestNewEvent_PayloadSerialization(t *testing.T) {
	payload := map[string]any{"id": "story-1", "complexity": 5}
	evt := NewEvent(EventStoryCreated, "planner", "story-1", payload)

	var decoded map[string]any
	if err := json.Unmarshal(evt.Payload, &decoded); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if decoded["id"] != "story-1" {
		t.Errorf("expected story-1, got %v", decoded["id"])
	}
}

func TestTypedPayload_StoryCreated(t *testing.T) {
	p := StoryCreatedPayload{
		ID:         "s-1",
		ReqID:      "r-1",
		Title:      "Add login",
		Complexity: 3,
		OwnedFiles: []string{"auth.go"},
		WaveHint:   "parallel",
		DependsOn:  []string{},
	}
	evt, err := NewTypedEvent(EventStoryCreated, "planner", "s-1", p)
	if err != nil {
		t.Fatalf("typed event error: %v", err)
	}

	var decoded StoryCreatedPayload
	if err := json.Unmarshal(evt.Payload, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.Title != "Add login" {
		t.Errorf("expected 'Add login', got %q", decoded.Title)
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement domain models** (`internal/state/models.go`) — Requirement, Story, Agent, Escalation, StoryDep, and all filter types with Limit/Offset pagination fields.

- [ ] **Step 4: Implement event types and typed payloads** (`internal/state/events.go`) — EventType constants, Event struct with ULID generation, NewEvent, NewTypedEvent, all typed payload structs.

- [ ] **Step 5: Implement store interfaces** (`internal/state/store.go`) — EventStore and ProjectionStore interfaces.

- [ ] **Step 6: Run tests — verify they pass**

- [ ] **Step 7: Commit**

```bash
git add internal/state/
git commit -m "feat: domain models, event types with typed payloads, and store interfaces"
```

---

### Task 1.4: JSONL Event Store (FileStore)

**Files:**
- Create: `internal/state/filestore.go`
- Create: `internal/state/filestore_test.go`

- [ ] **Step 1: Write FileStore tests** — Append/List, FilterByType, Count, Limit, PersistsToDisk (reopen from file).

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement FileStore** — JSON-encode per line, append to file, load into memory on open, filter in memory, thread-safe via `sync.Mutex`.

- [ ] **Step 4: Run tests — verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/state/filestore.go internal/state/filestore_test.go
git commit -m "feat: JSONL append-only event store with filtering and persistence"
```

---

### Task 1.5: SQLite Migration System

**Files:**
- Create: `internal/state/migrator.go`
- Create: `internal/state/migrator_test.go`
- Create: `migrations/001_init.sql`
- Create: `migrations/002_indexes.sql`
- Create: `migrations/003_token_usage.sql`
- Create: `migrations/004_session_health.sql`
- Create: `migrations/005_pipeline_runs.sql`

- [ ] **Step 1: Write migrator tests** — AppliesAllMigrations, SkipsAlreadyApplied, TracksVersions, InvalidSQL returns error.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Create migration SQL files** — 001_init (core tables + schema_migrations), 002_indexes (all performance indexes), 003-005 (token_usage, session_health, pipeline_runs).

- [ ] **Step 4: Implement migrator** — uses `embed.FS`, reads `schema_migrations`, applies missing files in order, forward-only.

- [ ] **Step 5: Run tests — verify they pass**

- [ ] **Step 6: Commit**

```bash
git add internal/state/migrator.go internal/state/migrator_test.go migrations/
git commit -m "feat: forward-only SQL migration system with embedded migration files"
```

---

### Task 1.6: SQLite Projection Store

**Files:**
- Create: `internal/state/sqlite.go`
- Create: `internal/state/sqlite_test.go`

- [ ] **Step 1: Write projection tests** — ProjectReqSubmitted, ProjectStoryLifecycle (full state machine), ListStoriesFiltered, ListRequirementsFiltered, TypedPayloadProjection, ArchiveRequirement, PaginationLimitOffset.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement SQLiteStore** — opens DB in WAL mode (`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000`), runs migrations, implements all ProjectionStore methods using typed payload structs. Pagination via LIMIT/OFFSET.

- [ ] **Step 4: Run tests — verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/state/sqlite.go internal/state/sqlite_test.go
git commit -m "feat: SQLite projection store with WAL mode, typed payloads, and pagination"
```

---

### Task 1.7: Async Projector

**Files:**
- Create: `internal/state/projector.go`
- Create: `internal/state/projector_test.go`

- [ ] **Step 1: Write projector tests** — events projected via channel, shutdown drains fully, errors don't crash goroutine.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement Projector** — buffered channel, background goroutine draining and applying to ProjectionStore, `Send()` for non-blocking enqueue, `Shutdown()` for clean drain.

- [ ] **Step 4: Run tests — verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/state/projector.go internal/state/projector_test.go
git commit -m "feat: async event projector with channel-based decoupling"
```

---

### Task 1.8: Structured Logging Infrastructure

**Files:**
- Create: `internal/logging/logger.go`

- [ ] **Step 1: Implement slog logger factory** — configurable log level from config, JSON output to stderr + rotated log file (`~/.px/logs/px.log`), component-tagged child loggers (e.g., `logger.With("component", "monitor")`), story_id context when applicable.

- [ ] **Step 2: Wire into CLI root** — initialize logger in `PersistentPreRunE`, make available to all subcommands.

- [ ] **Step 3: Commit**

```bash
git add internal/logging/
git commit -m "feat: structured slog logging with JSON output, file rotation, and component tagging"
```

---

### Task 1.9: Wire State Layer into CLI

**Files:**
- Modify: `internal/cli/root.go`
- Create: `internal/cli/migrate.go`
- Create: `internal/cli/events.go`
- Create: `internal/cli/status.go`

- [ ] **Step 1: Add `px migrate` command** — loads config, opens SQLite, runs migrations, prints count.

- [ ] **Step 2: Add `px events` command** — lists recent events with `--limit` flag.

- [ ] **Step 3: Add `px status` command** — shows requirements and story counts by status.

- [ ] **Step 4: Update root command** — `PersistentPreRunE` loads config, initializes stores in `appState` struct.

- [ ] **Step 5: Build and verify**

```bash
go build ./cmd/px && ./px migrate && ./px status && ./px events
```

- [ ] **Step 6: Commit**

```bash
git add internal/cli/
git commit -m "feat: wire state layer into CLI with migrate, events, and status commands"
```

---

## Phase 2: LLM Clients & Cost Protection

Produces: LLM client layer with budget enforcement and `px cost` command.

### Task 2.1: LLM Client Interface & Error Types

**Files:**
- Create: `internal/llm/client.go`
- Create: `internal/llm/errors.go`

- [ ] **Step 1: Define Client interface** — `Complete(ctx, CompletionRequest) (CompletionResponse, error)`, with `CompletionResponse` including `InputTokens` and `OutputTokens`.

- [ ] **Step 2: Define error types** — `APIError` (StatusCode, Message, Retryable), `BudgetExhaustedError`, `IsFatalAPIError()`.

- [ ] **Step 3: Commit**

```bash
git add internal/llm/
git commit -m "feat: LLM client interface, message types, and error classification"
```

---

### Task 2.2: Claude CLI Client

**Files:**
- Create: `internal/llm/claude_cli.go`
- Create: `internal/llm/claude_cli_test.go`

- [ ] **Step 1: Write tests** — prompt building, code fence trimming, error classification, JSON envelope parsing with token extraction.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement** — stdin piping, JSON output parsing, ANTHROPIC_API_KEY stripping, error classification, `InputTokens`/`OutputTokens` extraction from envelope.

- [ ] **Step 4: Run tests — verify they pass**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: Claude Code CLI client with stdin piping and token tracking"
```

---

### Task 2.3: Anthropic API Client

**Files:**
- Create: `internal/llm/anthropic.go`
- Create: `internal/llm/anthropic_test.go`

- [ ] **Step 1: Write tests** using `httptest` — response parsing, error handling, token extraction from `usage` field.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement** — direct HTTP to Messages API, extracts `input_tokens`/`output_tokens` from response.

- [ ] **Step 4: Run tests — verify they pass**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: Anthropic API client with token usage extraction"
```

---

### Task 2.4: OpenAI Client, Retry Wrapper, Replay Client

**Files:**
- Create: `internal/llm/openai.go`, `internal/llm/openai_test.go`
- Create: `internal/llm/retry.go`, `internal/llm/retry_test.go`
- Create: `internal/llm/replay.go`

- [ ] **Step 1: Write OpenAI client tests** — response parsing, error handling, token extraction from `usage` field.

- [ ] **Step 2: Implement OpenAI client** — chat completions API, token extraction from `usage`.

- [ ] **Step 3: Write retry wrapper tests** — retries on retryable errors, stops on fatal, respects max attempts, exponential backoff.

- [ ] **Step 3: Implement RetryClient** — wraps any `Client`, configurable max attempts and base delay.

- [ ] **Step 4: Implement ReplayClient** — loads fixture JSON, returns deterministic responses for testing.

- [ ] **Step 5: Run all LLM tests**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat: OpenAI client, retry wrapper with backoff, and replay test client"
```

---

### Task 2.5: Cost Ledger

**Files:**
- Create: `internal/cost/ledger.go`
- Create: `internal/cost/ledger_test.go`
- Create: `internal/cost/pricing.go`
- Create: `internal/cost/pricing_test.go`

- [ ] **Step 1: Write pricing tests** — ComputeCost for known models, unknown model fallback, zero tokens, rounding.

- [ ] **Step 2: Write ledger tests** — record usage, query by story/requirement/day, total computation.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement pricing table** (`pricing.go`) — `DefaultPricing` map, `ComputeCost()` function.

- [ ] **Step 4: Implement SQLiteLedger** (`ledger.go`) — `Record()`, `QueryByStory()`, `QueryByRequirement()`, `QueryByDay()`, writing to `token_usage` table.

- [ ] **Step 5: Run tests — verify they pass**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat: cost ledger with pricing table and SQLite-backed usage tracking"
```

---

### Task 2.6: Circuit Breaker

**Files:**
- Create: `internal/cost/breaker.go`
- Create: `internal/cost/breaker_test.go`

- [ ] **Step 1: Write tests** — allows within budget, blocks on story/requirement/daily breach, emits warning at threshold.

- [ ] **Step 2: Run tests — verify they fail**

- [ ] **Step 3: Implement BudgetBreaker** — wraps `llm.Client`, checks budgets pre-call, records usage post-call, emits `EventBudgetWarning` at threshold.

- [ ] **Step 4: Run tests — verify they pass**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: cost circuit breaker with pre-call budget enforcement"
```

---

### Task 2.7: `px cost` Command

**Files:**
- Create: `internal/cli/cost.go`

- [ ] **Step 1: Implement** — queries ledger, pretty-prints table with budget bars by story/requirement/day.

- [ ] **Step 2: Build and verify**

```bash
go build ./cmd/px && ./px cost
```

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: px cost command showing spending breakdown with budget indicators"
```

---

## Phase 3: Git, Graph & Planner

Produces: `px plan` command that decomposes requirements into stories.

### Task 3.1: Git Operations

**Files:**
- Create: `internal/git/ops.go`, `internal/git/ops_test.go`
- Create: `internal/git/worktree.go`, `internal/git/worktree_test.go`
- Create: `internal/git/github.go`, `internal/git/github_test.go`
- Create: `internal/git/scan.go`, `internal/git/scan_test.go`

- [ ] **Step 1: Write and implement git ops** — FetchBranch, RebaseOnto, Diff, MergeBase, DeleteRemoteBranch. Use `CommandRunner` interface for testability.

- [ ] **Step 2: Write and implement worktree ops** — CreateWorktree, RemoveWorktree.

- [ ] **Step 3: Write and implement GitHub ops** — PR create, merge, auto-merge via `gh` CLI.

- [ ] **Step 4: Implement tech stack scanner** — detect language/framework/test runner from marker files.

- [ ] **Step 5: Run all tests, commit**

```bash
git commit -m "feat: git operations, worktree management, GitHub PR integration, and tech stack detection"
```

---

### Task 3.2: DAG & Topological Sort

**Files:**
- Create: `internal/graph/dag.go`, `internal/graph/dag_test.go`
- Create: `internal/graph/topo.go`, `internal/graph/topo_test.go`

- [ ] **Step 1: Write DAG tests** (in `dag_test.go`: add nodes/edges, detect cycles; in `topo_test.go`: topo sort ordering, ReadyNodes, wave grouping) — add nodes/edges, detect cycles, topo sort, ReadyNodes, wave grouping.

- [ ] **Step 2: Implement** — adjacency list, in-degree tracking, Kahn's algorithm, wave grouping by topological layer.

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat: dependency DAG with topological sort and wave grouping"
```

---

### Task 3.3: Agent Roles & Prompts

**Files:**
- Create: `internal/agent/roles.go`, `internal/agent/roles_test.go`
- Create: `internal/agent/prompts.go`, `internal/agent/prompts_test.go`

- [ ] **Step 1: Write and implement roles** — TechLead, Senior, Intermediate, Junior, QA, Supervisor with complexity routing and ModelConfig lookup.

- [ ] **Step 2: Write and implement prompt templates** — structured system/goal prompts per role, incorporating story details, acceptance criteria, review feedback, tech stack.

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat: agent roles with complexity routing and structured prompt generation"
```

---

### Task 3.4: Two-Pass Planner

**Files:**
- Create: `internal/planner/planner.go`, `internal/planner/planner_test.go`
- Create: `internal/planner/validator.go`, `internal/planner/validator_test.go`
- Create: `internal/planner/techstack.go`

- [ ] **Step 1: Write planner tests** using replay client — decomposition into stories with deps, owned files, complexity.

- [ ] **Step 2: Implement Pass 1** — requirement + tech stack → structured JSON stories.

- [ ] **Step 3: Write validator tests** — all required fields present, complexity bounds, DAG acyclic, file ownership non-overlapping.

- [ ] **Step 4: Implement Pass 2** — validation LLM call, critique feedback loop (max 2 rounds).

- [ ] **Step 5: Implement techstack.go** — enhanced detection feeding into planner prompt.

- [ ] **Step 6: Run all tests, commit**

```bash
git commit -m "feat: two-pass planner with validation, quality enforcement, and tech stack context"
```

---

### Task 3.5: `px plan` Command

**Files:**
- Create: `internal/cli/plan.go`

- [ ] **Step 1: Implement** `px plan <file>` — reads requirement, runs planner, stores events, prints summary.

- [ ] **Step 2: Implement** `px plan --review <req-id>` — displays plan with stories, deps, complexity.

- [ ] **Step 3: Implement** `px plan --refine <req-id>` — re-plans with user feedback.

- [ ] **Step 4: Build and test**

```bash
echo "Add user authentication with OAuth2" > /tmp/req.txt
./px plan /tmp/req.txt
```

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: px plan command with plan/review/refine workflow"
```

---

## Phase 4: Runtime, Pipeline & Execution

Produces: `px resume` that dispatches and monitors agents through the full pipeline.

### Task 4.1: Tmux Session Management & Health

**Files:**
- Create: `internal/tmux/session.go`, `internal/tmux/session_test.go`
- Create: `internal/tmux/health.go`, `internal/tmux/health_test.go`

- [ ] **Step 1: Write and implement session ops** — Create, Kill, Exists, List, SendKeys, ReadOutput.

- [ ] **Step 2: Write and implement health monitor** — `SessionHealth()` using pane_pid/pane_dead/output hashing, returns `Healthy`/`Stale`/`Dead`/`Missing`.

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat: tmux session management with health monitoring and staleness detection"
```

---

### Task 4.2: Runtime Interface & Built-in Runtimes

**Files:**
- Create: `internal/runtime/runtime.go`
- Create: `internal/runtime/registry.go`, `internal/runtime/registry_test.go`
- Create: `internal/runtime/claude.go`, `internal/runtime/claude_test.go`
- Create: `internal/runtime/codex.go`, `internal/runtime/gemini.go`

- [ ] **Step 1: Define Runtime interface** — `Runtime`, `AgentStatus`, `HealthStatus`, `SessionConfig`, `RuntimeCapabilities`.

- [ ] **Step 2: Write and implement Registry** — register, get, list runtimes.

- [ ] **Step 3: Write and implement ClaudeCodeRuntime** — spawn args, status detection, godmode, health delegation.

- [ ] **Step 4: Implement CodexRuntime and GeminiRuntime.**

- [ ] **Step 5: Run tests, commit**

```bash
git commit -m "feat: runtime plugin interface with Claude Code, Codex, and Gemini implementations"
```

---

### Task 4.3: Runtime Router

**Files:**
- Create: `internal/runtime/router.go`, `internal/runtime/router_test.go`

- [ ] **Step 1: Write tests** — cost-optimized selection, capability matching, fallback on unhealthy.

- [ ] **Step 2: Implement router.**

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat: cost-aware runtime router with fallback chains"
```

---

### Task 4.4: Pipeline Stage Interface & Runner

**Files:**
- Create: `internal/pipeline/stage.go`
- Create: `internal/pipeline/runner.go`, `internal/pipeline/runner_test.go`

- [ ] **Step 1: Define Stage interface** — `Stage`, `StageResult{Passed, Failed, Fatal}`, `StoryContext`.

- [ ] **Step 2: Write runner tests** — stage sequencing, retry on Failed, halt on Fatal, budget check, event emission.

- [ ] **Step 3: Implement PipelineRunner** — configurable stage sequence, per-stage retries, cost breaker integration.

- [ ] **Step 4: Run tests, commit**

```bash
git commit -m "feat: pipeline stage interface and runner with per-stage retry policies"
```

---

### Task 4.5: Pipeline Stages

**Files:**
- Create: `internal/pipeline/autocommit.go`, `internal/pipeline/autocommit_test.go`
- Create: `internal/pipeline/diffcheck.go`, `internal/pipeline/diffcheck_test.go`
- Create: `internal/pipeline/review.go`, `internal/pipeline/review_test.go`
- Create: `internal/pipeline/qa.go`, `internal/pipeline/qa_test.go`
- Create: `internal/pipeline/rebase.go`, `internal/pipeline/rebase_test.go`
- Create: `internal/pipeline/merge.go`, `internal/pipeline/merge_test.go`
- Create: `internal/pipeline/cleanup.go`, `internal/pipeline/cleanup_test.go`

- [ ] **Step 1: Write and implement AutoCommitStage and DiffCheckStage** (with tests first).

- [ ] **Step 2: Write and implement ReviewStage** — LLM review with retry/escalation logic.

- [ ] **Step 3: Write and implement QAStage** — lint/build/test per tech stack.

- [ ] **Step 4: Write and implement RebaseStage** — rebase with LLM conflict resolution (max 10 rounds).

- [ ] **Step 5: Write and implement MergeStage** — push, PR, auto-merge.

- [ ] **Step 6: Implement CleanupStage.**

- [ ] **Step 7: Run all pipeline tests, commit**

```bash
git commit -m "feat: all pipeline stages - autocommit, diffcheck, review, qa, rebase, merge, cleanup"
```

---

### Task 4.6: Dispatcher & Executor

**Files:**
- Create: `internal/monitor/dispatcher.go`, `internal/monitor/dispatcher_test.go`
- Create: `internal/monitor/executor.go`, `internal/monitor/executor_test.go`

- [ ] **Step 1: Write and implement Dispatcher** — wave dispatch from DAG, sequential-first ordering, file overlap filtering, role assignment.

- [ ] **Step 2: Implement Executor** — worktree creation, CLAUDE.md instructions, runtime routing, tmux spawn, event emission.

- [ ] **Step 3: Run tests, commit**

```bash
git commit -m "feat: wave dispatcher with overlap filtering and executor with runtime routing"
```

---

### Task 4.7: Agent Poller (Monitor)

**Files:**
- Create: `internal/monitor/poller.go`, `internal/monitor/poller_test.go`

- [ ] **Step 1: Write tests** — polls agents, detects completion, hands off to pipeline, auto-dispatches next wave.

- [ ] **Step 2: Implement slim Poller** (~150 lines) — poll, check health, hand off, trigger wave.

- [ ] **Step 3: Implement graceful shutdown** — on context cancellation: stop polling, wait for in-flight pipeline stages (30s timeout), drain projection channel, leave tmux sessions alive, close stores. Test the shutdown sequence.

- [ ] **Step 4: Verify poller under 200 lines.**

- [ ] **Step 4: Run tests, commit**

```bash
git commit -m "feat: slim agent poller with health-aware monitoring and auto-wave dispatch"
```

---

### Task 4.8: Watchdog

**Files:**
- Create: `internal/monitor/watchdog.go`, `internal/monitor/watchdog_test.go`

- [ ] **Step 1: Write and implement** — permission bypass, plan escape, stuck detection, session health integration.

- [ ] **Step 2: Run tests, commit**

```bash
git commit -m "feat: watchdog with session health integration and stuck detection"
```

---

### Task 4.9: `px resume` Command

**Files:**
- Create: `internal/cli/resume.go`

- [ ] **Step 1: Implement** `px resume <req-id>` — load requirement, rebuild DAG, dispatch wave, start poller with pipeline.

- [ ] **Step 2: Add `--godmode` flag.**

- [ ] **Step 3: Build and smoke test.**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat: px resume command with full pipeline orchestration"
```

---

## Phase 5: Dashboards

Produces: `px dashboard` (TUI) and `px dashboard --web` (browser).

### Task 5.1: Scrollable TUI Dashboard

**Files:**
- Create all files in `internal/dashboard/`

- [ ] **Step 1: Implement viewport wrapper** — Bubbletea viewport with `j/k/g/G/PgUp/PgDn`.

- [ ] **Step 2: Implement app.go** — 6-panel Model with scrollable viewports, tab switching, data refresh.

- [ ] **Step 3: Implement all panels** — pipeline (kanban), agents (table + health), activity, escalations, cost (budget bars), logs (filtered stream).

- [ ] **Step 4: Implement styles and status bar** — wave progress, budget usage, scroll position.

- [ ] **Step 5: Build and test interactively.**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat: scrollable TUI dashboard with 6 panels, viewport navigation, and live data"
```

---

### Task 5.2: Web Dashboard — Server & API

**Files:**
- Create all files in `internal/web/`

- [ ] **Step 1: Write handler tests.**

- [ ] **Step 2: Implement server** — binds to `127.0.0.1` only, graceful shutdown. Add `--bind` flag with security warning if set to `0.0.0.0`.

- [ ] **Step 3: Implement REST handlers** — all `/api/*` endpoints.

- [ ] **Step 4: Implement SSE** — `/api/stream` for real-time push.

- [ ] **Step 5: Implement embed.go** — `embed.FS` for static files.

- [ ] **Step 6: Run tests, commit**

```bash
git commit -m "feat: web dashboard REST API with SSE streaming and embedded static serving"
```

---

### Task 5.3: Web Dashboard — Frontend

**Files:**
- Create: `web/index.html`, `web/app.js`, `web/style.css`

- [ ] **Step 1: Create HTML shell and Alpine.js app** — SSE connection, auto-reconnect, tab panels.

- [ ] **Step 2: Implement all views** — kanban pipeline, agent table, activity feed, escalations, cost bars, log viewer.

- [ ] **Step 3: Style with Tailwind.**

- [ ] **Step 4: Test in browser.**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: browser-based web dashboard with Alpine.js, SSE, and kanban pipeline view"
```

---

### Task 5.4: `px dashboard` Command

**Files:**
- Create: `internal/cli/dashboard.go`

- [ ] **Step 1: Implement** — TUI default, `--web` flag, `--port` flag, auto-open browser.

- [ ] **Step 2: Commit**

```bash
git commit -m "feat: px dashboard command with TUI and web mode"
```

---

## Phase 6: Integration, CI/CD & Release

Produces: tested, documented, packaged, open-source-ready project.

### Task 6.1: End-to-End Integration Tests

**Files:**
- Create: `test/e2e/pipeline_test.go`
- Create: `test/fixtures/replay/`

- [ ] **Step 1: Write full pipeline E2E test** — submit → plan → dispatch → "complete" → review → QA → merge → next wave → done.

- [ ] **Step 2: Write budget exhaustion E2E** — verify breaker pauses requirement.

- [ ] **Step 3: Write session health E2E** — verify dead session triggers recovery.

- [ ] **Step 4: Verify coverage >= 80%**

```bash
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1
```

- [ ] **Step 5: Commit**

```bash
git commit -m "test: end-to-end pipeline integration tests with replay fixtures"
```

---

### Task 6.2: CI/CD Pipeline

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`
- Create: `.goreleaser.yml`

- [ ] **Step 1: Create CI workflow** — vet, lint, test, coverage gate.

- [ ] **Step 2: Create release workflow** — GoReleaser on tag push.

- [ ] **Step 3: Create GoReleaser config** — linux/darwin/windows, amd64/arm64.

- [ ] **Step 4: Commit**

```bash
git commit -m "ci: GitHub Actions for CI testing and GoReleaser binary releases"
```

---

### Task 6.3: Documentation & Remaining CLI

**Files:**
- Create: `README.md`
- Create: `CHANGELOG.md`
- Create: `internal/cli/agents.go`, `internal/cli/config_cmd.go`, `internal/cli/gc.go`, `internal/cli/archive.go`

- [ ] **Step 1: Write README** — description, features, quick start, install, usage, architecture, config reference.

- [ ] **Step 2: Write CONTRIBUTING** — dev setup, code style, test conventions, PR process, architecture overview.

- [ ] **Step 3: Create issue templates** — `.github/ISSUE_TEMPLATE/bug_report.yml`, `feature_request.yml`, `runtime_plugin.yml`.

- [ ] **Step 4: Implement remaining CLI commands** — agents, config, gc, archive.

- [ ] **Step 5: Commit**

```bash
git commit -m "docs: README, CONTRIBUTING, issue templates, and remaining CLI commands"
```

---

### Task 6.4: Final Polish & Release

- [ ] **Step 1: Run full suite**

```bash
make lint && make test
```

- [ ] **Step 2: Verify coverage >= 80%**

- [ ] **Step 3: Build and smoke test all commands**

```bash
make build && ./px version && ./px config show && ./px migrate && ./px status && ./px cost && ./px dashboard
```

- [ ] **Step 4: Push to GitHub**

```bash
git push -u origin main
```

- [ ] **Step 5: Tag v0.1.0**

```bash
git tag -a v0.1.0 -m "Initial release: px-dispatch"
git push origin v0.1.0
```

---

## Dependency Order

```
Phase 1 (Foundation)
  └── Phase 2 (LLM & Cost)
        └── Phase 3 (Git, Graph, Planner)
              └── Phase 4 (Runtime, Pipeline, Execution)
                    └── Phase 5 (Dashboards)
                          └── Phase 6 (Integration & Release)
```

## Task Summary

| Phase | Tasks | Focus |
|-------|-------|-------|
| 1 | 9 | Foundation, State Layer & Logging |
| 2 | 7 | LLM Clients & Cost Protection |
| 3 | 5 | Git, Graph & Planner |
| 4 | 9 | Runtime, Pipeline & Execution |
| 5 | 4 | Dashboards |
| 6 | 4 | Integration & Release |
| **Total** | **38** | |
