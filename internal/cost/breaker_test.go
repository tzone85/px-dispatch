package cost

import (
	"context"
	"errors"
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/llm"
	"github.com/tzone85/px-dispatch/internal/state"
)

// mockLLMClient returns a fixed response.
type mockLLMClient struct {
	response llm.CompletionResponse
	err      error
	called   bool
}

func (m *mockLLMClient) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	m.called = true
	return m.response, m.err
}

// mockLedger tracks recorded usage and returns configurable query results.
type mockLedger struct {
	recorded  []TokenUsage
	storyCost float64
	reqCost   float64
	dayCost   float64
}

func (m *mockLedger) Record(u TokenUsage) error {
	m.recorded = append(m.recorded, u)
	return nil
}
func (m *mockLedger) QueryByStory(_ string) (float64, error)       { return m.storyCost, nil }
func (m *mockLedger) QueryByRequirement(_ string) (float64, error) { return m.reqCost, nil }
func (m *mockLedger) QueryByDay(_ string) (float64, error)         { return m.dayCost, nil }

// mockEventStore captures appended events for assertion.
type mockEventStore struct {
	events []state.Event
}

func (m *mockEventStore) Append(evt state.Event) error {
	m.events = append(m.events, evt)
	return nil
}
func (m *mockEventStore) List(_ state.EventFilter) ([]state.Event, error) { return nil, nil }
func (m *mockEventStore) Count(_ state.EventFilter) (int, error)          { return 0, nil }
func (m *mockEventStore) All() ([]state.Event, error)                     { return nil, nil }

// helper to build a default test breaker with sensible defaults.
func newTestBreaker(
	inner llm.Client,
	ledger Ledger,
	budgetCfg config.BudgetConfig,
	eventStore state.EventStore,
) *BudgetBreaker {
	pricing := map[string]PricingEntry{
		"test-model": {InputPer1M: 3.0, OutputPer1M: 15.0},
	}
	budgetCtx := BudgetContext{
		StoryID: "story-1",
		ReqID:   "req-1",
		AgentID: "agent-1",
		Stage:   "planning",
	}
	return NewBudgetBreaker(inner, ledger, budgetCfg, pricing, eventStore, budgetCtx)
}

func defaultBudgetCfg() config.BudgetConfig {
	return config.BudgetConfig{
		MaxCostPerStoryUSD:       2.0,
		MaxCostPerRequirementUSD: 20.0,
		MaxCostPerDayUSD:         50.0,
		WarningThresholdPct:      80,
		HardStop:                 true,
	}
}

func defaultResponse() llm.CompletionResponse {
	return llm.CompletionResponse{
		Content:      "hello",
		Model:        "test-model",
		InputTokens:  1000,
		OutputTokens: 500,
	}
}

func TestBreaker_AllowsWithinBudget(t *testing.T) {
	inner := &mockLLMClient{response: defaultResponse()}
	ledger := &mockLedger{storyCost: 0.5, reqCost: 5.0, dayCost: 10.0}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	resp, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", resp.Content)
	}
	if !inner.called {
		t.Error("expected inner client to be called")
	}
	if len(ledger.recorded) != 1 {
		t.Fatalf("expected 1 recorded usage, got %d", len(ledger.recorded))
	}
}

func TestBreaker_BlocksStoryBudget(t *testing.T) {
	inner := &mockLLMClient{response: defaultResponse()}
	ledger := &mockLedger{storyCost: 2.0, reqCost: 5.0, dayCost: 10.0}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var budgetErr *llm.BudgetExhaustedError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExhaustedError, got %T: %v", err, err)
	}
	if budgetErr.BudgetType != "story" {
		t.Errorf("expected budget type 'story', got %q", budgetErr.BudgetType)
	}
	if budgetErr.UsedUSD != 2.0 {
		t.Errorf("expected used $2.00, got $%.2f", budgetErr.UsedUSD)
	}
	if budgetErr.LimitUSD != 2.0 {
		t.Errorf("expected limit $2.00, got $%.2f", budgetErr.LimitUSD)
	}
	if inner.called {
		t.Error("inner client should NOT be called when budget is exhausted")
	}
}

func TestBreaker_BlocksRequirementBudget(t *testing.T) {
	inner := &mockLLMClient{response: defaultResponse()}
	ledger := &mockLedger{storyCost: 0.5, reqCost: 20.0, dayCost: 10.0}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var budgetErr *llm.BudgetExhaustedError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExhaustedError, got %T: %v", err, err)
	}
	if budgetErr.BudgetType != "requirement" {
		t.Errorf("expected budget type 'requirement', got %q", budgetErr.BudgetType)
	}
	if inner.called {
		t.Error("inner client should NOT be called when budget is exhausted")
	}
}

