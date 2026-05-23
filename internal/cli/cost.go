package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/cost"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newCostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cost [req-id]",
		Short: "Show cost breakdown by story, requirement, and daily total",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runCost,
	}
}

func runCost(cmd *cobra.Command, args []string) error {
	ledger := cost.NewSQLiteLedger(app.projStore.DB(), cost.DefaultPricing)

	today := time.Now().Format("2006-01-02")
	dailyCost, err := ledger.QueryByDay(today)
	if err != nil {
		return fmt.Errorf("query daily cost: %w", err)
	}
	dailyLimit := app.config.Budget.MaxCostPerDayUSD

	fmt.Printf("Daily Cost (%s)\n", today)
	fmt.Printf("  %s $%.4f / $%.2f\n\n", budgetBar(dailyCost, dailyLimit, 20), dailyCost, dailyLimit)

	if len(args) == 1 {
		return showReqCost(ledger, args[0])
	}

	return showAllReqCosts(ledger)
}

// showAllReqCosts lists all non-archived requirements with cost per requirement.
func showAllReqCosts(ledger *cost.SQLiteLedger) error {
	reqs, err := app.projStore.ListRequirements(state.ReqFilter{ExcludeArchived: true})
	if err != nil {
		return fmt.Errorf("list requirements: %w", err)
	}

	if len(reqs) == 0 {
		fmt.Println("No requirements found.")
		return nil
	}

	reqLimit := app.config.Budget.MaxCostPerRequirementUSD

	fmt.Println("Requirements:")
	for _, req := range reqs {
		reqCost, err := ledger.QueryByRequirement(req.ID)
		if err != nil {
			return fmt.Errorf("query cost for req %s: %w", req.ID, err)
		}
		fmt.Printf("  [%s] %-40s  %s $%.4f / $%.2f\n",
			req.ID,
			truncate(req.Title, 40),
			budgetBar(reqCost, reqLimit, 10),
			reqCost,
			reqLimit,
		)
	}

	return nil
}

// showReqCost shows cost per story within the given requirement.
func showReqCost(ledger *cost.SQLiteLedger, reqID string) error {
	req, err := app.projStore.GetRequirement(reqID)
	if err != nil {
		return fmt.Errorf("get requirement: %w", err)
	}

	stories, err := app.projStore.ListStories(state.StoryFilter{ReqID: reqID})
	if err != nil {
		return fmt.Errorf("list stories for req %s: %w", reqID, err)
	}

	reqCost, err := ledger.QueryByRequirement(reqID)
	if err != nil {
		return fmt.Errorf("query requirement cost: %w", err)
	}

	reqLimit := app.config.Budget.MaxCostPerRequirementUSD
	storyLimit := app.config.Budget.MaxCostPerStoryUSD

	fmt.Printf("Requirement: [%s] %s\n", req.ID, req.Title)
	fmt.Printf("Total Cost:  %s $%.4f / $%.2f\n\n", budgetBar(reqCost, reqLimit, 20), reqCost, reqLimit)

	if len(stories) == 0 {
		fmt.Println("No stories found.")
		return nil
	}

	fmt.Println("Stories:")
	for _, st := range stories {
		storyCost, err := ledger.QueryByStory(st.ID)
		if err != nil {
			return fmt.Errorf("query cost for story %s: %w", st.ID, err)
		}
		fmt.Printf("  [%s] %-40s  %s $%.4f / $%.2f\n",
			st.ID,
			truncate(st.Title, 40),
			budgetBar(storyCost, storyLimit, 10),
			storyCost,
			storyLimit,
		)
	}

	return nil
}

// budgetBar renders a text progress bar with a status indicator.
// Indicators: [!!] for >= 80%, [! ] for >= 60%, [ok] for < 60%.
func budgetBar(used, limit float64, width int) string {
	if limit <= 0 {
		return "[??] [" + strings.Repeat("?", width) + "]"
	}

	pct := used / limit
	if pct > 1 {
		pct = 1
	}

	filled := int(pct * float64(width))
	empty := width - filled
	bar := strings.Repeat("#", filled) + strings.Repeat(".", empty)

	switch {
	case pct >= 0.8:
		return "[!!] [" + bar + "]"
	case pct >= 0.6:
		return "[! ] [" + bar + "]"
	default:
		return "[ok] [" + bar + "]"
	}
}

// truncate shortens s to at most n runes, appending "..." if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}
