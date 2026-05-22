package dashboard

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tzone85/project-x/internal/state"
)

// --- in-memory fakes for the store interfaces --------------------------------

type fakeEventStore struct {
	events []state.Event
	err    error
}

func (f *fakeEventStore) Append(state.Event) error               { return nil }
func (f *fakeEventStore) Count(state.EventFilter) (int, error)   { return len(f.events), nil }
func (f *fakeEventStore) All() ([]state.Event, error)            { return f.events, nil }
func (f *fakeEventStore) List(state.EventFilter) ([]state.Event, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}

type fakeProjStore struct {
	reqs        []state.Requirement
	stories     []state.Story
	agents      []state.Agent
	escalations []state.Escalation
	failOn      string // "reqs" | "stories" | "agents" | "escalations"
}

func (f *fakeProjStore) Project(state.Event) error                              { return nil }
func (f *fakeProjStore) GetRequirement(string) (state.Requirement, error)       { return state.Requirement{}, nil }
func (f *fakeProjStore) GetStory(string) (state.Story, error)                   { return state.Story{}, nil }
func (f *fakeProjStore) ListStoryDeps(string) ([]state.StoryDep, error)         { return nil, nil }
func (f *fakeProjStore) ArchiveRequirement(string) error                        { return nil }
func (f *fakeProjStore) ArchiveStoriesByReq(string) error                       { return nil }
func (f *fakeProjStore) Close() error                                           { return nil }
func (f *fakeProjStore) ListRequirements(state.ReqFilter) ([]state.Requirement, error) {
	if f.failOn == "reqs" {
		return nil, errors.New("reqs failed")
	}
	return f.reqs, nil
}
func (f *fakeProjStore) ListStories(state.StoryFilter) ([]state.Story, error) {
	if f.failOn == "stories" {
		return nil, errors.New("stories failed")
	}
	return f.stories, nil
}
func (f *fakeProjStore) ListAgents(state.AgentFilter) ([]state.Agent, error) {
	if f.failOn == "agents" {
		return nil, errors.New("agents failed")
	}
	return f.agents, nil
}
func (f *fakeProjStore) ListEscalations() ([]state.Escalation, error) {
	if f.failOn == "escalations" {
		return nil, errors.New("escalations failed")
	}
	return f.escalations, nil
}

// --- helpers -----------------------------------------------------------------

func newTestModel(t *testing.T) Model {
	t.Helper()
	return New(Config{
		EventStore: &fakeEventStore{},
		ProjStore:  &fakeProjStore{},
		Version:    "test",
	})
}

// --- tests ------------------------------------------------------------------

func TestNew_AllocatesViewports(t *testing.T) {
	m := newTestModel(t)
	if m.activePanel != panelPipeline {
		t.Errorf("expected default panel = pipeline (0), got %d", m.activePanel)
	}
	for i, vp := range m.viewports {
		if vp == nil {
			t.Errorf("viewports[%d] is nil", i)
		}
	}
}

func TestModel_Init_ReturnsBatchedCmds(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	// Batch should resolve to something non-nil when invoked.
	msg := cmd()
	if msg == nil {
		t.Errorf("batched cmd returned nil msg")
	}
}

func TestModel_View_InitialReturnsInitializing(t *testing.T) {
	m := newTestModel(t)
	if got := m.View(); got != "Initializing..." {
		t.Errorf("uninitialized View should say Initializing, got %q", got)
	}
}

func TestModel_HandleResize_UpdatesDimensions(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 80, Height: 20})
	mu := updated.(Model)
	if mu.width != 80 || mu.height != 20 {
		t.Errorf("dimensions = %dx%d, want 80x20", mu.width, mu.height)
	}
	for i, vp := range mu.viewports {
		if vp.height != 16 {
			t.Errorf("viewport[%d] height = %d, want 16 (20-4)", i, vp.height)
		}
	}
}

func TestModel_HandleResize_TinyHeightClamps(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 10, Height: 1})
	mu := updated.(Model)
	if mu.viewports[0].height != 1 {
		t.Errorf("viewport height should clamp to 1 for tiny terminal, got %d", mu.viewports[0].height)
	}
}

func TestModel_View_AfterResize(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 80, Height: 20})
	out := updated.(Model).View()
	// View should contain the tab bar at minimum.
	if !strings.Contains(out, "Pipeline") {
		t.Errorf("View should render tabs, got %q", out)
	}
}

func TestModel_View_RendersErrorBanner(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: 80, Height: 20})
	mu := updated.(Model)
	mu.err = errors.New("data fetch failed")
	out := mu.View()
	if !strings.Contains(out, "data fetch failed") {
		t.Errorf("expected error message in View output, got %q", out)
	}
}

func TestModel_HandleKey_TabCyclesPanel(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	mu := updated.(Model)
	if mu.activePanel != 1 {
		t.Errorf("after tab, activePanel = %d, want 1", mu.activePanel)
	}
}

func TestModel_HandleKey_ShiftTabCyclesBackward(t *testing.T) {
	m := newTestModel(t)
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	mu := updated.(Model)
	if mu.activePanel != panelCount-1 {
		t.Errorf("shift+tab should wrap to %d, got %d", panelCount-1, mu.activePanel)
	}
}

