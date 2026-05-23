---
title: Lesson 06 · Adding a new runtime
tags: [px-dispatch, training, runtime, contributor]
---

# Lesson 06 · Adding a new runtime

**Goal:** Plug a fourth AI coding CLI into px-dispatch end-to-end. ~45
minutes; no LLM spend (you'll mock the CLI).

We'll add a fictional `roo` runtime — an imaginary AI CLI that prints "ROO
DID IT" to stdout. The mechanics are identical to plugging in a real one.

## The interface

```go
// internal/runtime/runtime.go
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

Six methods. `Capabilities` is a struct telling the router what you support.

## Step 1 — scaffold the file

```bash
cd /Users/mncedimini/Sites/misc/project-x
cat > internal/runtime/roo.go <<'EOF_GO'
package runtime

import (
    "fmt"
    "regexp"
    "strings"

    "github.com/tzone85/px-dispatch/internal/git"
    "github.com/tzone85/px-dispatch/internal/tmux"
)

var (
    rooPermissionRe = regexp.MustCompile(`(?i)(allow|approve)`)
    rooPlanModeRe   = regexp.MustCompile(`(?i)plan mode`)
    rooIdleRe       = regexp.MustCompile(`(?m)^\$\s*$`)
)

type RooRuntime struct {
    godmode bool
}

func NewRooRuntime(godmode bool) *RooRuntime { return &RooRuntime{godmode: godmode} }

func (r *RooRuntime) Name() string { return "roo" }

func (r *RooRuntime) Spawn(runner git.CommandRunner, cfg SessionConfig) error {
    cmd := r.buildCommand(cfg)
    return tmux.CreateSession(runner, cfg.SessionName, cfg.WorkDir, cmd)
}

func (r *RooRuntime) Kill(runner git.CommandRunner, sessionName string) error {
    return tmux.KillSession(runner, sessionName)
}

func (r *RooRuntime) DetectStatus(runner git.CommandRunner, sessionName string) (AgentStatus, error) {
    if !tmux.SessionExists(runner, sessionName) {
        return StatusDone, nil
    }
    output, err := tmux.ReadOutput(runner, sessionName, 50)
    if err != nil {
        return StatusWorking, nil
    }
    return r.classifyOutput(output), nil
}

func (r *RooRuntime) ReadOutput(runner git.CommandRunner, sessionName string, lines int) (string, error) {
    return tmux.ReadOutput(runner, sessionName, lines)
}

func (r *RooRuntime) SendInput(runner git.CommandRunner, sessionName string, input string) error {
    return tmux.SendKeys(runner, sessionName, input)
}

func (r *RooRuntime) Capabilities() RuntimeCapabilities {
    return RuntimeCapabilities{
        SupportsModel:      []string{"roo-1"},
        SupportsGodmode:    r.godmode,
        SupportsLogFile:    true,
        SupportsJsonOutput: false,
        MaxPromptLength:    0,
        CostTier:           CostTierSubscription,
    }
}

func (r *RooRuntime) buildCommand(cfg SessionConfig) string {
    parts := []string{"roo"}
    if r.godmode {
        parts = append(parts, "--skip-perms")
    }
    if cfg.Model != "" {
        parts = append(parts, "--model", cfg.Model)
    }
    parts = append(parts, "--prompt", "-")
    cmd := strings.Join(parts, " ")
    if cfg.LogFile != "" {
        cmd = cmd + " | tee " + shellQuote(cfg.LogFile)
    }
    delim := randomHeredocDelimiter()
    return "rm -f .px-done\n" +
        "cat <<'" + delim + "' | " + cmd + "\n" + sanitizeHeredocBody(cfg.Goal, delim) + "\n" + delim + "\n" +
        "rc=$?\n" +
        "printf '$\\n'\n" +
        "touch .px-done\n" +
        "sleep 30\n" +
        "exit $rc"
}

func (r *RooRuntime) classifyOutput(output string) AgentStatus {
    if rooPermissionRe.MatchString(output) { return StatusPermissionPrompt }
    if rooPlanModeRe.MatchString(output) { return StatusPlanMode }
    lines := strings.Split(strings.TrimRight(output, " \t\n"), "\n")
    if len(lines) > 0 {
        if rooIdleRe.MatchString(strings.TrimSpace(lines[len(lines)-1])) {
            return StatusIdle
        }
    }
    return StatusWorking
}

var _ = fmt.Sprintf  // keep imports tidy in stub
EOF_GO
```

Notes worth seeing:

- `randomHeredocDelimiter()` + `sanitizeHeredocBody()` are reused from
  `heredoc.go`. **Always** use them — fixed-delimiter heredocs were the
  Critical-1 security finding.
- `rm -f .px-done` and `touch .px-done` are non-negotiable. The poller
  depends on the sentinel.
- `rc=$?` not `status=$?` — `status` is read-only in zsh.

## Step 2 — register it

`internal/cli/resume.go` near the registry setup:

```go
reg.Register("claude-code", runtime.NewClaudeCodeRuntime(godmode))
reg.Register("codex",       runtime.NewCodexRuntime(godmode))
reg.Register("gemini",      runtime.NewGeminiRuntime())
reg.Register("roo",         runtime.NewRooRuntime(godmode))    // ← new
```

## Step 3 — test it

```go
// internal/runtime/roo_test.go
package runtime

import (
    "fmt"
    "strings"
    "testing"

    "github.com/tzone85/px-dispatch/internal/git"
)

func TestRooRuntime_BuildCommand(t *testing.T) {
    rt := NewRooRuntime(false)
    cmd := rt.buildCommand(SessionConfig{Goal: "say hi", Model: "roo-1"})
    for _, want := range []string{"roo", "--model", "roo-1", "rm -f .px-done", "touch .px-done"} {
        if !strings.Contains(cmd, want) { t.Errorf("missing %q in %q", want, cmd) }
    }
    if strings.Contains(cmd, "status=$?") {
        t.Errorf("must use rc=$?, not status=$?")
    }
}

func TestRooRuntime_Spawn(t *testing.T) {
    m := git.NewMockRunner()
    m.AddResponse("", fmt.Errorf("no session"))   // has-session fails
    m.AddResponse("", nil)                         // new-session succeeds
    rt := NewRooRuntime(false)
    err := rt.Spawn(m, SessionConfig{
        SessionName: "px-s-1", WorkDir: "/tmp/work", Goal: "x", Model: "roo-1",
    })
    if err != nil { t.Fatalf("spawn: %v", err) }
}
```

```bash
go test ./internal/runtime/... -run TestRoo
```

## Step 4 — let it be selected

```yaml
# px.yaml
routing:
  preferences:
    - { role: junior, prefer: roo }
    - { role: intermediate, prefer: roo }
```

When the dispatcher assigns a story to a role with `prefer: roo`, the router
picks `RooRuntime`. If `roo` isn't registered, the router falls through to
the next preference or the first registered runtime.

## Step 5 — stub the binary

For development without a real CLI, write a tiny script that mimics `roo`:

```bash
cat > /usr/local/bin/roo <<'STUB'
#!/bin/sh
cat - > /tmp/roo-input.txt
echo "ROO RECEIVED $(wc -l < /tmp/roo-input.txt) LINES"
echo "ROO DID IT"
exit 0
STUB
chmod +x /usr/local/bin/roo
```

Now `px resume` against a sandbox will spawn `roo`, "do nothing useful",
land an empty diff, and the pipeline's `diffcheck` stage will fail the story
— exactly the right behaviour, validating your wiring without spending real
LLM calls.

## Step 6 — capabilities and cost tier

If `roo` is a per-token API runtime (not subscription), set
`CostTier: CostTierAPI` in `Capabilities`. The `BudgetBreaker` then records
actual spend. The router still respects the `prefer:` setting but the cost
ledger reflects reality.

Pricing comes from `internal/cost/pricing.go::DefaultPricing` — add an entry
for `roo-1`:

```go
"roo-1": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
```

## Checks

- `go test ./internal/runtime/... -race` passes including your new file.
- `go vet ./...` clean.
- `px resume <req-id>` with `prefer: roo` actually invokes `roo`.
- The agent's transcript shows your CLI's output.
- The pipeline's `diffcheck` correctly fails when the agent produces no
  changes — proves the runtime wiring works without depending on real LLM
  output.

## Exercises

### A — implement Aider

Aider is a real CLI (`pip install aider-chat`). Add it as a fifth runtime
following the steps above. Aider has a non-trivial twist: it expects the
prompt as a `--message` flag, not via stdin. Adapt `buildCommand`
accordingly.

### B — model-specific routing

Make `roo` only get picked for stories with `OwnedFiles` containing `*.rs`
files (Rust). This requires extending `runtime.Router.SelectRuntime` to
accept the story's file list. Open question — there's no hook for this
today. Write up a design proposal in [[../11-Open-questions]] before
coding.

### C — capability-based gating

Add a new field `RuntimeCapabilities.SupportsLargeContext` (boolean) and
have the router skip runtimes without it for stories whose
`StoryDescription + AcceptanceCriteria` exceeds 8000 characters. Update
`claude-code` and `gemini` to declare `true`; leave `codex` at `false`.
Add a test that exercises the skip behaviour.

Next: [[07-Tuning-the-review-stage]] — once you can pick runtimes, learn to
calibrate the quality gate they feed into.
