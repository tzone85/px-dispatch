package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tzone85/px-dispatch/internal/state"
)

// renderEscalations produces a list of escalations with status highlighting.
func renderEscalations(escalations []state.Escalation) string {
	var b strings.Builder

	pendingCount := countPending(escalations)
	summaryText := fmt.Sprintf("Escalations: %d total, %d pending", len(escalations), pendingCount)
	b.WriteString(headerStyle.Render(summaryText))
	b.WriteString("\n\n")

	if len(escalations) == 0 {
		b.WriteString(dimStyle.Render("  No escalations"))
		b.WriteString("\n")
		return b.String()
	}

	// Table header.
	header := fmt.Sprintf(
		"  %-10s %-10s %-10s %-10s %s",
		"STORY", "FROM", "STATUS", "TIME", "REASON",
	)
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorWhite).Render(header))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  " + strings.Repeat("-", 70)))
	b.WriteString("\n")

	for _, e := range escalations {
		b.WriteString(renderEscalationRow(e))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderEscalationRow formats a single escalation as a table row.
func renderEscalationRow(e state.Escalation) string {
	storyID := truncateID(e.StoryID)
	fromAgent := truncateID(e.FromAgent)
	ts := formatEventTime(e.CreatedAt)

	reason := e.Reason
	if len(reason) > 40 {
		reason = reason[:37] + "..."
	}

	// Highlight pending escalations.
	statusStyled := lipgloss.NewStyle().
		Foreground(escalationStatusColor(e.Status)).
		Render(e.Status)

	row := fmt.Sprintf(
		"  %-10s %-10s %-10s %-10s %s",
		storyID,
		fromAgent,
		statusStyled,
		dimStyle.Render(ts),
		reason,
	)

	// Make the entire row stand out if pending.
	if e.Status == "pending" {
		return lipgloss.NewStyle().Bold(true).Render(row)
	}
	return row
}

// escalationStatusColor returns a color for the escalation status.
func escalationStatusColor(status string) lipgloss.Color {
	switch status {
	case "pending":
		return colorRed
	case "resolved":
		return colorGreen
	case "dismissed":
		return colorGray
	default:
		return colorYellow
	}
}

// countPending returns the number of escalations with "pending" status.
func countPending(escalations []state.Escalation) int {
	count := 0
	for _, e := range escalations {
		if e.Status == "pending" {
			count++
		}
	}
	return count
}
