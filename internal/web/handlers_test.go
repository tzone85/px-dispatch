package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tzone85/project-x/internal/state"
)

// setupTestHandlers creates in-memory stores and returns a configured Handlers.
func setupTestHandlers(t *testing.T) *Handlers {
	t.Helper()

	projStore, err := state.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { projStore.Close() })

	eventsDir := t.TempDir()
	eventsPath := filepath.Join(eventsDir, "events.jsonl")
	eventStore, err := state.NewFileStore(eventsPath)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	t.Cleanup(func() { eventStore.Close() })

	db := projStore.DB()

	// Create token_usage table for cost queries.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS token_usage (
		id TEXT PRIMARY KEY,
		req_id TEXT NOT NULL,
		story_id TEXT NOT NULL DEFAULT '',
		agent_id TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL,
		input_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cost_usd REAL NOT NULL DEFAULT 0.0,
		stage TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create token_usage: %v", err)
	}

	return &Handlers{
		eventStore: eventStore,
		projStore:  projStore,
		db:         db,
	}
}

// mustMarshalJSON marshals v to json.RawMessage for test event payloads.
func mustMarshalJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

// seedRequirement projects a requirement submission event into the store.
func seedRequirement(t *testing.T, h *Handlers, id, title string) {
	t.Helper()
	evt := state.Event{
		ID:        "evt-req-" + id,
		Type:      state.EventReqSubmitted,
		Timestamp: "2026-01-01T00:00:00Z",
		Payload: mustMarshalJSON(t, state.ReqSubmittedPayload{
			ID:          id,
			Title:       title,
			Description: "Test requirement " + id,
			RepoPath:    "/repo/test",
		}),
	}
	if err := h.projStore.Project(evt); err != nil {
		t.Fatalf("project req: %v", err)
	}
}

// seedStory projects a story creation event into the store.
func seedStory(t *testing.T, h *Handlers, id, reqID, title string) {
	t.Helper()
	evt := state.Event{
		ID:        "evt-story-" + id,
		Type:      state.EventStoryCreated,
		StoryID:   id,
		Timestamp: "2026-01-01T00:01:00Z",
		Payload: mustMarshalJSON(t, state.StoryCreatedPayload{
			ID:          id,
			ReqID:       reqID,
			Title:       title,
			Description: "Test story " + id,
			Complexity:  2,
			OwnedFiles:  []string{"main.go"},
			WaveHint:    "parallel",
		}),
	}
	if err := h.projStore.Project(evt); err != nil {
		t.Fatalf("project story: %v", err)
	}
}

// seedAgent projects an agent spawned event into the store.
func seedAgent(t *testing.T, h *Handlers, id, agentType, storyID string) {
	t.Helper()
	evt := state.Event{
		ID:        "evt-agent-" + id,
		Type:      state.EventAgentSpawned,
		AgentID:   id,
		Timestamp: "2026-01-01T00:02:00Z",
		Payload: mustMarshalJSON(t, state.AgentSpawnedPayload{
			ID:          id,
			Type:        agentType,
			Model:       "claude-sonnet-4-20250514",
			Runtime:     "tmux",
			SessionName: "sess-" + id,
			StoryID:     storyID,
		}),
	}
	if err := h.projStore.Project(evt); err != nil {
		t.Fatalf("project agent: %v", err)
	}
}

// seedEvent appends an event to the event store for testing ListEvents.
func seedEvent(t *testing.T, h *Handlers, id string, evtType state.EventType) {
	t.Helper()
	evt := state.Event{
		ID:        id,
		Type:      evtType,
		Timestamp: "2026-01-01T00:00:00Z",
	}
	if err := h.eventStore.Append(evt); err != nil {
		t.Fatalf("append event: %v", err)
	}
}

