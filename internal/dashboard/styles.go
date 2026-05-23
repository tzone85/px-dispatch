// Package dashboard provides a scrollable TUI dashboard for observing
// the px-dispatch pipeline, agents, events, escalations, costs, and logs.
package dashboard

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Color palette used throughout the dashboard.
var (
	colorGreen   = lipgloss.Color("#00FF00")
	colorYellow  = lipgloss.Color("#FFFF00")
	colorRed     = lipgloss.Color("#FF0000")
	colorCyan    = lipgloss.Color("#00FFFF")
	colorMagenta = lipgloss.Color("#FF00FF")
	colorGray    = lipgloss.Color("#888888")
	colorWhite   = lipgloss.Color("#FFFFFF")
	colorDimWhite = lipgloss.Color("#AAAAAA")
	colorBlue    = lipgloss.Color("#5588FF")
	colorOrange  = lipgloss.Color("#FF8800")
)

// Tab styles.
var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorCyan)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(colorGray).
				Padding(0, 1)
)

// Panel border style.
var panelBorderStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorCyan).
	Padding(0, 1)

// Status bar at the bottom.
var statusBarStyle = lipgloss.NewStyle().
	Foreground(colorDimWhite).
	Background(lipgloss.Color("#333333"))

// Status colors for agents/stories.
func statusColor(status string) lipgloss.Color {
	switch status {
	case "active", "in_progress", "merged":
		return colorGreen
	case "idle", "planned", "draft":
		return colorGray
	case "stuck", "qa_failed", "pending":
		return colorRed
	case "assigned":
		return colorBlue
	case "review", "pr_submitted":
		return colorYellow
	case "qa":
		return colorMagenta
	case "paused":
		return colorOrange
	default:
		return colorWhite
	}
}

// storyCardStyle returns a styled status badge.
func statusBadge(status string) string {
	return lipgloss.NewStyle().
		Foreground(statusColor(status)).
		Render("[" + status + "]")
}

// complexityBadge returns a styled complexity indicator.
func complexityBadge(complexity int) string {
	color := colorGreen
	if complexity >= 5 {
		color = colorYellow
	}
	if complexity >= 8 {
		color = colorRed
	}
	return lipgloss.NewStyle().
		Foreground(color).
		Render(formatComplexity(complexity))
}

func formatComplexity(c int) string {
	return fmt.Sprintf("[C%d]", c)
}

// headerStyle renders a section header.
var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(colorCyan)

// dimStyle renders dim/secondary text.
var dimStyle = lipgloss.NewStyle().
	Foreground(colorGray)

// warnStyle renders warning text.
var warnStyle = lipgloss.NewStyle().
	Foreground(colorOrange).
	Bold(true)

// errorStyle renders error text.
var errorStyle = lipgloss.NewStyle().
	Foreground(colorRed).
	Bold(true)
