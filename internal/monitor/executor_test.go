package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/agent"
	"github.com/tzone85/project-x/internal/config"
	"github.com/tzone85/project-x/internal/git"
	"github.com/tzone85/project-x/internal/planner"
	"github.com/tzone85/project-x/internal/runtime"
	"github.com/tzone85/project-x/internal/state"
)

// mockEventStore is a minimal EventStore for testing.
type mockEventStore struct {
	events []state.Event
}

func (m *mockEventStore) Append(evt state.Event) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *mockEventStore) List(_ state.EventFilter) ([]state.Event, error) {
	return m.events, nil
}

func (m *mockEventStore) Count(_ state.EventFilter) (int, error) {
	return len(m.events), nil
}

func (m *mockEventStore) All() ([]state.Event, error) {
	return m.events, nil
}

// mockProjector is a minimal projector for testing.
type mockProjector struct {
	events []state.Event
}

func (m *mockProjector) Send(evt state.Event) {
	m.events = append(m.events, evt)
}

// errNoSession simulates tmux has-session returning "no session" error.
var errNoSession = fmt.Errorf("no session: px-s-1")

// addSpawnResponses queues the mock responses for a single successful spawn.
// Pattern: worktree prune, worktree remove (cleanup), worktree add -b,
//
//	tmux has-session (not found), tmux new-session.
func addSpawnResponses(mr *git.MockRunner) {
	mr.AddResponse("", nil)          // git worktree prune
	mr.AddResponse("", nil)          // git worktree remove (best-effort cleanup)
	mr.AddResponse("", nil)          // git worktree add -b
	mr.AddResponse("", errNoSession) // tmux has-session -> not found
	mr.AddResponse("", nil)          // tmux new-session
}

func TestExecutor_SpawnAll(t *testing.T) {
	mockRunner := git.NewMockRunner()
	addSpawnResponses(mockRunner)

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	stateDir := t.TempDir()

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, es, ps)

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Setup DB", Description: "Create tables", AcceptanceCriteria: "Tables exist"},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("spawn error: %v", results[0].Error)
	}
	if results[0].Assignment.StoryID != "s-1" {
		t.Errorf("expected story s-1 in result, got %s", results[0].Assignment.StoryID)
	}
	if results[0].RuntimeName != "claude-code" {
		t.Errorf("expected runtime claude-code, got %s", results[0].RuntimeName)
	}
}

func TestExecutor_SpawnAll_WritesClaudeMD(t *testing.T) {
	mockRunner := git.NewMockRunner()
	addSpawnResponses(mockRunner)

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	stateDir := t.TempDir()

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, es, ps)

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Setup DB", Description: "Create tables", AcceptanceCriteria: "Tables exist"},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if results[0].Error != nil {
		t.Fatalf("spawn error: %v", results[0].Error)
	}

	// Verify CLAUDE.md was written in the worktree path
	claudeMDPath := filepath.Join(results[0].WorktreePath, "CLAUDE.md")
	data, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("CLAUDE.md should not be empty")
	}
}

func TestExecutor_SpawnAll_EmitsEvents(t *testing.T) {
	mockRunner := git.NewMockRunner()
	addSpawnResponses(mockRunner)

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	stateDir := t.TempDir()

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, es, ps)

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Setup DB", Description: "Create tables", AcceptanceCriteria: "Tables exist"},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if results[0].Error != nil {
		t.Fatalf("spawn error: %v", results[0].Error)
	}

	// Should have emitted EventStoryStarted to the event store
	if len(es.events) != 1 {
		t.Fatalf("expected 1 event in store, got %d", len(es.events))
	}
	if es.events[0].Type != state.EventStoryStarted {
		t.Errorf("expected event type %s, got %s", state.EventStoryStarted, es.events[0].Type)
	}
	if es.events[0].StoryID != "s-1" {
		t.Errorf("expected story ID s-1, got %s", es.events[0].StoryID)
	}

	// Should have sent event to projector
	if len(ps.events) != 1 {
		t.Fatalf("expected 1 event in projector, got %d", len(ps.events))
	}
}

