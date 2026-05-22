package dashboard

import (
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/state"
)

func TestCapitalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"a", "A"},
		{"hello", "Hello"},
		{"ALREADY", "ALREADY"},
	}
	for _, tt := range tests {
		if got := capitalize(tt.in); got != tt.want {
			t.Errorf("capitalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAgentSummary_Empty(t *testing.T) {
	got := agentSummary(nil)
	if got != "Total: 0" {
		t.Errorf("empty summary = %q, want 'Total: 0'", got)
	}
}

func TestAgentSummary_Mixed(t *testing.T) {
	agents := []state.Agent{
		{Status: "active"},
		{Status: "active"},
		{Status: "idle"},
		{Status: "stuck"},
		{Status: "weird"},
	}
	got := agentSummary(agents)
	if !strings.Contains(got, "Total: 5") {
		t.Errorf("expected Total: 5, got %q", got)
	}
	if !strings.Contains(got, "Active: 2") {
		t.Errorf("expected Active: 2, got %q", got)
	}
	if !strings.Contains(got, "Idle: 1") {
		t.Errorf("expected Idle: 1, got %q", got)
	}
	if !strings.Contains(got, "Stuck: 1") {
		t.Errorf("expected Stuck: 1, got %q", got)
	}
	if !strings.Contains(got, "Weird: 1") {
		t.Errorf("expected Weird: 1, got %q", got)
	}
}

func TestRenderAgents_Empty(t *testing.T) {
	out := renderAgents(nil)
	if !strings.Contains(out, "Total: 0") {
		t.Errorf("expected total header, got %q", out)
	}
	if !strings.Contains(out, "No agents registered") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestRenderAgents_WithRows(t *testing.T) {
	agents := []state.Agent{
		{
			ID:             "AGENT12345678EXTRA",
			Type:           "claude",
			Model:          "claude-sonnet-4-20250514-with-suffix",
			Status:         "active",
			CurrentStoryID: "STORY9876EXTRA",
			SessionName:    "px-story-name-extra-long",
		},
	}
	out := renderAgents(agents)
	if !strings.Contains(out, "AGENT123") {
		t.Errorf("expected truncated agent id, got %q", out)
	}
	if !strings.Contains(out, "claude-sonnet-4") {
		t.Errorf("expected truncated model, got %q", out)
	}
	if !strings.Contains(out, "STORY987") {
		t.Errorf("expected truncated story id, got %q", out)
	}
}

func TestRenderAgentRow_EmptyStoryAndSession(t *testing.T) {
	row := renderAgentRow(state.Agent{ID: "ID-1", Type: "claude", Status: "idle"})
	if !strings.Contains(row, " - ") {
		t.Errorf("expected dash placeholder for empty story/session, got %q", row)
	}
}