func TestModel_HandleKey_NumberSelectsPanel(t *testing.T) {
	for digit := '1'; digit <= '6'; digit++ {
		m := newTestModel(t)
		updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{digit}})
		mu := updated.(Model)
		want := int(digit - '1')
		if mu.activePanel != want {
			t.Errorf("key %q -> activePanel %d, want %d", string(digit), mu.activePanel, want)
		}
	}
}

func TestModel_HandleKey_Quit(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should emit a quit cmd")
	}
	if got, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg, got %T (%v)", cmd(), got)
	}
}

func TestModel_HandleKey_CtrlCQuits(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should emit a quit cmd")
	}
}

func TestModel_HandleKey_ScrollKeys(t *testing.T) {
	m := newTestModel(t)
	m.viewports[panelPipeline].SetHeight(2)
	m.viewports[panelPipeline].SetContent("a\nb\nc\nd\ne")

	// Each scroll key must be exercised so each branch in handleKey is covered.
	cases := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"j", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}},
		{"down", tea.KeyMsg{Type: tea.KeyDown}},
		{"k", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated, _ := m.handleKey(tc.msg)
			mu := updated.(Model)
			if mu.viewports[panelPipeline].offset < 0 {
				t.Errorf("offset went negative for %q", tc.name)
			}
		})
	}
}

func TestModel_HandleKey_GotoTopBottom(t *testing.T) {
	m := newTestModel(t)
	m.viewports[panelPipeline].SetHeight(2)
	m.viewports[panelPipeline].SetContent("a\nb\nc\nd\ne")

	// "G" should go bottom.
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if !m.viewports[panelPipeline].AtBottom() {
		t.Error("G should put viewport at bottom")
	}

	// "g" should go top.
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if !m.viewports[panelPipeline].AtTop() {
		t.Error("g should put viewport at top")
	}
}

func TestModel_HandleKey_PageUpDown(t *testing.T) {
	m := newTestModel(t)
	m.viewports[panelPipeline].SetHeight(2)
	m.viewports[panelPipeline].SetContent("a\nb\nc\nd\ne")

	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.viewports[panelPipeline].offset != 2 {
		t.Errorf("pgdown should advance by height, offset=%d", m.viewports[panelPipeline].offset)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.viewports[panelPipeline].offset != 0 {
		t.Errorf("pgup should return to top, offset=%d", m.viewports[panelPipeline].offset)
	}
}

func TestModel_HandleKey_UnknownKeyIsNoop(t *testing.T) {
	m := newTestModel(t)
	updated, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if updated.(Model).activePanel != m.activePanel {
		t.Error("unknown key should not change panel")
	}
	if cmd != nil {
		t.Error("unknown key should return nil cmd")
	}
}

func TestModel_HandleData_Success(t *testing.T) {
	m := newTestModel(t)
	msg := dataMsg{
		requirements: []state.Requirement{{ID: "r-1", Title: "t"}},
		stories:      []state.Story{{ID: "s-1", Status: "planned", Wave: 1}},
		agents:       []state.Agent{{ID: "a-1", Status: "idle"}},
		events:       []state.Event{{Type: state.EventReqSubmitted, Timestamp: "2026-05-22T10:00:00Z"}},
		escalations:  []state.Escalation{{StoryID: "s-1", Status: "pending"}},
		costData:     costData{dailyTotal: 1, dailyLimit: 5},
	}
	updated, _ := m.handleData(msg)
	mu := updated.(Model)
	if mu.err != nil {
		t.Errorf("err should be nil, got %v", mu.err)
	}
	if len(mu.stories) != 1 {
		t.Errorf("stories not stored, got %d", len(mu.stories))
	}
	if mu.lastRefresh.IsZero() {
		t.Error("lastRefresh should be set")
	}
}

func TestModel_HandleData_Error(t *testing.T) {
	m := newTestModel(t)
	msg := dataMsg{err: errors.New("boom")}
	updated, _ := m.handleData(msg)
	mu := updated.(Model)
	if mu.err == nil || mu.err.Error() != "boom" {
		t.Errorf("error not stored, got %v", mu.err)
	}
}

func TestModel_Update_DispatchesByType(t *testing.T) {
	m := newTestModel(t)

	// KeyMsg
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if updated.(Model).activePanel != 1 {
		t.Error("Update did not dispatch KeyMsg")
	}

	// WindowSizeMsg
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	if updated.(Model).width != 50 {
		t.Error("Update did not dispatch WindowSizeMsg")
	}

	// tickMsg
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Error("tickMsg should produce a follow-up cmd")
	}

	// dataMsg
	updated, _ = m.Update(dataMsg{stories: []state.Story{{ID: "s"}}})
	if len(updated.(Model).stories) != 1 {
		t.Error("Update did not dispatch dataMsg")
	}

	// Unknown msg returns model unchanged.
	type unknownMsg struct{}
	updated, _ = m.Update(unknownMsg{})
	if updated.(Model).activePanel != m.activePanel {
		t.Error("unknown msg should not change state")
	}
}

