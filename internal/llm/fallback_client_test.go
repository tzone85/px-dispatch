package llm

import (
	"context"
	"fmt"
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/modelswitch"
)

type stubClient struct {
	resp    CompletionResponse
	err     error
	results []stubResult
	calls   int
	reqs    []CompletionRequest
}

type stubResult struct {
	resp CompletionResponse
	err  error
}

func (c *stubClient) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	c.calls++
	c.reqs = append(c.reqs, req)
	if idx := c.calls - 1; idx < len(c.results) {
		result := c.results[idx]
		if result.err != nil {
			return CompletionResponse{}, result.err
		}
		return result.resp, nil
	}
	if c.err != nil {
		return CompletionResponse{}, c.err
	}
	return c.resp, nil
}

type stubApprover struct {
	approved  bool
	approvals []bool
	calls     int
	reqs      []modelswitch.Request
}

func (a *stubApprover) ApproveSwitch(req modelswitch.Request) (bool, error) {
	a.calls++
	a.reqs = append(a.reqs, req)
	if idx := a.calls - 1; idx < len(a.approvals) {
		return a.approvals[idx], nil
	}
	return a.approved, nil
}

func TestFallbackClient_SwitchesToOpenAIAfterApproval(t *testing.T) {
	primary := &stubClient{
		err: fmt.Errorf("claude CLI failed: credit balance exhausted"),
	}
	fallback := &stubClient{
		resp: CompletionResponse{Content: "ok", Model: "gpt-5.4"},
	}
	approver := &stubApprover{approved: true}

	client := NewFallbackClient(primary, fallback, config.FallbackConfig{
		Enabled:         true,
		RequireApproval: true,
		LLMModel:        "gpt-5.4",
	}, approver)

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "gpt-5.4" {
		t.Fatalf("expected fallback model, got %s", resp.Model)
	}
	if approver.calls != 1 {
		t.Fatalf("expected 1 approval prompt, got %d", approver.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("expected fallback client to be used once, got %d", fallback.calls)
	}

	// Second call should stay on fallback for the remainder of the command.
	_, err = client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "again"}},
	})
	if err != nil {
		t.Fatalf("unexpected sticky fallback error: %v", err)
	}
	if primary.calls != 1 {
		t.Fatalf("expected primary client to stop after first switch, got %d calls", primary.calls)
	}
	if fallback.calls != 2 {
		t.Fatalf("expected sticky fallback to reuse OpenAI, got %d calls", fallback.calls)
	}
	if fallback.reqs[0].Model != "gpt-5.4" {
		t.Fatalf("expected fallback request model to be injected, got %q", fallback.reqs[0].Model)
	}
}

func TestFallbackClient_DeclinedByUser(t *testing.T) {
	primary := &stubClient{
		err: fmt.Errorf("claude CLI failed: usage limit reached"),
	}
	fallback := &stubClient{
		resp: CompletionResponse{Content: "ok", Model: "gpt-5.4"},
	}
	approver := &stubApprover{approved: false}

	client := NewFallbackClient(primary, fallback, config.FallbackConfig{
		Enabled:         true,
		RequireApproval: true,
		LLMModel:        "gpt-5.4",
	}, approver)

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error when fallback is declined")
	}
	if fallback.calls != 0 {
		t.Fatal("fallback client should not be called when declined")
	}
}

func TestFallbackClient_SwitchesFromOpenAIAPIToCodexAfterQuotaApproval(t *testing.T) {
	primary := &stubClient{
		err: fmt.Errorf("claude CLI failed: usage limit reached"),
	}
	apiFallback := &stubClient{
		err: &APIError{
			StatusCode: 429,
			Message:    "You exceeded your current quota, please check your plan and billing details.",
			Retryable:  false,
		},
	}
	codexFallback := &stubClient{
		resp: CompletionResponse{Content: "ok", Model: "gpt-5.4"},
	}
	approver := &stubApprover{approvals: []bool{true, true}}

	client := NewFallbackClient(primary, apiFallback, config.FallbackConfig{
		Enabled:         true,
		RequireApproval: true,
		LLMModel:        "gpt-5.4",
	}, approver).WithCodexCLI(codexFallback)

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "gpt-5.4" {
		t.Fatalf("expected codex fallback model, got %s", resp.Model)
	}
	if approver.calls != 2 {
		t.Fatalf("expected 2 approval prompts, got %d", approver.calls)
	}
	if got := approver.reqs[0].TargetRuntime; got != "api" {
		t.Fatalf("expected first switch to API, got %q", got)
	}
	if got := approver.reqs[1].TargetRuntime; got != "codex" {
		t.Fatalf("expected second switch to codex, got %q", got)
	}
	if apiFallback.calls != 1 {
		t.Fatalf("expected API fallback client to be used once, got %d", apiFallback.calls)
	}
	if codexFallback.calls != 1 {
		t.Fatalf("expected codex fallback client to be used once, got %d", codexFallback.calls)
	}
	if codexFallback.reqs[0].Model != "gpt-5.4" {
		t.Fatalf("expected codex fallback request model to be injected, got %q", codexFallback.reqs[0].Model)
	}
}

func TestFallbackClient_SwitchesActiveOpenAIToCodexAfterQuota(t *testing.T) {
	primary := &stubClient{
		err: fmt.Errorf("claude CLI failed: usage limit reached"),
	}
	apiFallback := &stubClient{
		results: []stubResult{
			{resp: CompletionResponse{Content: "planned", Model: "gpt-5.4"}},
			{err: &APIError{
				StatusCode: 429,
				Message:    "You exceeded your current quota, please check your plan and billing details.",
				Retryable:  false,
			}},
		},
	}
	codexFallback := &stubClient{
		resp: CompletionResponse{Content: "continued", Model: "gpt-5.4"},
	}
	approver := &stubApprover{approvals: []bool{true, true}}

	client := NewFallbackClient(primary, apiFallback, config.FallbackConfig{
		Enabled:         true,
		RequireApproval: true,
		LLMModel:        "gpt-5.4",
	}, approver).WithCodexCLI(codexFallback)

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "first"}},
	})
	if err != nil {
		t.Fatalf("unexpected initial switch error: %v", err)
	}

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: RoleUser, Content: "second"}},
	})
	if err != nil {
		t.Fatalf("unexpected active fallback error: %v", err)
	}
	if resp.Content != "continued" {
		t.Fatalf("expected codex continuation response, got %q", resp.Content)
	}
	if primary.calls != 1 {
		t.Fatalf("expected primary client to be used once, got %d", primary.calls)
	}
	if apiFallback.calls != 2 {
		t.Fatalf("expected API fallback client to be used twice, got %d", apiFallback.calls)
	}
	if codexFallback.calls != 1 {
		t.Fatalf("expected codex fallback client to be used once, got %d", codexFallback.calls)
	}
	if approver.calls != 2 {
		t.Fatalf("expected 2 approval prompts, got %d", approver.calls)
	}
}
