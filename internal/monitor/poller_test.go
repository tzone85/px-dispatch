package monitor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/pipeline"
)

// mockPipelineRunner records calls.
type mockPipelineRunner struct {
	mu     sync.Mutex
	runs   []string // story IDs
	result pipeline.StageResult
}

func (m *mockPipelineRunner) Run(_ context.Context, sc pipeline.StoryContext) (pipeline.StageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = append(m.runs, sc.StoryID)
	return m.result, nil
}

// safeRunner wraps git.MockRunner with a mutex for concurrent test access.
type safeRunner struct {
	mu    sync.Mutex
	inner *git.MockRunner
}

func newSafeRunner() *safeRunner {
	return &safeRunner{inner: git.NewMockRunner()}
}

func (s *safeRunner) AddResponse(output string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner.AddResponse(output, err)
}

func (s *safeRunner) Run(dir, name string, args ...string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Run(dir, name, args...)
}

func TestPoller_DetectsCompletedAgent(t *testing.T) {
	runner := newSafeRunner()
	// SessionHealth calls: has-session (exists), list-panes, capture-pane (health)
	runner.AddResponse("", nil)        // tmux has-session -> exists
	runner.AddResponse("12345 0", nil) // tmux list-panes -> alive
	runner.AddResponse("$ ", nil)      // tmux capture-pane (health output hash)
	// pollOnce ReadOutput call for done detection
	runner.AddResponse("$ ", nil) // tmux capture-pane -> idle prompt (done)

	es := &mockEventStore{}
	pr := &mockPipelineRunner{result: pipeline.StagePassed}

	agents := []ActiveAgent{
		{Assignment: Assignment{StoryID: "s-1", SessionName: "px-s-1"}, WorktreePath: "/tmp/s1", RuntimeName: "claude-code"},
	}

	p := NewPoller(PollerConfig{PollIntervalMs: 10}, runner, nil, pr, es, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	p.Run(ctx, agents, ".")

	pr.mu.Lock()
	defer pr.mu.Unlock()
	if len(pr.runs) == 0 {
		t.Error("expected pipeline to run for completed agent")
	}
}

func TestPoller_GracefulShutdown(t *testing.T) {
	es := &mockEventStore{}
	pr := &mockPipelineRunner{result: pipeline.StagePassed}
	runner := newSafeRunner()
	// Keep returning "working" status for repeated polls
	for i := 0; i < 300; i++ {
		runner.AddResponse("", nil)                 // tmux has-session
		runner.AddResponse("12345 0", nil)          // tmux list-panes
		runner.AddResponse("still working...", nil) // capture-pane (health)
		runner.AddResponse("still working...", nil) // capture-pane (done check)
	}

	agents := []ActiveAgent{
		{Assignment: Assignment{StoryID: "s-1", SessionName: "px-s-1"}, RuntimeName: "claude-code"},
	}

	p := NewPoller(PollerConfig{PollIntervalMs: 10}, runner, nil, pr, es, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.Run(ctx, agents, ".")
		close(done)
	}()

	// Cancel quickly
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good - poller exited
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not shut down within timeout")
	}
}

func TestPoller_EmptyAgentList(t *testing.T) {
	es := &mockEventStore{}
	pr := &mockPipelineRunner{result: pipeline.StagePassed}
	runner := newSafeRunner()

	p := NewPoller(PollerConfig{PollIntervalMs: 10}, runner, nil, pr, es, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should exit immediately with no agents
	err := p.Run(ctx, nil, ".")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestPoller_SerializedMerge(t *testing.T) {
	es := &mockEventStore{}
	runner := newSafeRunner()
	// Both agents show as done on first poll: each needs health check + done check
	for i := 0; i < 20; i++ {
		runner.AddResponse("", nil)        // tmux has-session
		runner.AddResponse("12345 0", nil) // tmux list-panes
		runner.AddResponse("$ ", nil)      // capture-pane (health)
		runner.AddResponse("$ ", nil)      // capture-pane (done check)
	}

	pr := &mockPipelineRunner{result: pipeline.StagePassed}

	agents := []ActiveAgent{
		{Assignment: Assignment{StoryID: "s-1", SessionName: "px-s-1"}, RuntimeName: "claude-code"},
		{Assignment: Assignment{StoryID: "s-2", SessionName: "px-s-2"}, RuntimeName: "claude-code"},
	}

	p := NewPoller(PollerConfig{PollIntervalMs: 10}, runner, nil, pr, es, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	p.Run(ctx, agents, ".")

	pr.mu.Lock()
	defer pr.mu.Unlock()
	if len(pr.runs) < 2 {
		t.Errorf("expected both agents processed, got %d", len(pr.runs))
	}
}

func TestPoller_MissingSessionWithSentinelTriggersPipeline(t *testing.T) {
	// When the tmux session has already exited but the runtime wrote the
	// `.px-done` sentinel into the worktree, the poller must treat the
	// agent as finished and run the pipeline rather than emit agent.lost.
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, completionSentinel), nil, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	es := &mockEventStore{}
	runner := newSafeRunner()
	runner.AddResponse("", errNoSession) // tmux has-session -> missing

	pr := &mockPipelineRunner{result: pipeline.StagePassed}

	agents := []ActiveAgent{
		{
			Assignment:   Assignment{StoryID: "s-1", SessionName: "px-s-1"},
			WorktreePath: worktree,
			RuntimeName:  "claude-code",
		},
	}

	p := NewPoller(PollerConfig{PollIntervalMs: 10}, runner, nil, pr, es, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	p.Run(ctx, agents, ".")

	pr.mu.Lock()
	defer pr.mu.Unlock()
	if len(pr.runs) == 0 {
		t.Error("expected pipeline to run when sentinel exists despite missing session")
	}
}

func TestSentinelExists_HandlesEmptyPath(t *testing.T) {
	if sentinelExists("") {
		t.Error("empty worktree path must return false")
	}
}

func TestPoller_DeadSessionSkipsPipeline(t *testing.T) {
	es := &mockEventStore{}
	runner := newSafeRunner()
	// Session is missing (has-session fails)
	runner.AddResponse("", errNoSession) // tmux has-session -> missing

	pr := &mockPipelineRunner{result: pipeline.StagePassed}

	agents := []ActiveAgent{
		{Assignment: Assignment{StoryID: "s-1", SessionName: "px-s-1"}, RuntimeName: "claude-code"},
	}

	p := NewPoller(PollerConfig{PollIntervalMs: 10}, runner, nil, pr, es, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p.Run(ctx, agents, ".")

	pr.mu.Lock()
	defer pr.mu.Unlock()
	if len(pr.runs) != 0 {
		t.Error("pipeline should not run for dead/missing agent session")
	}
}