// seedCostRecord inserts a token usage record for cost testing.
func seedCostRecord(t *testing.T, h *Handlers, reqID, storyID string, costUSD float64) {
	t.Helper()
	_, err := h.db.Exec(
		`INSERT INTO token_usage (id, req_id, story_id, agent_id, model, input_tokens, output_tokens, cost_usd, stage)
		 VALUES (?, ?, ?, 'a1', 'claude-sonnet-4-20250514', 1000, 500, ?, 'review')`,
		"tu-"+reqID+"-"+storyID, reqID, storyID, costUSD,
	)
	if err != nil {
		t.Fatalf("insert cost: %v", err)
	}
}

// --- Tests ---

func TestListRequirements_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/requirements", nil)
	w := httptest.NewRecorder()

	h.ListRequirements(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListRequirements_WithData(t *testing.T) {
	h := setupTestHandlers(t)
	seedRequirement(t, h, "r1", "First Req")
	seedRequirement(t, h, "r2", "Second Req")

	req := httptest.NewRequest("GET", "/api/requirements", nil)
	w := httptest.NewRecorder()

	h.ListRequirements(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", contentType, "application/json")
	}

	var result []state.Requirement
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 requirements, got %d", len(result))
	}
}

func TestListStories_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/stories", nil)
	w := httptest.NewRecorder()

	h.ListStories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListStories_FilterByReqID(t *testing.T) {
	h := setupTestHandlers(t)
	seedRequirement(t, h, "r1", "Req 1")
	seedRequirement(t, h, "r2", "Req 2")
	seedStory(t, h, "s1", "r1", "Story 1")
	seedStory(t, h, "s2", "r1", "Story 2")
	seedStory(t, h, "s3", "r2", "Story 3")

	req := httptest.NewRequest("GET", "/api/stories?req_id=r1", nil)
	w := httptest.NewRecorder()

	h.ListStories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Story
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 stories for r1, got %d", len(result))
	}
}

func TestListStories_FilterByStatus(t *testing.T) {
	h := setupTestHandlers(t)
	seedRequirement(t, h, "r1", "Req 1")
	seedStory(t, h, "s1", "r1", "Story 1")
	seedStory(t, h, "s2", "r1", "Story 2")

	// Assign s1 to change its status
	h.projStore.Project(state.Event{
		ID: "evt-assign", Type: state.EventStoryAssigned, StoryID: "s1",
		Payload:   mustMarshalJSON(t, state.StoryAssignedPayload{AgentID: "a1", Wave: 1}),
		Timestamp: "2026-01-01T00:03:00Z",
	})

	req := httptest.NewRequest("GET", "/api/stories?status=assigned", nil)
	w := httptest.NewRecorder()

	h.ListStories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Story
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 assigned story, got %d", len(result))
	}
}

func TestListStories_Pagination(t *testing.T) {
	h := setupTestHandlers(t)
	seedRequirement(t, h, "r1", "Req 1")
	seedStory(t, h, "s1", "r1", "Story 1")
	seedStory(t, h, "s2", "r1", "Story 2")
	seedStory(t, h, "s3", "r1", "Story 3")

	req := httptest.NewRequest("GET", "/api/stories?limit=2&offset=1", nil)
	w := httptest.NewRecorder()

	h.ListStories(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Story
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 stories with limit=2 offset=1, got %d", len(result))
	}
}

func TestListAgents_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()

	h.ListAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListAgents_WithData(t *testing.T) {
	h := setupTestHandlers(t)
	seedAgent(t, h, "a1", "coder", "s1")
	seedAgent(t, h, "a2", "reviewer", "s2")

	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()

	h.ListAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Agent
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 agents, got %d", len(result))
	}
}

func TestListAgents_FilterByStatus(t *testing.T) {
	h := setupTestHandlers(t)
	seedAgent(t, h, "a1", "coder", "s1")

	req := httptest.NewRequest("GET", "/api/agents?status=active", nil)
	w := httptest.NewRecorder()

	h.ListAgents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Agent
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 active agent, got %d", len(result))
	}
}

func TestListEvents_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()

	h.ListEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListEvents_WithFilter(t *testing.T) {
	h := setupTestHandlers(t)
	seedEvent(t, h, "e1", state.EventReqSubmitted)
	seedEvent(t, h, "e2", state.EventStoryCreated)
	seedEvent(t, h, "e3", state.EventReqSubmitted)

	req := httptest.NewRequest("GET", "/api/events?type=req.submitted", nil)
	w := httptest.NewRecorder()

	h.ListEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Event
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 req.submitted events, got %d", len(result))
	}
}

