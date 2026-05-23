package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tzone85/px-dispatch/internal/state"
)

// Pipeline status columns in display order.
var pipelineStatuses = []string{
	"planned", "assigned", "in_progress", "review", "qa", "merged",
}

// renderPipeline produces a kanban-style view of stories grouped by status.
// Paused requirements are shown as a banner at the top.
func renderPipeline(stories []state.Story, requirements []state.Requirement) string {
	var b strings.Builder

	// Show paused requirements as banners.
	pausedReqs := filterPausedRequirements(requirements)
	if len(pausedReqs) > 0 {
		for _, r := range pausedReqs {
			banner := warnStyle.Render(fmt.Sprintf(
				"PAUSED: %s - %s",
				truncateID(r.ID), r.Title,
			))
			b.WriteString(banner)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Group stories by status.
	grouped := groupStoriesByStatus(stories)

	// Render each status column.
	for _, status := range pipelineStatuses {
		storyList := grouped[status]
		columnHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(statusColor(status)).
			Render(fmt.Sprintf("--- %s (%d) ---", strings.ToUpper(status), len(storyList)))

		b.WriteString(columnHeader)
		b.WriteString("\n")

		if len(storyList) == 0 {
			b.WriteString(dimStyle.Render("  (none)"))
			b.WriteString("\n")
		} else {
			for _, s := range storyList {
				b.WriteString(renderStoryCard(s))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Show stories in other statuses that don't fit the kanban columns.
	otherStories := collectOtherStatuses(stories, grouped)
	if len(otherStories) > 0 {
		otherHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorGray).
			Render(fmt.Sprintf("--- OTHER (%d) ---", len(otherStories)))

		b.WriteString(otherHeader)
		b.WriteString("\n")

		for _, s := range otherStories {
			b.WriteString(renderStoryCard(s))
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderStoryCard formats a single story as a card line.
func renderStoryCard(s state.Story) string {
	id := truncateID(s.ID)
	title := s.Title
	if len(title) > 50 {
		title = title[:47] + "..."
	}

	badge := complexityBadge(s.Complexity)
	agentInfo := ""
	if s.AgentID != "" {
		agentInfo = dimStyle.Render(" agent=" + truncateID(s.AgentID))
	}
	waveInfo := ""
	if s.Wave > 0 {
		waveInfo = dimStyle.Render(fmt.Sprintf(" w%d", s.Wave))
	}

	return fmt.Sprintf("  %s %s %s%s%s",
		dimStyle.Render(id),
		title,
		badge,
		agentInfo,
		waveInfo,
	)
}

// groupStoriesByStatus returns stories keyed by their status.
func groupStoriesByStatus(stories []state.Story) map[string][]state.Story {
	grouped := make(map[string][]state.Story)
	for _, s := range stories {
		grouped[s.Status] = append(grouped[s.Status], s)
	}
	return grouped
}

// filterPausedRequirements returns requirements with "paused" status.
func filterPausedRequirements(reqs []state.Requirement) []state.Requirement {
	var paused []state.Requirement
	for _, r := range reqs {
		if r.Status == "paused" {
			paused = append(paused, r)
		}
	}
	return paused
}

// collectOtherStatuses returns stories whose status is not in the pipeline columns.
func collectOtherStatuses(stories []state.Story, grouped map[string][]state.Story) []state.Story {
	known := make(map[string]bool, len(pipelineStatuses))
	for _, s := range pipelineStatuses {
		known[s] = true
	}

	var others []state.Story
	for _, s := range stories {
		if !known[s.Status] {
			others = append(others, s)
		}
	}
	return others
}

// truncateID returns the first 8 characters of an ID for display.
func truncateID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
