package monitor

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/pipeline"
	"github.com/tzone85/px-dispatch/internal/runtime"
	"github.com/tzone85/px-dispatch/internal/state"
	"github.com/tzone85/px-dispatch/internal/tmux"
)

var pollerIdleRe = regexp.MustCompile(`(?m)^\$\s*$`)

// completionSentinel is the filename a runtime touches in the worktree once
// the agent has finished its turn successfully. The poller checks for it so
// the pipeline can advance even after the tmux session has already exited.
const completionSentinel = ".px-done"

// PollerConfig holds configuration for the polling loop.
type PollerConfig struct {
	PollIntervalMs int
}

// PipelineRunner abstracts the pipeline.Runner for testing.
type PipelineRunner interface {
	Run(ctx context.Context, sc pipeline.StoryContext) (pipeline.StageResult, error)
}

// RuntimeFallbacker can replace a Claude agent with an approved fallback
// runtime when Claude account limits stop progress.
type RuntimeFallbacker interface {
	TrySwitch(ctx context.Context, ag ActiveAgent, output string) (ActiveAgent, bool, error)
}

// Poller polls active agent sessions and drives post-execution pipelines.
type Poller struct {
	config     PollerConfig
	runner     git.CommandRunner
	watchdog   *Watchdog
	pipeline   PipelineRunner
	eventStore state.EventStore
	registry   *runtime.Registry
	projector  EventSender
	fallbacker RuntimeFallbacker

	mergeMu sync.Mutex // serializes rebase-push-merge
}

// NewPoller creates a Poller with the given dependencies.
func NewPoller(
	cfg PollerConfig,
	runner git.CommandRunner,
	wd *Watchdog,
	pr PipelineRunner,
	es state.EventStore,
	reg *runtime.Registry,
	proj EventSender,
	fallbacker RuntimeFallbacker,
) *Poller {
	return &Poller{
		config:     cfg,
		runner:     runner,
		watchdog:   wd,
		pipeline:   pr,
		eventStore: es,
		registry:   reg,
		projector:  proj,
		fallbacker: fallbacker,
	}
}

