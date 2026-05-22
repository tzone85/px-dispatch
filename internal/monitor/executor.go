package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tzone85/project-x/internal/agent"
	"github.com/tzone85/project-x/internal/config"
	"github.com/tzone85/project-x/internal/git"
	"github.com/tzone85/project-x/internal/planner"
	"github.com/tzone85/project-x/internal/runtime"
	"github.com/tzone85/project-x/internal/state"
)

// EventSender is the interface for asynchronous event projection.
// The Projector satisfies this interface via its Send method.
type EventSender interface {
	Send(evt state.Event)
}

// ActiveAgent represents an agent that has been spawned and is running.
type ActiveAgent struct {
	Assignment     Assignment
	WorktreePath   string
	RuntimeName    string
	Model          string
	Story          planner.PlannedStory
	TranscriptPath string
}

// SpawnResult captures the outcome of spawning a single agent.
type SpawnResult struct {
	Assignment     Assignment
	WorktreePath   string
	RuntimeName    string
	Model          string
	TranscriptPath string
	Error          error
}

// Executor creates worktrees, writes agent instructions, and spawns
// runtime sessions for each assignment in a wave.
type Executor struct {
	runner     git.CommandRunner
	router     *runtime.Router
	config     config.Config
	eventStore state.EventStore
	projector  EventSender
}

// NewExecutor creates an Executor with the given dependencies.
func NewExecutor(
	runner git.CommandRunner,
	router *runtime.Router,
	cfg config.Config,
	es state.EventStore,
	proj EventSender,
) *Executor {
	return &Executor{
		runner:     runner,
		router:     router,
		config:     cfg,
		eventStore: es,
		projector:  proj,
	}
}

// SpawnAll iterates over assignments, creating a worktree and spawning a
// runtime session for each. Results are returned in the same order as
// assignments. Individual spawn failures are captured in SpawnResult.Error
// without aborting the remaining assignments.
func (e *Executor) SpawnAll(
	repoDir string,
	assignments []Assignment,
	stories map[string]planner.PlannedStory,
) []SpawnResult {
	results := make([]SpawnResult, 0, len(assignments))

	for _, a := range assignments {
		result := e.spawnOne(repoDir, a, stories)
		results = append(results, result)
	}

	return results
}

