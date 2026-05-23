package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tzone85/px-dispatch/internal/state"
)

// renderAgents produces a table-style view of all agents.
func renderAgents(agents []state.Agent) string {
	var b strings.Builder

	// Summary line.
	summary := agentSummary(agents)
	b.WriteString(headerStyle.Render(summary))
	b.WriteString("\n\n")

	// Table header.
	header := fmt.Sprintf(
		"  %-10s %-14s %-18s %-10s %-10s %s",
		"ID", "ROLE", "MODEL", "STATUS", "STORY", "SESSION",
	)
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorWhite).Render(header))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  " + strings.Repeat("-", 80)))
	b.WriteString("\n")

	if len(agents) == 0 {
		b.WriteString(dimStyle.Render("  No agents registered"))
		b.WriteString("\n")
		return b.String()
	}

	for _, a := range agents {
		b.WriteString(renderAgentRow(a))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderAgentRow formats a single agent as a table row.
func renderAgentRow(a state.Agent) string {
	statusStyled := lipgloss.NewStyle().
		Foreground(statusColor(a.Status)).
		Render(a.Status)

	storyID := a.CurrentStoryID
	if storyID == "" {
		storyID = "-"
	} else {
		storyID = truncateID(storyID)
	}

	session := a.SessionName
	if session == "" {
		session = "-"
	}
	if len(session) > 20 {
		session = session[:17] + "..."
	}

	model := a.Model
	if len(model) > 18 {
		model = model[:15] + "..."
	}

	return fmt.Sprintf(
		"  %-10s %-14s %-18s %-10s %-10s %s",
		truncateID(a.ID),
		a.Type,
		model,
		statusStyled,
		storyID,
		session,
	)
}

// agentSummary returns a one-line summary like "Total: 5 | Active: 3 | Idle: 1 | Stuck: 1".
func agentSummary(agents []state.Agent) string {
	counts := make(map[string]int)
	for _, a := range agents {
		counts[a.Status]++
	}

	parts := []string{
		fmt.Sprintf("Total: %d", len(agents)),
	}

	statuses := []string{"active", "idle", "stuck"}
	for _, s := range statuses {
		if c, ok := counts[s]; ok && c > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", capitalize(s), c))
		}
	}

	// Include any other statuses not in the standard list.
	for status, count := range counts {
		if status != "active" && status != "idle" && status != "stuck" && count > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", capitalize(status), count))
		}
	}

	return strings.Join(parts, " | ")
}

// capitalize returns a string with the first letter upper-cased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
