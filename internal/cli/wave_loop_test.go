package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/graph"
	"github.com/tzone85/project-x/internal/monitor"
	"github.com/tzone85/project-x/internal/planner"
	"github.com/tzone85/project-x/internal/state"
)

// --- mocks -----------------------------------------------------------------

type fakeDispatcher struct {
	waves     [][]monitor.Assignment
	errs      []error
	callCount int
}

func (f *fakeDispatcher) DispatchWave(_ *graph.DAG, _ map[string]bool, _ string, _ map[string]planner.PlannedStory, _ int) ([]monitor.Assignment, error) {
	idx := f.callCount
	f.callCount++
	if idx >= len(f.waves) {
		return nil, nil
	}
	var err error
	if idx < len(f.errs) {
		err = f.errs[idx]
	}
	return f.waves[idx], err
}

type fakeExecutor struct {
	rounds [][]monitor.SpawnResult
	calls  int
}

func (f *fakeExecutor) SpawnAll(_ string, _ []monitor.Assignment, _ map[string]planner.PlannedStory) []monitor.SpawnResult {
	idx := f.calls
	f.calls++
	if idx >= len(f.rounds) {
		return nil
	}
	return f.rounds[idx]
}

type fakePoller struct {
	err      error
	cancelAt int
	calls    int
}

func (f *fakePoller) Run(ctx context.Context, _ []monitor.ActiveAgent, _ string) error {
	f.calls++
	if f.cancelAt > 0 && f.calls >= f.cancelAt {
		if cancel, ok := ctx.Value(cancelKey{}).(context.CancelFunc); ok {
			cancel()
		}
	}
	return f.err
}

type cancelKey struct{}

// --- tests ------------------------------------------------------------------

func newWaveDeps(stories []state.Story) waveLoopDeps {
	storyMap := map[string]planner.PlannedStory{}
	for _, s := range stories {
		storyMap[s.ID] = planner.PlannedStory{ID: s.ID, Title: s.Title}
	}
	return waveLoopDeps{
		dag:       graph.NewDAG(),
		storyMap:  storyMap,
		completed: map[string]bool{},
		stories:   stories,
		reqID:     "RQ",
		repoDir:   ".",
	}
}

func TestRunWaveLoop_DispatchError(t *testing.T) {
	setupTestApp(t)

	d := newWaveDeps([]state.Story{{ID: "s-1"}})
	d.dispatcher = &fakeDispatcher{
		waves: [][]monitor.Assignment{nil},
		errs:  []error{errors.New("dispatcher boom")},
	}
	d.executor = &fakeExecutor{}
	d.poller = &fakePoller{}

	err := runWaveLoop(context.Background(), d)
	if err == nil || !strings.Contains(err.Error(), "dispatch wave 1") {
		t.Errorf("expected dispatch error, got %v", err)
	}
}

func TestRunWaveLoop_NoAssignmentsDepsNotMet(t *testing.T) {
	setupTestApp(t)

	d := newWaveDeps([]state.Story{{ID: "s-1"}, {ID: "s-2"}})
	d.dispatcher = &fakeDispatcher{
		waves: [][]monitor.Assignment{nil}, // first dispatch returns 0 assignments
	}
	d.executor = &fakeExecutor{}
	d.poller = &fakePoller{}

	out := captureStdout(t, func() {
		if err := runWaveLoop(context.Background(), d); err != nil {
			t.Fatalf("wave loop: %v", err)
		}
	})
	if !strings.Contains(out, "No stories ready") {
		t.Errorf("expected 'No stories ready' message, got %q", out)
	}
}

func TestRunWaveLoop_AllAgentsFailedToSpawn(t *testing.T) {
	setupTestApp(t)

	d := newWaveDeps([]state.Story{{ID: "s-1"}})
	d.dispatcher = &fakeDispatcher{
		waves: [][]monitor.Assignment{{{StoryID: "s-1", AgentID: "a-1", Branch: "px/s-1", Role: "junior"}}},
	}
	d.executor = &fakeExecutor{
		rounds: [][]monitor.SpawnResult{{{Assignment: monitor.Assignment{StoryID: "s-1"}, Error: errors.New("spawn failed")}}},
	}
	d.poller = &fakePoller{}

	out := captureStdout(t, func() {
		if err := runWaveLoop(context.Background(), d); err != nil {
			t.Fatalf("wave loop: %v", err)
		}
	})
	if !strings.Contains(out, "ERROR spawning s-1") {
		t.Errorf("expected spawn error in output, got %q", out)
	}
	if !strings.Contains(out, "No agents spawned successfully") {
		t.Errorf("expected no-agents banner, got %q", out)
	}
}

