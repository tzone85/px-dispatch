package dashboard

import (
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/state"
)

func TestFormatEventTime(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want string
	}{
		{"valid RFC3339", "2026-05-22T14:30:45Z", "14:30:45"},
		{"valid with nanos", "2026-05-22T14:30:45.123456Z", "14:30:45"},
		{"short fallback", "abc", "abc"},
		{"short fallback truncated", "garbage-input-here", "garbage-"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEventTime(tt.ts)
			if got != tt.want {
				t.Errorf("formatEventTime(%q) = %q, want %q", tt.ts, got, tt.want)
			}
		})
	}
}

func TestEventTypeStyled_CategoryPrefixes(t *testing.T) {
	for _, prefix := range []string{"req.", "story.", "agent.", "escalation.", "budget.", "weirdo."} {
		got := eventTypeStyled(prefix + "thing")
		if !strings.Contains(got, prefix+"thing") {
			t.Errorf("expected event type text in output for %q, got %q", prefix, got)
		}
	}
}

func TestRenderActivity_Empty(t *testing.T) {
	out := renderActivity(nil)
	if !strings.Contains(out, "Recent Events (0)") {
		t.Errorf("expected header with 0 count, got %q", out)
	}
	if !strings.Contains(out, "No events recorded") {
		t.Errorf("expected placeholder for empty events, got %q", out)
	}
}

func TestRenderActivity_NewestFirst(t *testing.T) {
	events := []state.Event{
		{ID: "1", Type: state.EventReqSubmitted, Timestamp: "2026-05-22T10:00:00Z", AgentID: "A1"},
		{ID: "2", Type: state.EventStoryCreated, Timestamp: "2026-05-22T11:00:00Z", StoryID: "S1"},
		{ID: "3", Type: state.EventAgentLost, Timestamp: "2026-05-22T12:00:00Z", AgentID: "A2", StoryID: "S2"},
	}
	out := renderActivity(events)
	idx1 := strings.Index(out, "12:00:00")
	idx2 := strings.Index(out, "11:00:00")
	idx3 := strings.Index(out, "10:00:00")
	if !(idx1 < idx2 && idx2 < idx3) {
		t.Errorf("expected newest first; positions: 12=%d 11=%d 10=%d in %q", idx1, idx2, idx3, out)
	}
	if !strings.Contains(out, "Recent Events (3)") {
		t.Errorf("expected count of 3, got %q", out)
	}
}

func TestRenderEventLine_WithAgentAndStory(t *testing.T) {
	evt := state.Event{
		Type:      state.EventStoryCompleted,
		Timestamp: "2026-05-22T09:00:01Z",
		AgentID:   "AGENT123",
		StoryID:   "STORY456",
	}
	line := renderEventLine(evt)
	if !strings.Contains(line, "09:00:01") {
		t.Errorf("missing timestamp in %q", line)
	}
	if !strings.Contains(line, "agent=AGENT123") {
		t.Errorf("missing agent= in %q", line)
	}
	if !strings.Contains(line, "story=STORY456") {
		t.Errorf("missing story= in %q", line)
	}
}

func TestRenderEventLine_WithoutAgentOrStory(t *testing.T) {
	evt := state.Event{
		Type:      state.EventReqSubmitted,
		Timestamp: "2026-05-22T09:00:00Z",
	}
	line := renderEventLine(evt)
	if strings.Contains(line, "agent=") {
		t.Errorf("should omit agent when empty, got %q", line)
	}
	if strings.Contains(line, "story=") {
		t.Errorf("should omit story when empty, got %q", line)
	}
}