// Run polls agents until all finish or the context is cancelled.
func (p *Poller) Run(ctx context.Context, agents []ActiveAgent, repoDir string) error {
	if len(agents) == 0 {
		return nil
	}

	pollInterval := time.Duration(p.config.PollIntervalMs) * time.Millisecond
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	active := make(map[string]ActiveAgent, len(agents))
	for _, a := range agents {
		active[a.Assignment.SessionName] = a
	}

	var pipelineWG sync.WaitGroup
	slog.Info("poller started", "agents", len(active), "interval", pollInterval)
	for {
		select {
		case <-ctx.Done():
			slog.Info("poller stopping, waiting for pipelines", "active", len(active))
			pipelineWG.Wait()
			return nil

		case <-ticker.C:
			if len(active) == 0 {
				slog.Info("all agents finished, waiting for pipelines")
				pipelineWG.Wait()
				return nil
			}
			p.pollOnce(ctx, &pipelineWG, active, repoDir)
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context, wg *sync.WaitGroup, active map[string]ActiveAgent, repoDir string) {
	for sessionName, ag := range active {
		health := tmux.SessionHealth(p.runner, sessionName, "")

		if health.Status == tmux.Dead || health.Status == tmux.Missing {
			if sentinelExists(ag.WorktreePath) {
				slog.Info("agent finished (sentinel after session exit)",
					"story", ag.Assignment.StoryID,
					"status", health.Status)
				p.runPipelineForCompletedAgent(ctx, wg, ag, repoDir)
				delete(active, sessionName)
				continue
			}
			slog.Warn("agent session dead/missing", "story", ag.Assignment.StoryID, "status", health.Status)
			p.emitAgentEvent(ag, health)
			delete(active, sessionName)
			continue
		}

		if p.watchdog != nil {
			rt := p.resolveRuntime(ag.RuntimeName)
			p.watchdog.Check(p.runner, sessionName, rt)
		}

		output, err := tmux.ReadOutput(p.runner, sessionName, 5)
		if err != nil {
			continue
		}
		if p.fallbacker != nil {
			updated, switched, switchErr := p.fallbacker.TrySwitch(ctx, ag, output)
			if switchErr != nil {
				slog.Error("runtime fallback failed", "story", ag.Assignment.StoryID, "error", switchErr)
				delete(active, sessionName)
				continue
			}
			if switched {
				if p.watchdog != nil {
					p.watchdog.ClearFingerprint(sessionName)
				}
				if updated.Assignment.SessionName != sessionName {
					delete(active, sessionName)
				}
				active[updated.Assignment.SessionName] = updated
				continue
			}
		}
		if !isAgentDone(output) && !sentinelExists(ag.WorktreePath) {
			continue
		}

		slog.Info("agent finished", "story", ag.Assignment.StoryID)
		p.runPipelineForCompletedAgent(ctx, wg, ag, repoDir)

		if p.watchdog != nil {
			p.watchdog.ClearFingerprint(sessionName)
		}
		delete(active, sessionName)
	}
}

// runPipelineForCompletedAgent emits the agent-finished event and dispatches
// the pipeline goroutine. Shared between the in-session "$" sentinel path and
// the post-exit `.px-done` sentinel path so the trigger logic stays in one
// place.
func (p *Poller) runPipelineForCompletedAgent(ctx context.Context, wg *sync.WaitGroup, ag ActiveAgent, repoDir string) {
	p.emitCompleted(ag)
	wg.Add(1)
	go func(a ActiveAgent) {
		defer wg.Done()
		p.mergeMu.Lock()
		defer p.mergeMu.Unlock()

		sc := pipeline.StoryContext{
			StoryID:            a.Assignment.StoryID,
			Branch:             a.Assignment.Branch,
			WorktreePath:       a.WorktreePath,
			RepoDir:            repoDir,
			AgentID:            a.Assignment.AgentID,
			RuntimeName:        a.RuntimeName,
			BaseBranch:         "main",
			StoryTitle:         a.Story.Title,
			StoryDescription:   a.Story.Description,
			AcceptanceCriteria: a.Story.AcceptanceCriteria,
			OwnedFiles:         a.Story.OwnedFiles,
		}
		result, err := p.pipeline.Run(ctx, sc)
		if err != nil {
			slog.Error("pipeline error", "story", a.Assignment.StoryID, "error", err)
		}
		slog.Info("pipeline complete", "story", a.Assignment.StoryID, "result", result)
	}(ag)
}

// sentinelExists reports whether the runtime has written the completion
// sentinel into the worktree. Empty worktreePath returns false.
func sentinelExists(worktreePath string) bool {
	if worktreePath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(worktreePath, completionSentinel))
	return err == nil
}

func isAgentDone(output string) bool {
	trimmed := strings.TrimRight(output, " \t\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 {
		return false
	}
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	return pollerIdleRe.MatchString(lastLine)
}

func (p *Poller) resolveRuntime(name string) runtime.Runtime {
	if p.registry == nil {
		return nil
	}
	rt, _ := p.registry.Get(name)
	return rt
}

func (p *Poller) emit(evt state.Event) {
	_ = p.eventStore.Append(evt)
	if p.projector != nil {
		p.projector.Send(evt)
	}
}

func (p *Poller) emitCompleted(ag ActiveAgent) {
	p.emit(state.NewEvent(state.EventStoryCompleted, ag.Assignment.AgentID, ag.Assignment.StoryID, map[string]any{
		"session_name": ag.Assignment.SessionName,
		"runtime":      ag.RuntimeName,
	}))
}

func (p *Poller) emitAgentEvent(ag ActiveAgent, health tmux.HealthResult) {
	evtType := state.EventAgentDied
	if health.Status == tmux.Missing {
		evtType = state.EventAgentLost
	}
	p.emit(state.NewEvent(evtType, ag.Assignment.AgentID, ag.Assignment.StoryID, map[string]any{
		"session_name": ag.Assignment.SessionName,
		"health":       string(health.Status),
	}))
}