func TestRunWaveLoop_PollerError(t *testing.T) {
	setupTestApp(t)

	d := newWaveDeps([]state.Story{{ID: "s-1"}})
	d.dispatcher = &fakeDispatcher{
		waves: [][]monitor.Assignment{{{StoryID: "s-1", AgentID: "a-1", Branch: "px/s-1", Role: "junior"}}},
	}
	d.executor = &fakeExecutor{
		rounds: [][]monitor.SpawnResult{{{
			Assignment:   monitor.Assignment{StoryID: "s-1", AgentID: "a-1"},
			WorktreePath: "/tmp/w", RuntimeName: "claude-code",
		}}},
	}
	d.poller = &fakePoller{err: errors.New("poll boom")}

	err := runWaveLoop(context.Background(), d)
	if err == nil || !strings.Contains(err.Error(), "poll boom") {
		t.Errorf("expected poll error, got %v", err)
	}
}

func TestRunWaveLoop_SuccessfulRun(t *testing.T) {
	setupTestApp(t)

	// Story exists in state DB so the refresh step finds it merged.
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "RQ", "title": "t", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)
	s := state.NewEvent(state.EventStoryCreated, "planner", "s-1", map[string]any{
		"id": "s-1", "req_id": "RQ", "title": "t", "description": "d",
		"acceptance_criteria": "a", "complexity": 1, "owned_files": []string{},
		"wave_hint": "parallel", "depends_on": []string{},
	})
	_ = app.projStore.Project(s)
	m := state.NewEvent(state.EventStoryMerged, "monitor", "s-1", map[string]any{})
	_ = app.projStore.Project(m)

	d := newWaveDeps([]state.Story{{ID: "s-1"}})
	d.dispatcher = &fakeDispatcher{
		waves: [][]monitor.Assignment{
			{{StoryID: "s-1", AgentID: "a-1", Branch: "px/s-1", Role: "junior"}},
			nil, // 2nd dispatch returns 0 → loop ends with allDone=true
		},
	}
	d.executor = &fakeExecutor{
		rounds: [][]monitor.SpawnResult{{{
			Assignment:   monitor.Assignment{StoryID: "s-1", AgentID: "a-1"},
			WorktreePath: "/tmp/w", RuntimeName: "claude-code",
		}}},
	}
	d.poller = &fakePoller{}

	out := captureStdout(t, func() {
		if err := runWaveLoop(context.Background(), d); err != nil {
			t.Fatalf("wave loop: %v", err)
		}
	})
	if !strings.Contains(out, "All 1 stories complete") {
		t.Errorf("expected all-complete summary, got %q", out)
	}
}

func TestRunWaveLoop_ContextCancelled(t *testing.T) {
	setupTestApp(t)

	d := newWaveDeps([]state.Story{{ID: "s-1"}})
	d.dispatcher = &fakeDispatcher{
		waves: [][]monitor.Assignment{
			{{StoryID: "s-1", AgentID: "a-1", Branch: "px/s-1", Role: "junior"}},
		},
	}
	d.executor = &fakeExecutor{
		rounds: [][]monitor.SpawnResult{{{
			Assignment:   monitor.Assignment{StoryID: "s-1", AgentID: "a-1"},
			WorktreePath: "/tmp/w", RuntimeName: "claude-code",
		}}},
	}
	d.poller = &fakePoller{cancelAt: 1}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, cancelKey{}, cancel)
	t.Cleanup(cancel)

	out := captureStdout(t, func() {
		if err := runWaveLoop(ctx, d); err != nil {
			t.Fatalf("wave loop: %v", err)
		}
	})
	if !strings.Contains(out, "Shutdown complete") {
		t.Errorf("expected shutdown message, got %q", out)
	}
}
