package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/graph"
	"github.com/tzone85/px-dispatch/internal/monitor"
	"github.com/tzone85/px-dispatch/internal/pipeline"
	"github.com/tzone85/px-dispatch/internal/planner"
	"github.com/tzone85/px-dispatch/internal/runtime"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newResumeCmd() *cobra.Command {
	var godmode bool

	cmd := &cobra.Command{
		Use:   "resume <req-id>",
		Short: "Dispatch and monitor agents for a planned requirement",
		Long:  "Builds the dependency DAG, dispatches the next wave of agents, and monitors through the full pipeline (review, QA, rebase, merge).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(cmd.Context(), args[0], godmode)
		},
	}

	cmd.Flags().BoolVar(&godmode, "godmode", false, "enable autonomous operation (skip permission prompts)")
	return cmd
}

func runResume(ctx context.Context, reqID string, godmode bool) error {
	// 1. Load requirement and validate it exists
	req, err := app.projStore.GetRequirement(reqID)
	if err != nil {
		return fmt.Errorf("requirement %s not found: %w", reqID, err)
	}
	if req.Status == "archived" {
		return fmt.Errorf("requirement %s is archived", reqID)
	}

	fmt.Printf("Resuming requirement: %s (%s)\n", reqID, req.Title)

	// 2. Load stories and build DAG
	stories, err := app.projStore.ListStories(state.StoryFilter{ReqID: reqID})
	if err != nil {
		return fmt.Errorf("list stories: %w", err)
	}
	if len(stories) == 0 {
		return fmt.Errorf("no stories found for requirement %s — run 'px plan' first", reqID)
	}

	deps, err := app.projStore.ListStoryDeps(reqID)
	if err != nil {
		return fmt.Errorf("list story deps: %w", err)
	}

	dag := graph.NewDAG()
	storyMap := make(map[string]planner.PlannedStory, len(stories))
	completed := make(map[string]bool)

	for _, s := range stories {
		dag.AddNode(s.ID)
		storyMap[s.ID] = planner.PlannedStory{
			ID: s.ID, Title: s.Title, Description: s.Description,
			AcceptanceCriteria: s.AcceptanceCriteria, Complexity: s.Complexity,
			OwnedFiles: s.OwnedFiles, WaveHint: s.WaveHint,
		}
		if s.Status == "merged" || s.Status == "pr_submitted" {
			completed[s.ID] = true
		}
	}
	for _, d := range deps {
		dag.AddEdge(d.DependsOnID, d.StoryID)
	}

	if len(completed) == len(stories) {
		fmt.Println("All stories are already complete!")
		return nil
	}

	// 3. Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived shutdown signal, finishing in-flight work...")
		cancel()
	}()

	// 4. Determine repo directory
	repoDir := req.RepoPath
	if repoDir == "" {
		repoDir, _ = os.Getwd()
	}

	// 5. Set up runtime registry and router
	runner := git.ExecRunner{}
	reg := runtime.NewRegistry()
	reg.Register("claude-code", runtime.NewClaudeCodeRuntime(godmode))
	reg.Register("codex", runtime.NewCodexRuntime(godmode))
	reg.Register("gemini", runtime.NewGeminiRuntime())

	router := runtime.NewRouter(reg, app.config)

	// 6. Set up the pipeline stages
	llmClient := buildLLMClient()
	pipelineStages := []pipeline.Stage{
		pipeline.NewAutoCommitStage(runner),
		pipeline.NewDiffCheckStage(runner),
		pipeline.NewReviewStage(runner, llmClient),
		pipeline.NewQAStage(runner),
		pipeline.NewRebaseStage(runner, llmClient, 10),
		pipeline.NewMergeStage(runner, app.config.Merge.AutoMerge),
		pipeline.NewCleanupStage(runner),
	}
	pipelineRunner := pipeline.NewRunner(pipelineStages, app.config.Pipeline, app.eventStore)

	// 7. Set up dispatcher, executor, watchdog, poller
	dispatcher := monitor.NewDispatcher(app.config.Routing)
	executor := monitor.NewExecutor(runner, router, app.config, app.eventStore, app.projector)
	watchdog := monitor.NewWatchdog(monitor.WatchdogConfig{
		StuckThresholdS: app.config.Monitor.StuckThresholdS,
	}, app.eventStore)

	poller := monitor.NewPoller(
		monitor.PollerConfig{PollIntervalMs: app.config.Monitor.PollIntervalMs},
		runner, watchdog, pipelineRunner, app.eventStore, reg, app.projector,
		monitor.NewRuntimeFallbackManager(
			runner, reg, app.config.Fallback, app.eventStore, app.projector, modelSwitchApprover,
		),
	)

	// 8. Wave loop: dispatch → monitor → repeat until all done
	return runWaveLoop(ctx, waveLoopDeps{
		dispatcher: dispatcher,
		executor:   executor,
		poller:     poller,
		dag:        dag,
		storyMap:   storyMap,
		completed:  completed,
		stories:    stories,
		reqID:      reqID,
		repoDir:    repoDir,
	})
}

