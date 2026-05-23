package cli

import (
	"os"
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
)

// withEnv sets an env var for the duration of a test.
func withEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv(key)
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestBuildLLMClient_FallbackDisabled(t *testing.T) {
	prev := app
	t.Cleanup(func() { app = prev })

	cfg := config.Defaults()
	cfg.Fallback.Enabled = false
	app = appState{config: cfg}

	withEnv(t, "PX_USE_API", "")
	c := buildLLMClient()
	if c == nil {
		t.Fatal("nil client returned")
	}
}

func TestBuildLLMClient_FallbackEnabled_NoOpenAIKey(t *testing.T) {
	prev := app
	t.Cleanup(func() { app = prev })

	cfg := config.Defaults()
	cfg.Fallback.Enabled = true
	app = appState{config: cfg}

	withEnv(t, "PX_USE_API", "")
	withEnv(t, "OPENAI_API_KEY", "")
	c := buildLLMClient()
	if c == nil {
		t.Fatal("nil client returned")
	}
}

func TestBuildLLMClient_FallbackEnabled_WithOpenAIKey(t *testing.T) {
	prev := app
	t.Cleanup(func() { app = prev })

	cfg := config.Defaults()
	cfg.Fallback.Enabled = true
	app = appState{config: cfg}

	withEnv(t, "PX_USE_API", "")
	withEnv(t, "OPENAI_API_KEY", "sk-test")
	c := buildLLMClient()
	if c == nil {
		t.Fatal("nil client returned")
	}
}

func TestBuildLLMClient_UseAPI_WithKey(t *testing.T) {
	prev := app
	t.Cleanup(func() { app = prev })

	cfg := config.Defaults()
	cfg.Fallback.Enabled = false
	app = appState{config: cfg}

	withEnv(t, "PX_USE_API", "true")
	withEnv(t, "ANTHROPIC_API_KEY", "sk-test-anthropic")
	c := buildLLMClient()
	if c == nil {
		t.Fatal("nil client returned")
	}
}

func TestBuildLLMClient_UseAPI_NoKey_FallsBackToCLI(t *testing.T) {
	prev := app
	t.Cleanup(func() { app = prev })

	cfg := config.Defaults()
	cfg.Fallback.Enabled = false
	app = appState{config: cfg}

	withEnv(t, "PX_USE_API", "true")
	withEnv(t, "ANTHROPIC_API_KEY", "")
	c := buildLLMClient()
	if c == nil {
		t.Fatal("nil client returned (must fall back to Claude CLI)")
	}
}
