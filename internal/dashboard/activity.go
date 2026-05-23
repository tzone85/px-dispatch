package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tzone85/px-dispatch/internal/state"
)

const activityEventLimit = 50

// renderActivity produces a chronological event log with newest events first.
func renderActivity(events []state.Event) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(fmt.Sprintf("Recent Events (%d)", len(events))))
	b.WriteString("\n\n")

	if len(events) == 0 {
		b.WriteString(dimStyle.Render("  No events recorded"))
		b.WriteString("\n")
		return b.String()
	}

	// Show events in reverse chronological order (newest first).
	// The events slice from the store is oldest-first, so iterate backwards.
	for i := len(events) - 1; i >= 0; i-- {
		b.WriteString(renderEventLine(events[i]))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderEventLine formats a single event as a log line.
// Format: [HH:MM:SS] EVENT_TYPE agent=X story=Y
func renderEventLine(evt state.Event) string {
	ts := formatEventTime(evt.Timestamp)

	parts := []string{
		dimStyle.Render("[" + ts + "]"),
		eventTypeStyled(string(evt.Type)),
	}

	if evt.AgentID != "" {
		parts = append(parts, dimStyle.Render("agent="+truncateID(evt.AgentID)))
	}
	if evt.StoryID != "" {
		parts = append(parts, dimStyle.Render("story="+truncateID(evt.StoryID)))
	}

	return "  " + strings.Join(parts, " ")
}

// formatEventTime extracts HH:MM:SS from an RFC3339 timestamp.
func formatEventTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		// Fall back to showing the raw timestamp if parsing fails.
		if len(timestamp) >= 8 {
			return timestamp[:8]
		}
		return timestamp
	}
	return t.Format("15:04:05")
}

// eventTypeStyled applies color based on event type category.
func eventTypeStyled(eventType string) string {
	var color lipgloss.Color

	switch {
	case strings.HasPrefix(eventType, "req."):
		color = colorCyan
	case strings.HasPrefix(eventType, "story."):
		color = colorBlue
	case strings.HasPrefix(eventType, "agent."):
		color = colorMagenta
	case strings.HasPrefix(eventType, "escalation."):
		color = colorRed
	case strings.HasPrefix(eventType, "budget."):
		color = colorOrange
	default:
		color = colorWhite
	}

	return lipgloss.NewStyle().Foreground(color).Render(eventType)
}
