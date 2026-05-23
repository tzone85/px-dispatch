package llm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/modelswitch"
)

// FallbackClient keeps Claude as the primary client but can fail over to
// OpenAI after explicit user approval when Claude account limits are reached.
type FallbackClient struct {
	primary       Client
	apiFallback   Client
	codexFallback Client
	cfg           config.FallbackConfig
	approver      modelswitch.Approver
	activeMode    string
	fallbackLock  sync.Mutex
}

// NewFallbackClient creates a client that prefers primary and only uses
// fallback when Claude can no longer continue.
func NewFallbackClient(
	primary Client,
	fallback Client,
	cfg config.FallbackConfig,
	approver modelswitch.Approver,
) *FallbackClient {
	return &FallbackClient{
		primary:     primary,
		apiFallback: fallback,
		cfg:         cfg,
		approver:    approver,
	}
}

// WithCodexCLI configures a Codex CLI continuation fallback after OpenAI API.
func (c *FallbackClient) WithCodexCLI(cli Client) *FallbackClient {
	c.codexFallback = cli
	return c
}

// Complete routes requests to Claude until account limits are hit and the user
// approves switching the remainder of the command to OpenAI.
func (c *FallbackClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	c.fallbackLock.Lock()
	activeMode := c.activeMode
	c.fallbackLock.Unlock()

	switch activeMode {
	case "api":
		resp, err := c.completeWithNamedFallback(ctx, req, "api", c.apiFallback)
		if err == nil {
			return resp, nil
		}

		reason, ok := shouldFallbackFromOpenAIAPI(err)
		if !ok || c.codexFallback == nil {
			return CompletionResponse{}, err
		}

		resp, nextMode, fallbackErr := c.tryOneFallback(ctx, req, fallbackAttempt{
			mode:            "codex",
			client:          c.codexFallback,
			currentProvider: "openai",
			targetProvider:  "openai",
			targetRuntime:   "codex",
			targetModel:     c.cfg.LLMModel,
			reason:          reason,
			note:            "If approved, Codex CLI will be used for the remainder of this command instead of the OpenAI API.",
		})
		if fallbackErr != nil {
			return CompletionResponse{}, fallbackErr
		}

		c.setActiveMode(nextMode)
		return resp, nil
	case "codex":
		return c.completeWithNamedFallback(ctx, req, "codex", c.codexFallback)
	}

	resp, err := c.primary.Complete(ctx, req)
	if err == nil {
		return resp, nil
	}

	reason, ok := shouldFallbackToOpenAI(err)
	if !ok {
		return CompletionResponse{}, err
	}
	resp, activeMode, fallbackErr := c.tryApprovedFallbacks(ctx, req, reason, "anthropic", err)
	if fallbackErr != nil {
		return CompletionResponse{}, fallbackErr
	}

	c.setActiveMode(activeMode)

	return resp, nil
}

func (c *FallbackClient) setActiveMode(mode string) {
	c.fallbackLock.Lock()
	c.activeMode = mode
	c.fallbackLock.Unlock()
}

func (c *FallbackClient) completeWithNamedFallback(ctx context.Context, req CompletionRequest, mode string, client Client) (CompletionResponse, error) {
	if client == nil {
		return CompletionResponse{}, fmt.Errorf("fallback mode %q is not configured", mode)
	}
	fallbackReq := req
	if fallbackReq.Model == "" {
		fallbackReq.Model = c.cfg.LLMModel
	}
	return client.Complete(ctx, fallbackReq)
}

func shouldFallbackToOpenAI(err error) (string, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if reason, ok := modelswitch.DetectClaudeExhaustion(apiErr.Message); ok {
			return reason, true
		}
	}

	if reason, ok := modelswitch.DetectClaudeExhaustion(err.Error()); ok {
		return reason, true
	}

	// Claude CLI failures may be wrapped with extra prefix text.
	if reason, ok := modelswitch.DetectClaudeExhaustion(strings.ToLower(err.Error())); ok {
		return reason, true
	}

	return "", false
}

