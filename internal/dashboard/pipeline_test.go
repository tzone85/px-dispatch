package dashboard

import (
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/state"
)

func TestTruncateID(t *testing.T) {
	if got := truncateID("01ABC"); got != "01ABC" {
		t.Errorf("short id should pass through, got %q", got)
	}
	if got := truncateID("0123456789ABCDEF"); got != "01234567" {
		t.Errorf("truncate to 8 chars, got %q", got)
	}
	if got := truncateID(""); got != "" {
		t.Errorf("empty id should be empty, got %q", got)
	}
	if got := truncateID("12345678"); got != "12345678" {
		t.Errorf("exactly 8 chars should pass through, got %q", got)
	}
}

func TestGroupStoriesByStatus(t *testing.T) {
	stories := []state.Story{
		{ID: "s-1", Status: "planned"},
		{ID: "s-2", Status: "planned"},
		{ID: "s-3", Status: "merged"},
	}
	grouped := groupStoriesByStatus(stories)
	if len(grouped["planned"]) != 2 {
		t.Errorf("planned group size = %d, want 2", len(grouped["planned"]))
	}
	if len(grouped["merged"]) != 1 {
		t.Errorf("merged group size = %d, want 1", len(grouped["merged"]))
	}
	if _, ok := grouped["missing"]; ok {
		t.Error("unexpected key 'missing'")
	}
}

func TestFilterPausedRequirements(t *testing.T) {
	reqs := []state.Requirement{
		{ID: "r-1", Status: "active"},
		{ID: "r-2", Status: "paused"},
		{ID: "r-3", Status: "paused"},
		{ID: "r-4", Status: "archived"},
	}
	paused := filterPausedRequirements(reqs)
	if len(paused) != 2 {
		t.Errorf("paused count = %d, want 2", len(paused))
	}
	if paused[0].ID != "r-2" || paused[1].ID != "r-3" {
		t.Errorf("wrong paused reqs: %v", paused)
	}
}

func TestFilterPausedRequirements_None(t *testing.T) {
	if got := filterPausedRequirements(nil); got != nil {
		t.Errorf("nil input should yield nil, got %v", got)
	}
}

func TestCollectOtherStatuses(t *testing.T) {
	stories := []state.Story{
		{ID: "s-1", Status: "planned"},
		{ID: "s-2", Status: "weird"},
		{ID: "s-3", Status: "merged"},
		{ID: "s-4", Status: "blocked"},
	}
	grouped := groupStoriesByStatus(stories)
	others := collectOtherStatuses(stories, grouped)
	if len(others) != 2 {
		t.Fatalf("others count = %d, want 2", len(others))
	}
	got := map[string]bool{}
	for _, s := range others {
		got[s.Status] = true
	}
	if !got["weird"] || !got["blocked"] {
		t.Errorf("expected 'weird' and 'blocked' in others, got %v", got)
	}
}

func TestRenderStoryCard_TruncatesLongTitle(t *testing.T) {
	s := state.Story{
		ID:         "0123456789ABCDEF",
		Title:      strings.Repeat("X", 100),
		Complexity: 4,
		AgentID:    "AGENT-FULL-ID",
		Wave:       3,
	}
	out := renderStoryCard(s)
	if !strings.Contains(out, "01234567") {
		t.Errorf("expected truncated id, got %q", out)
	}
	if strings.Count(out, "X") > 50 {
		t.Errorf("title should be truncated to 50 chars, got %d Xs", strings.Count(out, "X"))
	}
	if !strings.Contains(out, "...") {
		t.Errorf("expected ellipsis in long title, got %q", out)
	}
	if !strings.Contains(out, "agent=AGENT-FU") {
		t.Errorf("expected agent info, got %q", out)
	}
	if !strings.Contains(out, "w3") {
		t.Errorf("expected wave info, got %q", out)
	}
}

func TestRenderStoryCard_NoAgentNoWave(t *testing.T) {
	s := state.Story{ID: "short", Title: "ok", Complexity: 2}
	out := renderStoryCard(s)
	if strings.Contains(out, "agent=") {
		t.Errorf("should omit agent= when AgentID empty, got %q", out)
	}
	if strings.Contains(out, " w") {
		t.Errorf("should omit wave when Wave=0, got %q", out)
	}
}

func TestRenderPipeline_EmptyButWithPaused(t *testing.T) {
	reqs := []state.Requirement{
		{ID: "01PAUSED1234", Title: "blocked work", Status: "paused"},
	}
	out := renderPipeline(nil, reqs)
	if !strings.Contains(out, "PAUSED") {
		t.Errorf("expected PAUSED banner, got %q", out)
	}
	if !strings.Contains(out, "blocked work") {
		t.Errorf("expected paused title, got %q", out)
	}
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' placeholder for empty columns, got %q", out)
	}
}

func TestRenderPipeline_WithStoriesAndOther(t *testing.T) {
	stories := []state.Story{
		{ID: "s-1", Title: "t1", Status: "planned", Complexity: 1},
		{ID: "s-2", Title: "t2", Status: "merged", Complexity: 2},
		{ID: "s-3", Title: "t3", Status: "weird-state", Complexity: 3},
	}
	out := renderPipeline(stories, nil)
	if !strings.Contains(out, "PLANNED (1)") {
		t.Errorf("expected PLANNED (1) header, got %q", out)
	}
	if !strings.Contains(out, "MERGED (1)") {
		t.Errorf("expected MERGED (1) header, got %q", out)
	}
	if !strings.Contains(out, "OTHER (1)") {
		t.Errorf("expected OTHER section, got %q", out)
	}
	if !strings.Contains(out, "t3") {
		t.Errorf("expected story title in other section, got %q", out)
	}
}
