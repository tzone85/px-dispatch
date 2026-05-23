package runtime

import (
	"fmt"
	"testing"

	"github.com/tzone85/px-dispatch/internal/agent"
	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/git"
)

func TestRouter_CostOptimized_PrefersConfigured(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Strategy: "cost_optimized",
			Preferences: []config.RoutingPreference{
				{Role: "junior", Prefer: "codex", Fallback: "claude-code"},
				{Role: "senior", Prefer: "claude-code", Fallback: "codex"},
			},
		},
	}

	router := NewRouter(reg, cfg)

	// Junior should prefer codex
	rt, err := router.SelectRuntime(agent.RoleJunior)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "codex" {
		t.Errorf("expected codex for junior, got %s", rt.Name())
	}

	// Senior should prefer claude-code
	rt, err = router.SelectRuntime(agent.RoleSenior)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code for senior, got %s", rt.Name())
	}
}

func TestRouter_FallbackWhenPreferredMissing(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	// codex NOT registered

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Preferences: []config.RoutingPreference{
				{Role: "junior", Prefer: "codex", Fallback: "claude-code"},
			},
		},
	}

	router := NewRouter(reg, cfg)
	rt, err := router.SelectRuntime(agent.RoleJunior)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected fallback claude-code, got %s", rt.Name())
	}
}

func TestRouter_DefaultsToFirstRuntime(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))

	cfg := config.Config{} // no routing preferences

	router := NewRouter(reg, cfg)
	rt, err := router.SelectRuntime(agent.RoleIntermediate)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code (only registered), got %s", rt.Name())
	}
}

func TestRouter_ErrorWhenNoRuntimes(t *testing.T) {
	reg := NewRegistry()
	cfg := config.Config{}
	router := NewRouter(reg, cfg)

	_, err := router.SelectRuntime(agent.RoleJunior)
	if err == nil {
		t.Error("expected error when no runtimes registered")
	}
}

func TestRouter_NoPreferenceForRole_UsesDefault(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Preferences: []config.RoutingPreference{
				{Role: "senior", Prefer: "claude-code", Fallback: "codex"},
			},
		},
	}

	router := NewRouter(reg, cfg)
	// QA has no preference configured — should get first available
	rt, err := router.SelectRuntime(agent.RoleQA)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	// Should get one of the registered runtimes
	if rt.Name() != "claude-code" && rt.Name() != "codex" {
		t.Errorf("expected a registered runtime, got %s", rt.Name())
	}
}

func TestRouter_SelectForModel_Finds(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{}
	router := NewRouter(reg, cfg)

	rt, err := router.SelectForModel("gpt-5.4")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "codex" {
		t.Errorf("expected codex for gpt-5.4, got %s", rt.Name())
	}
}

func TestRouter_SelectForModel_ClaudeModel(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{}
	router := NewRouter(reg, cfg)

	rt, err := router.SelectForModel("claude-opus-4-20250514")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code for claude model, got %s", rt.Name())
	}
}

func TestRouter_SelectForModel_NotFound(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))

	cfg := config.Config{}
	router := NewRouter(reg, cfg)

	_, err := router.SelectForModel("unknown-model-xyz")
	if err == nil {
		t.Error("expected error for unsupported model")
	}
}

func TestRouter_SelectForModel_CostOptimized(t *testing.T) {
	reg := NewRegistry()
	// Register two runtimes that both support a model.
	// Claude (subscription) should be preferred over a hypothetical API runtime.
	claude := NewClaudeCodeRuntime(false)
	reg.Register("claude-code", claude)

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Strategy: "cost_optimized",
		},
	}
	router := NewRouter(reg, cfg)

	rt, err := router.SelectForModel("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code (subscription tier), got %s", rt.Name())
	}
}

func TestRouter_SelectHealthy_NoRunner(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Preferences: []config.RoutingPreference{
				{Role: "senior", Prefer: "claude-code"},
			},
		},
	}

	// No runner = no health checks, returns first candidate.
	router := NewRouter(reg, cfg)
	rt, err := router.SelectHealthy(agent.RoleSenior, "px-story-1")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code, got %s", rt.Name())
	}
}

func TestRouter_SelectHealthy_EmptySession(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))

	cfg := config.Config{}
	mock := git.NewMockRunner()
	router := NewRouterWithHealth(reg, cfg, mock)

	// Empty session name = skip health checks.
	rt, err := router.SelectHealthy(agent.RoleSenior, "")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code, got %s", rt.Name())
	}
}

func TestRouter_SelectHealthy_HealthySession(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Preferences: []config.RoutingPreference{
				{Role: "senior", Prefer: "claude-code", Fallback: "codex"},
			},
		},
	}

	mock := git.NewMockRunner()
	// Claude health check: session exists, process alive, output changing.
	mock.AddResponse("", nil)                           // has-session
	mock.AddResponse("12345 0 0", nil)                  // list-panes
	mock.AddResponse("Processing files...", nil)         // capture-pane for health

	router := NewRouterWithHealth(reg, cfg, mock)
	rt, err := router.SelectHealthy(agent.RoleSenior, "px-story-1")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "claude-code" {
		t.Errorf("expected healthy claude-code, got %s", rt.Name())
	}
}