func TestBreaker_BlocksDailyBudget(t *testing.T) {
	inner := &mockLLMClient{response: defaultResponse()}
	ledger := &mockLedger{storyCost: 0.5, reqCost: 5.0, dayCost: 50.0}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var budgetErr *llm.BudgetExhaustedError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExhaustedError, got %T: %v", err, err)
	}
	if budgetErr.BudgetType != "daily" {
		t.Errorf("expected budget type 'daily', got %q", budgetErr.BudgetType)
	}
	if inner.called {
		t.Error("inner client should NOT be called when budget is exhausted")
	}
}

func TestBreaker_RecordsUsageAfterSuccess(t *testing.T) {
	resp := llm.CompletionResponse{
		Content:      "result",
		Model:        "test-model",
		InputTokens:  2000,
		OutputTokens: 1000,
	}
	inner := &mockLLMClient{response: resp}
	ledger := &mockLedger{}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ledger.recorded) != 1 {
		t.Fatalf("expected 1 recorded usage, got %d", len(ledger.recorded))
	}

	usage := ledger.recorded[0]
	if usage.StoryID != "story-1" {
		t.Errorf("expected story ID 'story-1', got %q", usage.StoryID)
	}
	if usage.ReqID != "req-1" {
		t.Errorf("expected req ID 'req-1', got %q", usage.ReqID)
	}
	if usage.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got %q", usage.AgentID)
	}
	if usage.Model != "test-model" {
		t.Errorf("expected model 'test-model', got %q", usage.Model)
	}
	if usage.InputTokens != 2000 {
		t.Errorf("expected 2000 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 1000 {
		t.Errorf("expected 1000 output tokens, got %d", usage.OutputTokens)
	}
	if usage.Stage != "planning" {
		t.Errorf("expected stage 'planning', got %q", usage.Stage)
	}
}

func TestBreaker_DoesNotRecordOnError(t *testing.T) {
	inner := &mockLLMClient{err: errors.New("api failure")}
	ledger := &mockLedger{}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(ledger.recorded) != 0 {
		t.Errorf("expected no recorded usage on error, got %d", len(ledger.recorded))
	}
}

func TestBreaker_EmitsWarningAtThreshold(t *testing.T) {
	// Story cost at 80% of $2.00 = $1.60
	// The warning should trigger after the call completes and we re-evaluate.
	// We set storyCost to $1.50, then the call adds some cost to push past 80%.
	// After the call, total cost (existing + new) should exceed 80%.
	// With test-model: 1000 input * 3.0/1M + 500 output * 15.0/1M = 0.003 + 0.0075 = 0.0105
	// Total: 1.50 + 0.0105 = 1.5105, which is 75.5% of 2.0 — not enough.
	// Let's set storyCost to 1.60 so total = 1.6105, which is 80.5% of 2.0.
	inner := &mockLLMClient{response: defaultResponse()}
	ledger := &mockLedger{storyCost: 1.60, reqCost: 5.0, dayCost: 10.0}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have emitted a budget warning event.
	found := false
	for _, evt := range events.events {
		if evt.Type == state.EventBudgetWarning {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected EventBudgetWarning to be emitted when cost hits 80% threshold")
	}
}

func TestBreaker_PassesThroughInnerError(t *testing.T) {
	expectedErr := &llm.APIError{
		StatusCode: 500,
		Message:    "internal server error",
		Retryable:  true,
	}
	inner := &mockLLMClient{err: expectedErr}
	ledger := &mockLedger{}
	events := &mockEventStore{}

	breaker := newTestBreaker(inner, ledger, defaultBudgetCfg(), events)
	_, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "internal server error" {
		t.Errorf("expected message 'internal server error', got %q", apiErr.Message)
	}
}

func TestBreaker_HardStopDisabled(t *testing.T) {
	inner := &mockLLMClient{response: defaultResponse()}
	// All budgets exceeded.
	ledger := &mockLedger{storyCost: 100.0, reqCost: 100.0, dayCost: 100.0}
	events := &mockEventStore{}

	cfg := defaultBudgetCfg()
	cfg.HardStop = false

	breaker := newTestBreaker(inner, ledger, cfg, events)
	resp, err := breaker.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("expected no error with HardStop disabled, got %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", resp.Content)
	}
	if !inner.called {
		t.Error("expected inner client to be called even when over budget with HardStop disabled")
	}
	if len(ledger.recorded) != 1 {
		t.Errorf("expected usage to still be recorded, got %d records", len(ledger.recorded))
	}
}