func shouldFallbackFromOpenAIAPI(err error) (string, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if reason, ok := detectOpenAIQuotaExhaustion(apiErr.Message); ok {
			return reason, true
		}
	}

	if reason, ok := detectOpenAIQuotaExhaustion(err.Error()); ok {
		return reason, true
	}
	return "", false
}

func detectOpenAIQuotaExhaustion(text string) (string, bool) {
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "exceeded your current quota", "check your plan and billing"):
		return "OpenAI API quota is exhausted", true
	case containsAny(lower, "insufficient_quota", "quota exhausted"):
		return "OpenAI API quota is exhausted", true
	}
	return "", false
}

func (c *FallbackClient) tryApprovedFallbacks(
	ctx context.Context,
	req CompletionRequest,
	reason string,
	currentProvider string,
	originalErr error,
) (CompletionResponse, string, error) {
	if c.apiFallback != nil {
		resp, mode, err := c.tryOneFallback(ctx, req, fallbackAttempt{
			mode:            "api",
			client:          c.apiFallback,
			currentProvider: currentProvider,
			targetProvider:  "openai",
			targetRuntime:   "api",
			targetModel:     c.cfg.LLMModel,
			reason:          reason,
			note:            "If approved, OpenAI API will be used for the remainder of this command.",
		})
		if err == nil {
			return resp, mode, nil
		}
		if nextReason, ok := shouldFallbackFromOpenAIAPI(err); ok && c.codexFallback != nil {
			return c.tryOneFallback(ctx, req, fallbackAttempt{
				mode:            "codex",
				client:          c.codexFallback,
				currentProvider: "openai",
				targetProvider:  "openai",
				targetRuntime:   "codex",
				targetModel:     c.cfg.LLMModel,
				reason:          nextReason,
				note:            "If approved, Codex CLI will be used for the remainder of this command instead of the OpenAI API.",
			})
		}
		return CompletionResponse{}, "", err
	}

	if c.codexFallback != nil {
		return c.tryOneFallback(ctx, req, fallbackAttempt{
			mode:            "codex",
			client:          c.codexFallback,
			currentProvider: currentProvider,
			targetProvider:  "openai",
			targetRuntime:   "codex",
			targetModel:     c.cfg.LLMModel,
			reason:          reason,
			note:            "If approved, Codex CLI will be used for the remainder of this command.",
		})
	}

	return CompletionResponse{}, "", fmt.Errorf(
		"claude is unavailable (%s), but no approved OpenAI fallback is configured: %w",
		reason, originalErr,
	)
}

type fallbackAttempt struct {
	mode            string
	client          Client
	currentProvider string
	targetProvider  string
	targetRuntime   string
	targetModel     string
	reason          string
	note            string
}

func (c *FallbackClient) tryOneFallback(
	ctx context.Context,
	req CompletionRequest,
	attempt fallbackAttempt,
) (CompletionResponse, string, error) {
	if c.cfg.RequireApproval && c.approver != nil {
		approved, approveErr := c.approver.ApproveSwitch(modelswitch.Request{
			Scope:           modelswitch.ScopeLLM,
			Operation:       "planner/review/rebase LLM call",
			CurrentProvider: attempt.currentProvider,
			TargetProvider:  attempt.targetProvider,
			TargetRuntime:   attempt.targetRuntime,
			TargetModel:     attempt.targetModel,
			Reason:          attempt.reason,
			Note:            attempt.note,
		})
		if approveErr != nil {
			return CompletionResponse{}, "", fmt.Errorf("%s fallback approval failed: %w", attempt.mode, approveErr)
		}
		if !approved {
			return CompletionResponse{}, "", fmt.Errorf(
				"%s fallback was declined after %s",
				attempt.mode, attempt.reason,
			)
		}
	}

	resp, err := c.completeWithNamedFallback(ctx, req, attempt.mode, attempt.client)
	if err != nil {
		return CompletionResponse{}, "", err
	}
	return resp, attempt.mode, nil
}

// HasCodexCLI reports whether the codex binary is available on PATH.
func HasCodexCLI() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}