func TestRouter_SelectHealthy_FallsBackOnDead(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Preferences: []config.RoutingPreference{
				{Role: "senior", Prefer: "claude-code", Fallback: "codex"},
			},
		},
	}

	mock := git.NewMockRunner()
	// Claude health: session exists but pane is dead.
	mock.AddResponse("", nil)                  // has-session (claude)
	mock.AddResponse("12345 1 1", nil)          // list-panes: pane_dead=1 (claude is dead)
	// Codex health: session missing (ok for new session).
	mock.AddResponse("", fmt.Errorf("no session")) // has-session (codex) -> missing

	router := NewRouterWithHealth(reg, cfg, mock)
	rt, err := router.SelectHealthy(agent.RoleSenior, "px-story-1")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if rt.Name() != "codex" {
		t.Errorf("expected fallback codex when claude-code is dead, got %s", rt.Name())
	}
}

func TestRouter_SelectHealthy_AllUnhealthy(t *testing.T) {
	reg := NewRegistry()
	reg.Register("claude-code", NewClaudeCodeRuntime(false))
	reg.Register("codex", NewCodexRuntime(false))

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Preferences: []config.RoutingPreference{
				{Role: "senior", Prefer: "claude-code", Fallback: "codex"},
			},
		},
	}

	mock := git.NewMockRunner()
	// Claude: dead
	mock.AddResponse("", nil)          // has-session
	mock.AddResponse("12345 1 1", nil) // dead
	// Codex: also dead
	mock.AddResponse("", nil)          // has-session
	mock.AddResponse("99999 1 1", nil) // dead

	router := NewRouterWithHealth(reg, cfg, mock)
	rt, err := router.SelectHealthy(agent.RoleSenior, "px-story-1")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	// When all are unhealthy, returns first candidate.
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code (first candidate), got %s", rt.Name())
	}
}

func TestRouter_SelectHealthy_NoRuntimes(t *testing.T) {
	reg := NewRegistry()
	cfg := config.Config{}
	mock := git.NewMockRunner()
	router := NewRouterWithHealth(reg, cfg, mock)

	_, err := router.SelectHealthy(agent.RoleSenior, "px-story-1")
	if err == nil {
		t.Error("expected error when no runtimes registered")
	}
}

func TestRouter_BuildCandidateList_CostOptimized(t *testing.T) {
	reg := NewRegistry()
	reg.Register("codex", NewCodexRuntime(false))     // API tier
	reg.Register("gemini", NewGeminiRuntime())    // API tier
	reg.Register("claude-code", NewClaudeCodeRuntime(false)) // Subscription tier

	cfg := config.Config{
		Routing: config.RoutingConfig{
			Strategy: "cost_optimized",
		},
	}

	router := NewRouter(reg, cfg)
	// No preference for QA — all runtimes are candidates.
	// With cost_optimized, subscription should come first.
	rt, err := router.SelectRuntime(agent.RoleQA)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	// First in sorted list is "claude-code" alphabetically, and it's also subscription tier.
	if rt.Name() != "claude-code" {
		t.Errorf("expected claude-code first, got %s", rt.Name())
	}
}

func TestCodexRuntime_Version(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("codex 0.5.1\n", nil)

	rt := NewCodexRuntime(false)
	ver, err := rt.Version(mock)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if ver != "codex 0.5.1" {
		t.Errorf("expected 'codex 0.5.1', got %q", ver)
	}
}

func TestCodexRuntime_CostTier(t *testing.T) {
	rt := NewCodexRuntime(false)
	caps := rt.Capabilities()
	if caps.CostTier != CostTierAPI {
		t.Errorf("expected CostTierAPI, got %d", caps.CostTier)
	}
}

func TestGeminiRuntime_Version(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("gemini-cli 2.0.0\n", nil)

	rt := NewGeminiRuntime()
	ver, err := rt.Version(mock)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if ver != "gemini-cli 2.0.0" {
		t.Errorf("expected 'gemini-cli 2.0.0', got %q", ver)
	}
}

func TestGeminiRuntime_CostTier(t *testing.T) {
	rt := NewGeminiRuntime()
	caps := rt.Capabilities()
	if caps.CostTier != CostTierAPI {
		t.Errorf("expected CostTierAPI, got %d", caps.CostTier)
	}
}

func TestGeminiRuntime_Health(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)                    // has-session
	mock.AddResponse("99999 0 0", nil)            // list-panes
	mock.AddResponse("generating response...", nil) // capture-pane

	rt := NewGeminiRuntime()
	result, err := rt.Health(mock, "px-gemini-1")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if result.Status != "healthy" {
		t.Errorf("expected 'healthy', got %q", result.Status)
	}
}

func TestCodexRuntime_Health(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails

	rt := NewCodexRuntime(false)
	result, err := rt.Health(mock, "px-codex-1")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if result.Status != "missing" {
		t.Errorf("expected 'missing', got %q", result.Status)
	}
}
