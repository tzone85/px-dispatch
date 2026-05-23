package cost

import (
	"context"
	"time"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/llm"
	"github.com/tzone85/px-dispatch/internal/state"
)

// BudgetContext provides the story/requirement context for budget checking.
// It is immutable per breaker instance — each pipeline stage creates its own
// breaker with the appropriate context.
type BudgetContext struct {
	StoryID string
	ReqID   string
	AgentID string
	Stage   string // planning, review, qa, conflict_resolution
}

// BudgetBreaker wraps an llm.Client with pre-call budget checks.
// Before each LLM call, it checks story, requirement, and daily budgets.
// After each successful call, it records usage in the ledger.
type BudgetBreaker struct {
	inner      llm.Client
	ledger     Ledger
	budgetCfg  config.BudgetConfig
	pricing    map[string]PricingEntry
	eventStore state.EventStore
	budgetCtx  BudgetContext
}

// Compile-time check that BudgetBreaker implements llm.Client.
var _ llm.Client = (*BudgetBreaker)(nil)

// NewBudgetBreaker creates a new BudgetBreaker that wraps the given inner
// client with budget enforcement. The pricing map is used to compute the
// dollar cost of each completion from its token counts.
func NewBudgetBreaker(
	inner llm.Client,
	ledger Ledger,
	budgetCfg config.BudgetConfig,
	pricing map[string]PricingEntry,
	eventStore state.EventStore,
	budgetCtx BudgetContext,
) *BudgetBreaker {
	return &BudgetBreaker{
		inner:      inner,
		ledger:     ledger,
		budgetCfg:  budgetCfg,
		pricing:    pricing,
		eventStore: eventStore,
		budgetCtx:  budgetCtx,
	}
}

// Complete performs budget checks, delegates to the inner client, records
// usage on success, and emits warning events when thresholds are crossed.
func (b *BudgetBreaker) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if b.budgetCfg.HardStop {
		if err := b.checkBudgets(); err != nil {
			return llm.CompletionResponse{}, err
		}
	}

	resp, err := b.inner.Complete(ctx, req)
	if err != nil {
		return llm.CompletionResponse{}, err
	}

	usage := TokenUsage{
		ReqID:        b.budgetCtx.ReqID,
		StoryID:      b.budgetCtx.StoryID,
		AgentID:      b.budgetCtx.AgentID,
		Model:        resp.Model,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		Stage:        b.budgetCtx.Stage,
	}

	if recordErr := b.ledger.Record(usage); recordErr != nil {
		return llm.CompletionResponse{}, recordErr
	}

	b.emitWarningsIfNeeded()

	return resp, nil
}

// checkBudgets queries the ledger for current spending and returns a
// BudgetExhaustedError if any limit has been reached or exceeded.
// Budget checks are ordered: story, requirement, daily.
func (b *BudgetBreaker) checkBudgets() error {
	storyCost, err := b.ledger.QueryByStory(b.budgetCtx.StoryID)
	if err != nil {
		return err
	}
	if storyCost >= b.budgetCfg.MaxCostPerStoryUSD {
		return &llm.BudgetExhaustedError{
			BudgetType: "story",
			UsedUSD:    storyCost,
			LimitUSD:   b.budgetCfg.MaxCostPerStoryUSD,
		}
	}

	reqCost, err := b.ledger.QueryByRequirement(b.budgetCtx.ReqID)
	if err != nil {
		return err
	}
	if reqCost >= b.budgetCfg.MaxCostPerRequirementUSD {
		return &llm.BudgetExhaustedError{
			BudgetType: "requirement",
			UsedUSD:    reqCost,
			LimitUSD:   b.budgetCfg.MaxCostPerRequirementUSD,
		}
	}

	today := time.Now().UTC().Format("2006-01-02")
	dayCost, err := b.ledger.QueryByDay(today)
	if err != nil {
		return err
	}
	if dayCost >= b.budgetCfg.MaxCostPerDayUSD {
		return &llm.BudgetExhaustedError{
			BudgetType: "daily",
			UsedUSD:    dayCost,
			LimitUSD:   b.budgetCfg.MaxCostPerDayUSD,
		}
	}

	return nil
}

// emitWarningsIfNeeded re-queries the ledger (which now includes the
// just-recorded usage) and emits an EventBudgetWarning if any budget
// has crossed the warning threshold percentage.
func (b *BudgetBreaker) emitWarningsIfNeeded() {
	thresholdPct := float64(b.budgetCfg.WarningThresholdPct) / 100.0

	// Check story budget warning.
	storyCost, err := b.ledger.QueryByStory(b.budgetCtx.StoryID)
	if err == nil && storyCost >= b.budgetCfg.MaxCostPerStoryUSD*thresholdPct {
		pct := int(storyCost / b.budgetCfg.MaxCostPerStoryUSD * 100)
		b.emitWarning(storyCost, b.budgetCfg.MaxCostPerStoryUSD, pct)
		return
	}

	// Check requirement budget warning.
	reqCost, err := b.ledger.QueryByRequirement(b.budgetCtx.ReqID)
	if err == nil && reqCost >= b.budgetCfg.MaxCostPerRequirementUSD*thresholdPct {
		pct := int(reqCost / b.budgetCfg.MaxCostPerRequirementUSD * 100)
		b.emitWarning(reqCost, b.budgetCfg.MaxCostPerRequirementUSD, pct)
		return
	}

	// Check daily budget warning.
	today := time.Now().UTC().Format("2006-01-02")
	dayCost, err := b.ledger.QueryByDay(today)
	if err == nil && dayCost >= b.budgetCfg.MaxCostPerDayUSD*thresholdPct {
		pct := int(dayCost / b.budgetCfg.MaxCostPerDayUSD * 100)
		b.emitWarning(dayCost, b.budgetCfg.MaxCostPerDayUSD, pct)
		return
	}
}

// emitWarning appends a budget warning event to the event store.
func (b *BudgetBreaker) emitWarning(usedUSD, limitUSD float64, percentage int) {
	evt := state.NewEvent(
		state.EventBudgetWarning,
		b.budgetCtx.AgentID,
		b.budgetCtx.StoryID,
		map[string]any{
			"req_id":     b.budgetCtx.ReqID,
			"story_id":   b.budgetCtx.StoryID,
			"used_usd":   usedUSD,
			"limit_usd":  limitUSD,
			"percentage": percentage,
		},
	)
	// Best-effort: budget warnings should not fail the call.
	_ = b.eventStore.Append(evt)
}
