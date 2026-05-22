package cli

import (
	"os"
	"time"

	"github.com/tzone85/project-x/internal/llm"
)

const (
	retryMaxAttempts = 3
	retryBaseDelay   = 2 * time.Second
)

// llmClientBuilder is the package-level constructor for the LLM client used by
// commands. Tests override it to inject a deterministic in-memory client so
// runPlan/runPlanRefine can be exercised without a live subscription or API.
var llmClientBuilder = defaultBuildLLMClient

// buildLLMClient returns an LLM client wrapped with retry logic.
// Claude CLI is ALWAYS the primary client (uses subscription, no per-token cost).
// Anthropic API is only used as a fallback if PX_USE_API=true is explicitly set.
// This prevents accidental API spend while still allowing approved fallback
// to OpenAI when Claude is exhausted.
func buildLLMClient() llm.Client {
	return llmClientBuilder()
}

func defaultBuildLLMClient() llm.Client {
	var base llm.Client

	if os.Getenv("PX_USE_API") == "true" {
		if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
			base = llm.NewAnthropicClient(apiKey)
		} else {
			base = llm.NewClaudeCLIClient()
		}
	} else {
		base = llm.NewClaudeCLIClient()
	}

	primary := llm.NewRetryClient(base, retryMaxAttempts, retryBaseDelay)
	if !app.config.Fallback.Enabled {
		return primary
	}

	var fallbackClient llm.Client
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		fallbackClient = llm.NewRetryClient(llm.NewOpenAIClient(apiKey), retryMaxAttempts, retryBaseDelay)
	}

	client := llm.NewFallbackClient(primary, fallbackClient, app.config.Fallback, modelSwitchApprover)
	if llm.HasCodexCLI() {
		client.WithCodexCLI(
			llm.NewRetryClient(llm.NewCodexCLIClient(), retryMaxAttempts, retryBaseDelay),
		)
	}

	return client
}