// spawnOne handles the full lifecycle for a single assignment:
// 1. Create worktree directory and git worktree
// 2. Write CLAUDE.md with agent instructions
// 3. Select runtime via router
// 4. Build prompts and spawn session
// 5. Emit EventStoryStarted
func (e *Executor) spawnOne(
	repoDir string,
	a Assignment,
	stories map[string]planner.PlannedStory,
) SpawnResult {
	story := stories[a.StoryID]
	worktreePath := filepath.Join(e.config.Workspace.StateDir, "worktrees", a.StoryID)

	// Step 1: Create worktree directory and git worktree with branch.
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return SpawnResult{Assignment: a, Error: fmt.Errorf("creating worktree parent dir: %w", err)}
	}

	if err := git.CreateWorktree(e.runner, repoDir, worktreePath, a.Branch); err != nil {
		return SpawnResult{Assignment: a, Error: fmt.Errorf("creating worktree for %s: %w", a.StoryID, err)}
	}

	// Step 2: Write CLAUDE.md with agent instructions.
	claudeMD := buildClaudeMD(a, story)
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		return SpawnResult{Assignment: a, Error: fmt.Errorf("ensuring worktree dir: %w", err)}
	}
	claudeMDPath := filepath.Join(worktreePath, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte(claudeMD), 0o644); err != nil {
		return SpawnResult{Assignment: a, Error: fmt.Errorf("writing CLAUDE.md for %s: %w", a.StoryID, err)}
	}

	// Step 3: Select runtime via router.
	rt, err := e.router.SelectRuntime(a.Role)
	if err != nil {
		return SpawnResult{Assignment: a, Error: fmt.Errorf("selecting runtime for %s: %w", a.StoryID, err)}
	}

	// Step 4: Build prompts and spawn runtime session.
	promptCtx := agent.PromptContext{
		TeamName:           "Project X",
		StoryID:            a.StoryID,
		StoryTitle:         story.Title,
		StoryDescription:   story.Description,
		AcceptanceCriteria: story.AcceptanceCriteria,
		RepoPath:           worktreePath,
		Complexity:         story.Complexity,
		IsExistingCodebase: detectExistingCodebase(worktreePath),
		IsBugFix:           classifyIsBugFix(story.Title, story.Description),
		IsInfrastructure:   classifyIsInfrastructure(story.Title, story.Description, story.OwnedFiles),
		DesignApproach:     "ddd-tdd", // project-x default — agents follow DDD+TDD unless overridden
		WaveContext:        e.loadWaveContext(worktreePath),
	}

	goal := agent.GoalPrompt(a.Role, promptCtx)
	systemPrompt := agent.SystemPrompt(a.Role, promptCtx)

	sessionCfg := runtime.SessionConfig{
		SessionName:  a.SessionName,
		WorkDir:      worktreePath,
		Goal:         goal,
		SystemPrompt: systemPrompt,
	}

	modelCfg := a.Role.ModelConfig(e.config.Models)
	if modelCfg.Model != "" {
		sessionCfg.Model = modelCfg.Model
	}
	if rt.Capabilities().SupportsLogFile {
		sessionCfg.LogFile = filepath.Join(worktreePath, transcriptFileName)
	}

	if err := rt.Spawn(e.runner, sessionCfg); err != nil {
		return SpawnResult{Assignment: a, Error: fmt.Errorf("spawning runtime for %s: %w", a.StoryID, err)}
	}

	// Step 5: Emit EventStoryStarted.
	evt := state.NewEvent(state.EventStoryStarted, a.AgentID, a.StoryID, map[string]any{
		"wave":         a.Wave,
		"role":         string(a.Role),
		"branch":       a.Branch,
		"session_name": a.SessionName,
		"runtime":      rt.Name(),
	})

	_ = e.eventStore.Append(evt)
	e.projector.Send(evt)

	return SpawnResult{
		Assignment:     a,
		WorktreePath:   worktreePath,
		RuntimeName:    rt.Name(),
		Model:          sessionCfg.Model,
		TranscriptPath: sessionCfg.LogFile,
	}
}

// buildClaudeMD generates the CLAUDE.md content that suppresses brainstorming
// and skills, providing focused instructions for the agent.
func buildClaudeMD(a Assignment, story planner.PlannedStory) string {
	var b strings.Builder

	b.WriteString("# Agent Instructions\n\n")
	b.WriteString("## Constraints\n\n")
	b.WriteString("- Do NOT brainstorm or explore alternatives unless explicitly asked\n")
	b.WriteString("- Do NOT use slash commands or skills\n")
	b.WriteString("- Focus exclusively on the assigned story\n")
	b.WriteString("- Commit your work when the acceptance criteria are met\n")
	b.WriteString("- Do NOT modify files outside the scope of this story\n\n")

	b.WriteString(fmt.Sprintf("## Story: %s\n\n", story.Title))
	b.WriteString(fmt.Sprintf("**ID:** %s\n", a.StoryID))
	b.WriteString(fmt.Sprintf("**Branch:** %s\n", a.Branch))
	b.WriteString(fmt.Sprintf("**Role:** %s\n", string(a.Role)))
	b.WriteString(fmt.Sprintf("**Wave:** %d\n\n", a.Wave))

	if story.Description != "" {
		b.WriteString("### Description\n\n")
		b.WriteString(story.Description)
		b.WriteString("\n\n")
	}

	if story.AcceptanceCriteria != "" {
		b.WriteString("### Acceptance Criteria\n\n")
		b.WriteString(story.AcceptanceCriteria)
		b.WriteString("\n\n")
	}

	if len(story.OwnedFiles) > 0 {
		b.WriteString("### Owned Files\n\n")
		for _, f := range story.OwnedFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	return b.String()
}