// waveDispatcher abstracts monitor.Dispatcher for testability.
type waveDispatcher interface {
	DispatchWave(dag *graph.DAG, completed map[string]bool, reqID string, storyMap map[string]planner.PlannedStory, wave int) ([]monitor.Assignment, error)
}

// waveExecutor abstracts monitor.Executor for testability.
type waveExecutor interface {
	SpawnAll(repoDir string, assignments []monitor.Assignment, storyMap map[string]planner.PlannedStory) []monitor.SpawnResult
}

// wavePoller abstracts monitor.Poller for testability.
type wavePoller interface {
	Run(ctx context.Context, agents []monitor.ActiveAgent, repoDir string) error
}

// waveLoopDeps bundles the inputs that the wave-loop body needs. Splitting it
// out lets tests substitute fake dispatcher/executor/poller without touching
// the rest of runResume.
type waveLoopDeps struct {
	dispatcher waveDispatcher
	executor   waveExecutor
	poller     wavePoller
	dag        *graph.DAG
	storyMap   map[string]planner.PlannedStory
	completed  map[string]bool
	stories    []state.Story
	reqID      string
	repoDir    string
}

func runWaveLoop(ctx context.Context, d waveLoopDeps) error {
	waveNumber := 0
	for {
		waveNumber++
		assignments, err := d.dispatcher.DispatchWave(d.dag, d.completed, d.reqID, d.storyMap, waveNumber)
		if err != nil {
			return fmt.Errorf("dispatch wave %d: %w", waveNumber, err)
		}
		if len(assignments) == 0 {
			allDone := len(d.completed) == len(d.stories)
			if allDone {
				break
			}
			fmt.Println("No stories ready for dispatch (dependencies not met). Waiting...")
			break
		}

		fmt.Printf("\nWave %d: dispatching %d stories\n", waveNumber, len(assignments))
		for _, a := range assignments {
			fmt.Printf("  %s → %s (branch: %s)\n", a.StoryID, a.Role, a.Branch)
		}

		for _, a := range assignments {
			evt := state.NewEvent(state.EventStoryAssigned, a.AgentID, a.StoryID, map[string]any{
				"agent_id": a.AgentID,
				"wave":     waveNumber,
			})
			app.eventStore.Append(evt)
			app.projector.Send(evt)
		}

		results := d.executor.SpawnAll(d.repoDir, assignments, d.storyMap)
		var activeAgents []monitor.ActiveAgent
		for _, r := range results {
			if r.Error != nil {
				fmt.Printf("  ERROR spawning %s: %v\n", r.Assignment.StoryID, r.Error)
				continue
			}
			activeAgents = append(activeAgents, monitor.ActiveAgent{
				Assignment:     r.Assignment,
				WorktreePath:   r.WorktreePath,
				RuntimeName:    r.RuntimeName,
				Model:          r.Model,
				Story:          d.storyMap[r.Assignment.StoryID],
				TranscriptPath: r.TranscriptPath,
			})
		}

		if len(activeAgents) == 0 {
			fmt.Println("No agents spawned successfully")
			break
		}

		if err := d.poller.Run(ctx, activeAgents, d.repoDir); err != nil {
			return fmt.Errorf("poller: %w", err)
		}

		refreshedStories, _ := app.projStore.ListStories(state.StoryFilter{ReqID: d.reqID})
		for _, s := range refreshedStories {
			if s.Status == "merged" || s.Status == "pr_submitted" {
				d.completed[s.ID] = true
			}
		}

		if ctx.Err() != nil {
			fmt.Println("Shutdown complete. Resume later with: px resume", d.reqID)
			return nil
		}
	}

	if len(d.completed) == len(d.stories) {
		compEvt := state.NewEvent(state.EventReqCompleted, "monitor", "", map[string]any{"id": d.reqID})
		app.eventStore.Append(compEvt)
		app.projector.Send(compEvt)
		fmt.Printf("\nAll %d stories complete! Requirement %s is done.\n", len(d.stories), d.reqID)

		// Fire-and-forget cleanup: remove worktrees + kill tmux panes for
		// every story under this requirement so the workspace is empty when
		// the user comes back. Branches stay locally for a moment because the
		// merge stage may still have a final commit in transit; the next gc
		// run (manual or scheduled) will reap them.
		autoCleanupAfterCompletion(d.reqID, d.repoDir, d.stories)
	}

	return nil
}

// autoCleanupAfterCompletion deletes worktrees and kills tmux sessions
// belonging to a completed requirement so the user does not have to run gc.
// Best-effort: failures are logged but do not propagate.
func autoCleanupAfterCompletion(reqID, repoDir string, stories []state.Story) {
	worktreesDir := filepath.Join(app.stateDir, "worktrees")
	for _, s := range stories {
		path := filepath.Join(worktreesDir, s.ID)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		// Use git's worktree-aware remove first; fall back to plain rm.
		if err := runGit(repoDir, "worktree", "remove", path, "--force"); err != nil {
			_ = os.RemoveAll(path)
		}
		_ = runGit(repoDir, "branch", "-D", "px/"+s.ID)
		_ = runShellQuiet("tmux", "kill-session", "-t", "px-"+s.ID)
	}
	fmt.Println("Cleanup complete: worktrees + tmux sessions for completed stories removed.")
}
