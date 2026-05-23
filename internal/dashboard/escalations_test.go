package dashboard

import (
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/state"
)

func TestEscalationStatusColor(t *testing.T) {
	tests := []struct {
		status, want string
	}{
		{"pending", "#FF0000"},
		{"resolved", "#00FF00"},
		{"dismissed", "#888888"},
		{"weird", "#FFFF00"},
		{"", "#FFFF00"},
	}
	for _, tt := range tests {
		if got := string(escalationStatusColor(tt.status)); got != tt.want {
			t.Errorf("escalationStatusColor(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestCountPending(t *testing.T) {
	escs := []state.Escalation{
		{Status: "pending"},
		{Status: "pending"},
		{Status: "resolved"},
		{Status: "dismissed"},
	}
	if got := countPending(escs); got != 2 {
		t.Errorf("countPending = %d, want 2", got)
	}
	if got := countPending(nil); got != 0 {
		t.Errorf("nil escalations should be 0, got %d", got)
	}
}

func TestRenderEscalations_Empty(t *testing.T) {
	out := renderEscalations(nil)
	if !strings.Contains(out, "Escalations: 0 total, 0 pending") {
		t.Errorf("expected counts header, got %q", out)
	}
	if !strings.Contains(out, "No escalations") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestRenderEscalations_Rows(t *testing.T) {
	escs := []state.Escalation{
		{StoryID: "STORY1XYZ", FromAgent: "AGENT1XYZ", Status: "pending", CreatedAt: "2026-05-22T10:00:00Z", Reason: "blocked"},
		{StoryID: "STORY2", FromAgent: "AGENT2", Status: "resolved", CreatedAt: "2026-05-22T11:00:00Z", Reason: strings.Repeat("R", 100)},
	}
	out := renderEscalations(escs)
	if !strings.Contains(out, "Escalations: 2 total, 1 pending") {
		t.Errorf("expected 2 total / 1 pending, got %q", out)
	}
	if !strings.Contains(out, "STORY1XY") {
		t.Errorf("expected truncated story id, got %q", out)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("expected long reason to be truncated with ellipsis, got %q", out)
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("expected short reason preserved, got %q", out)
	}
}

func TestRenderEscalationRow_PendingStyled(t *testing.T) {
	e := state.Escalation{
		StoryID:   "S",
		FromAgent: "A",
		Status:    "pending",
		CreatedAt: "2026-05-22T09:00:00Z",
		Reason:    "x",
	}
	row := renderEscalationRow(e)
	if !strings.Contains(row, "pending") {
		t.Errorf("expected status word in row, got %q", row)
	}
}
