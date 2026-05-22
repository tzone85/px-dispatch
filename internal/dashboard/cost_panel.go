package dashboard

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tzone85/project-x/internal/state"
)

// costData holds pre-queried cost information for rendering.
type costData struct {
	dailyTotal float64
	dailyLimit float64
	reqCosts   []reqCost
}

// reqCost pairs a requirement with its accumulated cost.
type reqCost struct {
	req  state.Requirement
	cost float64
}

// queryCostData fetches cost information from the database.
// Returns a zero-value costData (not an error) if the token_usage table
// has no rows, since this is a read-only dashboard.
func queryCostData(db *sql.DB, requirements []state.Requirement, dailyLimit float64) costData {
	data := costData{
		dailyLimit: dailyLimit,
	}

	// Query daily total.
	row := db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE date(created_at, 'localtime') = date('now', 'localtime')",
	)
	if err := row.Scan(&data.dailyTotal); err != nil {
		data.dailyTotal = 0
	}

	// Query per-requirement costs.
	for _, r := range requirements {
		var cost float64
		row := db.QueryRow(
			"SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE req_id = ?",
			r.ID,
		)
		if err := row.Scan(&cost); err != nil {
			cost = 0
		}
		data.reqCosts = append(data.reqCosts, reqCost{req: r, cost: cost})
	}

	return data
}

// renderCost produces a cost breakdown panel.
func renderCost(data costData) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Cost Overview"))
	b.WriteString("\n\n")

	// Daily total with budget bar.
	b.WriteString(renderBudgetLine("Daily", data.dailyTotal, data.dailyLimit))
	b.WriteString("\n\n")

	// Per-requirement costs.
	if len(data.reqCosts) == 0 {
		b.WriteString(dimStyle.Render("  No requirements tracked"))
		b.WriteString("\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorWhite).Render("  Per-Requirement Costs:"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  " + strings.Repeat("-", 50)))
		b.WriteString("\n")

		for _, rc := range data.reqCosts {
			title := rc.req.Title
			if len(title) > 30 {
				title = title[:27] + "..."
			}
			line := fmt.Sprintf("  %-10s %-30s %s",
				truncateID(rc.req.ID),
				title,
				costStyled(rc.cost),
			)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderBudgetLine shows a label, cost, limit, and a visual bar.
func renderBudgetLine(label string, used, limit float64) string {
	pct := 0.0
	if limit > 0 {
		pct = (used / limit) * 100
	}

	bar := renderBar(used, limit, 30)
	costStr := fmt.Sprintf("$%.2f / $%.2f (%.0f%%)", used, limit, pct)

	// Color the cost string based on percentage.
	var styledCost string
	switch {
	case pct >= 100:
		styledCost = errorStyle.Render(costStr)
	case pct >= 80:
		styledCost = warnStyle.Render(costStr)
	default:
		styledCost = lipgloss.NewStyle().Foreground(colorGreen).Render(costStr)
	}

	return fmt.Sprintf("  %s: %s %s", headerStyle.Render(label), styledCost, bar)
}

// renderBar produces a visual budget bar like [==========------].
func renderBar(used, limit float64, width int) string {
	if limit <= 0 {
		return dimStyle.Render("[" + strings.Repeat("-", width) + "]")
	}

	filled := int((used / limit) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	empty := width - filled

	var color lipgloss.Color
	pct := (used / limit) * 100
	switch {
	case pct >= 100:
		color = colorRed
	case pct >= 80:
		color = colorOrange
	case pct >= 50:
		color = colorYellow
	default:
		color = colorGreen
	}

	filledStr := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("=", filled))
	emptyStr := dimStyle.Render(strings.Repeat("-", empty))

	return "[" + filledStr + emptyStr + "]"
}

// costStyled returns a cost string colored by magnitude.
func costStyled(cost float64) string {
	costStr := fmt.Sprintf("$%.2f", cost)
	switch {
	case cost >= 10:
		return errorStyle.Render(costStr)
	case cost >= 5:
		return warnStyle.Render(costStr)
	default:
		return lipgloss.NewStyle().Foreground(colorGreen).Render(costStr)
	}
}
