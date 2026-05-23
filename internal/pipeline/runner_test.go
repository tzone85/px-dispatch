package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/state"
)

// errTestSentinel is a known error value used to verify error propagation.
var errTestSentinel = errors.New("test sentinel error")

// mockStage is a configurable test stage.
type mockStage struct {
	name    string
	results []StageResult // returns results in order, cycling
	errors  []error
	calls   int
}

func newMockStage(name string, results ...StageResult) *mockStage {
	errs := make([]error, len(results))
	return &mockStage{name: name, results: results, errors: errs}
}

func newMockStageWithError(name string, result StageResult, err error) *mockStage {
	return &mockStage{name: name, results: []StageResult{result}, errors: []error{err}}
}

func (m *mockStage) Name() string { return m.name }

func (m *mockStage) Execute(_ context.Context, _ StoryContext) (StageResult, error) {
	idx := m.calls % len(m.results)
	m.calls++
	return m.results[idx], m.errors[idx]
}

// mockEventStore captures events emitted during a pipeline run.
type mockEventStore struct {
	events []state.Event
}

func (m *mockEventStore) Append(evt state.Event) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *mockEventStore) List(_ state.EventFilter) ([]state.Event, error) {
	return nil, nil
}

func (m *mockEventStore) Count(_ state.EventFilter) (int, error) { return 0, nil }

func (m *mockEventStore) All() ([]state.Event, error) { return nil, nil }

func TestRunner_AllStagesPass(t *testing.T) {
	stages := []Stage{
		newMockStage("autocommit", StagePassed),
		newMockStage("diffcheck", StagePassed),
		newMockStage("review", StagePassed),
		newMockStage("merge", StagePassed),
	}
	es := &mockEventStore{}
	runner := NewRunner(stages, config.PipelineConfig{}, es)

	result, err := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result != StagePassed {
		t.Errorf("expected Passed, got %s", result)
	}
}

func TestRunner_StageFailsWithRetry(t *testing.T) {
	// Review fails once, then passes on retry.
	review := newMockStage("review", StageFailed, StagePassed)
	stages := []Stage{
		newMockStage("diffcheck", StagePassed),
		review,
		newMockStage("merge", StagePassed),
	}

	es := &mockEventStore{}
	cfg := config.PipelineConfig{
		Stages: map[string]config.StageConfig{
			"review": {MaxRetries: 2, OnExhaust: "pause_requirement"},
		},
	}
	runner := NewRunner(stages, cfg, es)

	result, _ := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	if result != StagePassed {
		t.Errorf("expected Passed after retry, got %s", result)
	}
	if review.calls != 2 {
		t.Errorf("expected 2 review calls, got %d", review.calls)
	}
}

func TestRunner_StageExhaustsRetries(t *testing.T) {
	// Review always fails.
	stages := []Stage{
		newMockStage("diffcheck", StagePassed),
		newMockStage("review", StageFailed),
	}

	es := &mockEventStore{}
	cfg := config.PipelineConfig{
		Stages: map[string]config.StageConfig{
			"review": {MaxRetries: 2, OnExhaust: "pause_requirement"},
		},
	}
	runner := NewRunner(stages, cfg, es)

	result, _ := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	if result != StageFatal {
		t.Errorf("expected Fatal after retry exhaustion, got %s", result)
	}
}

func TestRunner_FatalStopsImmediately(t *testing.T) {
	merge := newMockStage("merge", StagePassed)
	stages := []Stage{
		newMockStage("review", StageFatal),
		merge,
	}

	es := &mockEventStore{}
	runner := NewRunner(stages, config.PipelineConfig{}, es)

	result, _ := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	if result != StageFatal {
		t.Errorf("expected Fatal, got %s", result)
	}
	if merge.calls != 0 {
		t.Error("merge should not have been called after fatal")
	}
}

func TestRunner_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	stages := []Stage{
		newMockStage("review", StagePassed),
	}
	es := &mockEventStore{}
	runner := NewRunner(stages, config.PipelineConfig{}, es)

	_, err := runner.Run(ctx, StoryContext{StoryID: "s-1"})
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

func TestRunner_EmitsEvents(t *testing.T) {
	stages := []Stage{
		newMockStage("review", StagePassed),
	}
	es := &mockEventStore{}
	runner := NewRunner(stages, config.PipelineConfig{}, es)

	_, _ = runner.Run(context.Background(), StoryContext{StoryID: "s-1"})

	// Should have emitted stage transition events.
	if len(es.events) == 0 {
		t.Error("expected events to be emitted")
	}
}

func TestRunner_OnExhaustEscalate(t *testing.T) {
	stages := []Stage{
		newMockStage("review", StageFailed),
	}
	es := &mockEventStore{}
	cfg := config.PipelineConfig{
		Stages: map[string]config.StageConfig{
			"review": {MaxRetries: 1, OnExhaust: "escalate"},
		},
	}
	runner := NewRunner(stages, cfg, es)

	result, _ := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	// Escalate should emit an escalation event and return Failed (not Fatal)
	// so the story can be re-dispatched to a senior agent.
	if result != StageFailed {
		t.Errorf("expected Failed for escalation, got %s", result)
	}
	// Check an escalation event was emitted.
	foundEscalation := false
	for _, evt := range es.events {
		if evt.Type == state.EventEscalationCreated {
			foundEscalation = true
		}
	}
	if !foundEscalation {
		t.Error("expected escalation event")
	}
}

func TestRunner_StageErrorPropagated(t *testing.T) {
	// A stage that returns an error along with a fatal result.
	errStage := newMockStageWithError("broken", StageFatal, errTestSentinel)
	stages := []Stage{errStage}
	es := &mockEventStore{}
	runner := NewRunner(stages, config.PipelineConfig{}, es)

	result, err := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	if result != StageFatal {
		t.Errorf("expected Fatal, got %s", result)
	}
	if err != errTestSentinel {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestRunner_DefaultRetryIsOneAttempt(t *testing.T) {
	// A stage with no explicit config should get exactly one attempt.
	review := newMockStage("review", StageFailed)
	stages := []Stage{review}
	es := &mockEventStore{}
	runner := NewRunner(stages, config.PipelineConfig{}, es)

	result, _ := runner.Run(context.Background(), StoryContext{StoryID: "s-1"})
	if result != StageFatal {
		t.Errorf("expected Fatal (default pause on exhaustion), got %s", result)
	}
	if review.calls != 1 {
		t.Errorf("expected 1 call (default single attempt), got %d", review.calls)
	}
}

func TestStageResult_String(t *testing.T) {
	tests := []struct {
		result StageResult
		want   string
	}{
		{StagePassed, "passed"},
		{StageFailed, "failed"},
		{StageFatal, "fatal"},
		{StageResult(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.result.String(); got != tt.want {
			t.Errorf("StageResult(%d).String() = %q, want %q", tt.result, got, tt.want)
		}
	}
}