func TestListEvents_WithLimit(t *testing.T) {
	h := setupTestHandlers(t)
	seedEvent(t, h, "e1", state.EventReqSubmitted)
	seedEvent(t, h, "e2", state.EventReqSubmitted)
	seedEvent(t, h, "e3", state.EventReqSubmitted)

	req := httptest.NewRequest("GET", "/api/events?limit=2", nil)
	w := httptest.NewRecorder()

	h.ListEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Event
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 events with limit=2, got %d", len(result))
	}
}

func TestGetCost_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/cost", nil)
	w := httptest.NewRecorder()

	h.GetCost(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result costResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.TodayUSD != 0 {
		t.Errorf("expected 0 daily cost, got %f", result.TodayUSD)
	}
}

func TestGetCost_Daily(t *testing.T) {
	h := setupTestHandlers(t)
	seedCostRecord(t, h, "r1", "s1", 0.05)
	seedCostRecord(t, h, "r1", "s2", 0.03)

	req := httptest.NewRequest("GET", "/api/cost", nil)
	w := httptest.NewRecorder()

	h.GetCost(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result costResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.TodayUSD < 0.08 {
		t.Errorf("expected daily cost >= 0.08, got %f", result.TodayUSD)
	}
}

func TestGetCost_ByRequirement(t *testing.T) {
	h := setupTestHandlers(t)
	seedCostRecord(t, h, "r1", "s1", 0.10)

	req := httptest.NewRequest("GET", "/api/cost?req_id=r1", nil)
	w := httptest.NewRecorder()

	h.GetCost(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result costResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ReqUSD < 0.10 {
		t.Errorf("expected req cost >= 0.10, got %f", result.ReqUSD)
	}
}

func TestGetCost_IncludesDailyLimit(t *testing.T) {
	h := setupTestHandlers(t)
	h.dailyLimitUSD = 42.5

	req := httptest.NewRequest("GET", "/api/cost", nil)
	w := httptest.NewRecorder()
	h.GetCost(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result costResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.DailyLimitUSD != 42.5 {
		t.Errorf("DailyLimitUSD: got %v, want 42.5", result.DailyLimitUSD)
	}
}

func TestGetLogs_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	h.logPath = filepath.Join(t.TempDir(), "px.log") // file does not exist

	req := httptest.NewRequest("GET", "/api/logs", nil)
	w := httptest.NewRecorder()
	h.GetLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result logsResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Lines) != 0 {
		t.Errorf("expected empty lines, got %d", len(result.Lines))
	}
}

func TestGetLogs_TailLimit(t *testing.T) {
	h := setupTestHandlers(t)
	dir := t.TempDir()
	h.logPath = filepath.Join(dir, "px.log")

	var content []byte
	for i := 0; i < 250; i++ {
		content = append(content, []byte(fmt.Sprintf("line-%d\n", i))...)
	}
	if err := os.WriteFile(h.logPath, content, 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/logs?limit=50", nil)
	w := httptest.NewRecorder()
	h.GetLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result logsResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Lines) != 50 {
		t.Fatalf("expected 50 lines (tail), got %d", len(result.Lines))
	}
	if result.Lines[0] != "line-200" {
		t.Errorf("first tailed line: got %q, want %q", result.Lines[0], "line-200")
	}
	if result.Lines[49] != "line-249" {
		t.Errorf("last tailed line: got %q, want %q", result.Lines[49], "line-249")
	}
}

func TestGetLogs_RejectsExcessiveLimit(t *testing.T) {
	h := setupTestHandlers(t)
	dir := t.TempDir()
	h.logPath = filepath.Join(dir, "px.log")
	if err := os.WriteFile(h.logPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/logs?limit=999999", nil)
	w := httptest.NewRecorder()
	h.GetLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result logsResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Should be clamped to maxLogLines, not blow up memory.
	if len(result.Lines) > maxLogLines {
		t.Errorf("limit clamping failed: got %d, max %d", len(result.Lines), maxLogLines)
	}
}

func TestGetHealth(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	h.GetHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", contentType, "application/json")
	}

	var result healthResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("status: got %q, want %q", result.Status, "ok")
	}
	if result.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestEnsureSlice_NilBecomesEmptyArray(t *testing.T) {
	w := httptest.NewRecorder()
	// ensureSlice converts nil to empty slice, so JSON encodes as [].
	writeJSON(w, ensureSlice([]string(nil)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body != "[]\n" {
		t.Errorf("expected [], got %q", body)
	}
}

func TestEnsureSlice_NonNilPassesThrough(t *testing.T) {
	input := []string{"a", "b"}
	result := ensureSlice(input)
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
}

func TestGetAbout(t *testing.T) {
	h := setupTestHandlers(t)
	h.version = "v9.9.9"

	req := httptest.NewRequest("GET", "/api/about", nil)
	w := httptest.NewRecorder()

	h.GetAbout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}

	var result aboutResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Name != "Project X" {
		t.Errorf("Name: got %q, want %q", result.Name, "Project X")
	}
	if result.Version != "v9.9.9" {
		t.Errorf("Version: got %q, want %q", result.Version, "v9.9.9")
	}
	if result.Tagline == "" {
		t.Error("Tagline must not be empty")
	}
	if result.Description == "" {
		t.Error("Description must not be empty")
	}
	if len(result.Features) == 0 {
		t.Error("Features must not be empty")
	}
	if len(result.PipelineStages) != 7 {
		t.Errorf("PipelineStages: expected 7, got %d", len(result.PipelineStages))
	}
	if len(result.Runtimes) == 0 {
		t.Error("Runtimes must not be empty")
	}
	if result.Repo == "" {
		t.Error("Repo must not be empty")
	}
}

func TestGetAbout_DefaultVersion(t *testing.T) {
	h := setupTestHandlers(t)
	// Version not set → default "dev"
	req := httptest.NewRequest("GET", "/api/about", nil)
	w := httptest.NewRecorder()

	h.GetAbout(w, req)

	var result aboutResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Version != "dev" {
		t.Errorf("Version: expected default %q, got %q", "dev", result.Version)
	}
}

func TestListEscalations_Empty(t *testing.T) {
	h := setupTestHandlers(t)
	req := httptest.NewRequest("GET", "/api/escalations", nil)
	w := httptest.NewRecorder()

	h.ListEscalations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Escalation
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestListEscalations_WithData(t *testing.T) {
	h := setupTestHandlers(t)
	seedRequirement(t, h, "r1", "Req 1")
	seedStory(t, h, "s1", "r1", "Story 1")

	// Project escalation event
	evt := state.Event{
		ID:        "evt-esc-1",
		Type:      state.EventEscalationCreated,
		StoryID:   "s1",
		Timestamp: "2026-01-01T00:05:00Z",
		Payload: mustMarshalJSON(t, state.EscalationCreatedPayload{
			ID:        "esc-1",
			StoryID:   "s1",
			FromAgent: "junior-a1",
			Reason:    "Out of depth — requires senior",
		}),
	}
	if err := h.projStore.Project(evt); err != nil {
		t.Fatalf("project escalation: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/escalations", nil)
	w := httptest.NewRecorder()

	h.ListEscalations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []state.Escalation
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 escalation, got %d", len(result))
	}
	if result[0].StoryID != "s1" {
		t.Errorf("StoryID: got %q, want %q", result[0].StoryID, "s1")
	}
}

// Ensure the test environment is valid (this prevents accidentally missing the
// sqlite3 driver).
func TestSetupTestHandlers_Smoke(t *testing.T) {
	h := setupTestHandlers(t)
	if h.eventStore == nil {
		t.Fatal("eventStore is nil")
	}
	if h.projStore == nil {
		t.Fatal("projStore is nil")
	}
	if h.db == nil {
		t.Fatal("db is nil")
	}
}
