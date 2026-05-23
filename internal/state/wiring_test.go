package state

import (
	"path/filepath"
	"testing"
)

// TestProject_EveryDeclaredEventTypeIsWired enforces that every EventType
// constant has an explicit case in (*SQLiteStore).Project — either a real
// projection, a deliberate no-op, or an explicit enumeration. The default
// branch is a soft fallback that silently drops events; without this guard,
// adding a new EventType without wiring it would project no row.
//
// This codifies the "Wiring tests guard event-sourcing default cases"
// learning from SHARED_LEARNINGS.md.
func TestProject_EveryDeclaredEventTypeIsWired(t *testing.T) {
	// The list mirrors the const blocks in events.go. If a new EventType is
	// added there, add it here AND to the Project switch.
	declared := []EventType{
		EventReqSubmitted, EventReqAnalyzed, EventReqPlanned, EventReqPaused,
		EventReqResumed, EventReqCompleted,
		EventStoryCreated, EventStoryEstimated, EventStoryAssigned,
		EventStoryStarted, EventStoryProgress, EventStoryCompleted,
		EventStoryReviewRequested, EventStoryReviewPassed, EventStoryReviewFailed,
		EventStoryQAStarted, EventStoryQAPassed, EventStoryQAFailed,
		EventStoryPRCreated, EventStoryMerged,
		EventAgentSpawned, EventAgentStuck, EventAgentDied, EventAgentStale,
		EventAgentLost,
		EventBudgetWarning, EventBudgetExhausted,
		EventEscalationCreated,
	}

	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "px.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	for _, et := range declared {
		t.Run(string(et), func(t *testing.T) {
			evt := NewEvent(et, "a", "s", map[string]any{
				"id":          "id",
				"req_id":      "r",
				"agent_id":    "a",
				"title":       "t",
				"description": "d",
				"repo_path":   ".",
				// Minimal-but-valid payload for the strict projections.
				"complexity":          1,
				"acceptance_criteria": "a",
				"owned_files":         []string{},
				"wave_hint":           "parallel",
				"depends_on":          []string{},
				"type":                "junior",
				"model":               "m",
				"runtime":             "claude-code",
				"session_name":        "s",
				"story_id":            "s",
				"reason":              "r",
				"from_agent":          "a",
				"status":              "pending",
				"pr_url":              "http://example",
			})
			if err := store.Project(evt); err != nil {
				t.Errorf("Project(%s) returned error: %v", et, err)
			}
		})
	}
}

// TestProject_UnknownEventTypeIsAcceptedQuietly documents that the default
// case is a deliberate fallthrough (drop & log nothing). If we ever decide
// unknown events should error, flip this assertion.
func TestProject_UnknownEventTypeIsAcceptedQuietly(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "px.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	evt := NewEvent(EventType("totally.made.up"), "a", "s", map[string]any{"id": "x"})
	if err := store.Project(evt); err != nil {
		t.Errorf("unknown event should not error (default branch), got %v", err)
	}
}