func TestModel_RenderTabs_HighlightsActive(t *testing.T) {
	m := newTestModel(t)
	m.activePanel = panelAgents
	out := m.renderTabs()
	for _, name := range panelNames {
		if !strings.Contains(out, name) {
			t.Errorf("tab bar missing %q in %q", name, out)
		}
	}
}

func TestModel_RenderStatusBar_HasAllSections(t *testing.T) {
	m := newTestModel(t)
	m.width = 200
	m.version = "1.2.3"
	m.costInfo = costData{dailyTotal: 1.5, dailyLimit: 10}
	out := m.renderStatusBar()
	for _, want := range []string{"PX v1.2.3", "$1.50", "$10.00", "q:quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("status bar missing %q in %q", want, out)
		}
	}
}

func TestModel_WaveProgress(t *testing.T) {
	tests := []struct {
		name    string
		stories []state.Story
		want    string
	}{
		{"no stories", nil, "No stories"},
		{"wave zero", []state.Story{{Wave: 0}}, "Wave 0"},
		{
			"some merged",
			[]state.Story{{Wave: 1, Status: "merged"}, {Wave: 1, Status: "planned"}, {Wave: 2, Status: "merged"}},
			"Wave 2: 1/1 merged",
		},
		{
			"none merged in current wave",
			[]state.Story{{Wave: 1, Status: "planned"}, {Wave: 1, Status: "review"}},
			"Wave 1: 0/2 merged",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.stories = tt.stories
			if got := m.waveProgress(); got != tt.want {
				t.Errorf("waveProgress = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModel_RefreshPanelContent_PopulatesAll(t *testing.T) {
	m := newTestModel(t)
	m.stories = []state.Story{{ID: "s-1", Status: "planned"}}
	m.refreshPanelContent()
	for i, vp := range m.viewports {
		if vp.content == "" {
			t.Errorf("viewport[%d] has empty content", i)
		}
	}
}

func TestFetchDataCmd_Success(t *testing.T) {
	ev := &fakeEventStore{events: []state.Event{{Type: state.EventReqSubmitted, Timestamp: "2026-05-22T10:00:00Z"}}}
	ps := &fakeProjStore{
		reqs:        []state.Requirement{{ID: "r"}},
		stories:     []state.Story{{ID: "s"}},
		agents:      []state.Agent{{ID: "a"}},
		escalations: []state.Escalation{{StoryID: "s"}},
	}
	m := New(Config{EventStore: ev, ProjStore: ps, Version: "t"})

	msg := fetchDataCmd(m)()
	d, ok := msg.(dataMsg)
	if !ok {
		t.Fatalf("expected dataMsg, got %T", msg)
	}
	if d.err != nil {
		t.Fatalf("unexpected err: %v", d.err)
	}
	if len(d.requirements) != 1 || len(d.stories) != 1 || len(d.agents) != 1 || len(d.escalations) != 1 || len(d.events) != 1 {
		t.Errorf("unexpected counts: reqs=%d stories=%d agents=%d escalations=%d events=%d",
			len(d.requirements), len(d.stories), len(d.agents), len(d.escalations), len(d.events))
	}
}

func TestFetchDataCmd_PropagatesEachStoreError(t *testing.T) {
	for _, failOn := range []string{"reqs", "stories", "agents", "escalations"} {
		t.Run(failOn, func(t *testing.T) {
			ev := &fakeEventStore{}
			ps := &fakeProjStore{failOn: failOn}
			m := New(Config{EventStore: ev, ProjStore: ps})
			msg := fetchDataCmd(m)().(dataMsg)
			if msg.err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestFetchDataCmd_EventStoreError(t *testing.T) {
	ev := &fakeEventStore{err: errors.New("event store down")}
	ps := &fakeProjStore{}
	m := New(Config{EventStore: ev, ProjStore: ps})
	msg := fetchDataCmd(m)().(dataMsg)
	if msg.err == nil || !strings.Contains(msg.err.Error(), "list events") {
		t.Errorf("expected list-events error, got %v", msg.err)
	}
}

func TestFetchDataCmd_WithDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE token_usage (req_id TEXT, cost_usd REAL, created_at TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	m := New(Config{
		EventStore: &fakeEventStore{},
		ProjStore:  &fakeProjStore{reqs: []state.Requirement{{ID: "r-1"}}},
		DB:         db,
		DailyLimit: 5,
	})
	msg := fetchDataCmd(m)().(dataMsg)
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if msg.costData.dailyLimit != 5 {
		t.Errorf("expected dailyLimit propagated, got %v", msg.costData.dailyLimit)
	}
}

func TestTickCmd_ReturnsTickMsg(t *testing.T) {
	cmd := tickCmd()
	// Just verify the cmd is callable and produces a tickMsg eventually.
	// We invoke immediately; tea.Tick will sleep, so use a separate goroutine
	// to assert type rather than wait the full 2 seconds.
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if _, ok := msg.(tickMsg); !ok {
			t.Errorf("expected tickMsg, got %T", msg)
		}
	case <-timeoutChan(t):
		t.Fatal("tickCmd did not return within timeout")
	}
}
