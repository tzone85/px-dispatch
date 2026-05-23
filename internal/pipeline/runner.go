package pipeline

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/state"
)

// Runner executes pipeline stages sequentially for a story.
// Each stage is retried according to its per-stage configuration.
// The runner is safe for concurrent use across different story contexts.
type Runner struct {
	stages    []Stage
	stagesCfg map[string]config.StageConfig
	events    state.EventStore
}

// NewRunner creates a pipeline runner with the given stages, configuration,
// and event store. Stages execute in the order provided.
func NewRunner(stages []Stage, cfg config.PipelineConfig, es state.EventStore) *Runner {
	stageCfg := make(map[string]config.StageConfig, len(cfg.Stages))
	for k, v := range cfg.Stages {
		stageCfg[k] = v
	}
	return &Runner{
		stages:    stages,
		stagesCfg: stageCfg,
		events:    es,
	}
}

// Run executes all stages for the given story context.
//
// Returns StagePassed if every stage passes, StageFatal if any stage is fatal
// or exhausts retries with a pause policy, and StageFailed if escalation is
// triggered after retry exhaustion.
//
// Context cancellation is checked before each stage for graceful shutdown.
func (r *Runner) Run(ctx context.Context, sc StoryContext) (StageResult, error) {
	for _, stage := range r.stages {
		if err := ctx.Err(); err != nil {
			return StageFatal, err
		}

		result, err := r.executeWithRetries(ctx, stage, sc)
		if result != StagePassed {
			return result, err
		}
	}
	return StagePassed, nil
}

// executeWithRetries runs a single stage up to maxRetries times, applying the
// on_exhaust policy when retries are exhausted.
func (r *Runner) executeWithRetries(ctx context.Context, stage Stage, sc StoryContext) (StageResult, error) {
	cfg := r.stagesCfg[stage.Name()]
	maxAttempts := cfg.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		r.emitStageEvent(sc, stage.Name(), "started", attempt)

		result, err := stage.Execute(ctx, sc)
		lastErr = err

		r.emitStageEvent(sc, stage.Name(), result.String(), attempt)

		switch result {
		case StagePassed:
			return StagePassed, nil
		case StageFatal:
			return StageFatal, err
		case StageFailed:
			if attempt < maxAttempts {
				slog.Warn("stage failed, retrying",
					"stage", stage.Name(),
					"story", sc.StoryID,
					"attempt", attempt,
					"max_attempts", maxAttempts,
				)
				continue
			}
		}
	}

	// All retries exhausted with StageFailed.
	return r.handleExhaustion(sc, stage.Name(), cfg.OnExhaust, lastErr)
}

// handleExhaustion applies the on_exhaust policy after all retries are consumed.
func (r *Runner) handleExhaustion(
	sc StoryContext,
	stageName string,
	onExhaust string,
	lastErr error,
) (StageResult, error) {
	switch onExhaust {
	case "escalate":
		r.events.Append(state.NewEvent(
			state.EventEscalationCreated,
			"pipeline",
			sc.StoryID,
			map[string]any{
				"reason": fmt.Sprintf("stage %s exhausted retries", stageName),
				"stage":  stageName,
			},
		))
		return StageFailed, lastErr

	default: // "pause_requirement" or unspecified
		return StageFatal, lastErr
	}
}

// emitStageEvent records a stage transition event. Emission is best-effort;
// errors are logged but do not fail the pipeline.
func (r *Runner) emitStageEvent(sc StoryContext, stageName, status string, attempt int) {
	err := r.events.Append(state.NewEvent(
		state.EventStoryProgress,
		"pipeline",
		sc.StoryID,
		map[string]any{
			"stage":   stageName,
			"status":  status,
			"attempt": attempt,
		},
	))
	if err != nil {
		slog.Warn("failed to emit pipeline event",
			"stage", stageName,
			"status", status,
			"error", err,
		)
	}
}