func TestExecutor_SpawnAll_ConfiguresTranscriptLogForClaude(t *testing.T) {
	mockRunner := git.NewMockRunner()
	addSpawnResponses(mockRunner)

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	stateDir := t.TempDir()
	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, &mockEventStore{}, &mockProjector{})

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Setup DB", Description: "Create tables", AcceptanceCriteria: "Tables exist"},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if results[0].Error != nil {
		t.Fatalf("spawn error: %v", results[0].Error)
	}
	if results[0].TranscriptPath == "" {
		t.Fatal("expected transcript path for claude runtime")
	}

	newCmd := mockRunner.Commands[len(mockRunner.Commands)-1]
	lastArg := newCmd.Args[len(newCmd.Args)-1]
	if strings.Contains(lastArg, "--output-file") {
		t.Fatalf("claude CLI has no --output-file flag; expected tee redirect, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "tee ") {
		t.Fatalf("expected tee redirect for transcript capture, got %q", lastArg)
	}
	if !strings.Contains(lastArg, transcriptFileName) {
		t.Fatalf("expected claude command to point at transcript file, got %q", lastArg)
	}
}

func TestExecutor_SpawnAll_WorktreeError(t *testing.T) {
	mockRunner := git.NewMockRunner()
	mockRunner.AddResponse("", nil)                           // prune
	mockRunner.AddResponse("", nil)                           // remove (cleanup)
	mockRunner.AddResponse("", fmt.Errorf("fatal git error")) // first add fails
	mockRunner.AddResponse("", nil)                           // branch -D
	mockRunner.AddResponse("", fmt.Errorf("fatal git error")) // second add also fails

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	stateDir := t.TempDir()

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, es, ps)

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Setup DB", Complexity: 2},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error from worktree creation failure")
	}

	// No events should be emitted on failure
	if len(es.events) != 0 {
		t.Errorf("expected 0 events on failure, got %d", len(es.events))
	}
}

func TestExecutor_SpawnAll_MultipleAssignments(t *testing.T) {
	mockRunner := git.NewMockRunner()
	// Two assignments: each needs worktree + tmux has-session (not found) + tmux new-session
	addSpawnResponses(mockRunner)
	addSpawnResponses(mockRunner)

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	stateDir := t.TempDir()

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, es, ps)

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Story 1", Description: "First story", AcceptanceCriteria: "AC1", Complexity: 2},
		"s-2": {ID: "s-2", Title: "Story 2", Description: "Second story", AcceptanceCriteria: "AC2", Complexity: 5},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
		{StoryID: "s-2", AgentID: "a-2", Role: agent.RoleIntermediate, Branch: "px/s-2", SessionName: "px-s-2", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result[%d] error: %v", i, r.Error)
		}
	}

	if len(es.events) != 2 {
		t.Errorf("expected 2 events, got %d", len(es.events))
	}
}

func TestExecutor_SpawnAll_PartialFailure(t *testing.T) {
	mockRunner := git.NewMockRunner()
	// First assignment succeeds
	addSpawnResponses(mockRunner)
	// Second assignment fails at worktree creation
	mockRunner.AddResponse("", nil)                             // prune
	mockRunner.AddResponse("", nil)                             // remove (cleanup)
	mockRunner.AddResponse("", fmt.Errorf("worktree conflict")) // first add fails
	mockRunner.AddResponse("", nil)                             // branch -D
	mockRunner.AddResponse("", fmt.Errorf("worktree conflict")) // second add also fails

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	stateDir := t.TempDir()

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: stateDir},
	}, es, ps)

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Story 1", Description: "First", AcceptanceCriteria: "AC1"},
		"s-2": {ID: "s-2", Title: "Story 2", Description: "Second", AcceptanceCriteria: "AC2"},
	}
	assignments := []Assignment{
		{StoryID: "s-1", AgentID: "a-1", Role: agent.RoleJunior, Branch: "px/s-1", SessionName: "px-s-1", Wave: 1},
		{StoryID: "s-2", AgentID: "a-2", Role: agent.RoleJunior, Branch: "px/s-2", SessionName: "px-s-2", Wave: 1},
	}

	results := executor.SpawnAll(".", assignments, stories)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First should succeed
	if results[0].Error != nil {
		t.Errorf("first spawn should succeed, got: %v", results[0].Error)
	}
	// Second should fail
	if results[1].Error == nil {
		t.Error("second spawn should fail due to worktree conflict")
	}

	// Only one event emitted (for the successful spawn)
	if len(es.events) != 1 {
		t.Errorf("expected 1 event (only successful spawn), got %d", len(es.events))
	}
}

func TestExecutor_SpawnAll_Empty(t *testing.T) {
	mockRunner := git.NewMockRunner()

	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(false))
	router := runtime.NewRouter(reg, config.Config{})

	es := &mockEventStore{}
	ps := &mockProjector{}

	executor := NewExecutor(mockRunner, router, config.Config{
		Workspace: config.WorkspaceConfig{StateDir: t.TempDir()},
	}, es, ps)

	results := executor.SpawnAll(".", nil, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty assignments, got %d", len(results))
	}
}
