package e2e

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/cost"
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/graph"
	"github.com/tzone85/px-dispatch/internal/llm"
	"github.com/tzone85/px-dispatch/internal/monitor"
	"github.com/tzone85/px-dispatch/internal/pipeline"
	"github.com/tzone85/px-dispatch/internal/planner"
	"github.com/tzone85/px-dispatch/internal/state"
	"github.com/tzone85/px-dispatch/internal/tmux"
)

// setupTestStores creates ephemeral file-based event store and SQLite
// projection store in a temporary directory. Both are cleaned up when
// the test completes.
func setupTestStores(t *testing.T) (state.EventStore, *state.SQLiteStore) {
	t.Helper()
	dir := t.TempDir()

	es, err := state.NewFileStore(dir + "/events.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	ps, err := state.NewSQLiteStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ps.Close() })

	return es, ps
}

// TestFullPipeline_PlanDispatchMerge validates the complete flow:
// requirement -> stories -> DAG -> wave dispatch -> pipeline stages
// -> merge -> next wave unlock -> all done.
func TestFullPipeline_PlanDispatchMerge(t *testing.T) {
	es, ps := setupTestStores(t)

	// 1. Submit requirement
	reqID := "req-test-1"
	reqEvt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": reqID, "title": "Add user auth", "description": "Implement OAuth2 login", "repo_path": "/tmp/repo",
	})
	if err := es.Append(reqEvt); err != nil {
		t.Fatal(err)
	}
	if err := ps.Project(reqEvt); err != nil {
		t.Fatal(err)
	}

	// 2. Create planned stories with dependencies
	stories := []planner.PlannedStory{
		{ID: "s-1", Title: "Setup DB schema", Complexity: 2, OwnedFiles: []string{"db.go"}, DependsOn: []string{}},
		{ID: "s-2", Title: "Add user model", Complexity: 3, OwnedFiles: []string{"user.go"}, DependsOn: []string{"s-1"}},
		{ID: "s-3", Title: "Add auth endpoints", Complexity: 5, OwnedFiles: []string{"auth.go"}, DependsOn: []string{"s-2"}},
	}

	for _, s := range stories {
		evt := state.NewEvent(state.EventStoryCreated, "planner", s.ID, map[string]any{
			"id": s.ID, "req_id": reqID, "title": s.Title,
			"description": "desc", "acceptance_criteria": "ac",
			"complexity": s.Complexity, "owned_files": s.OwnedFiles,
			"wave_hint": "parallel", "depends_on": s.DependsOn,
		})
		if err := es.Append(evt); err != nil {
			t.Fatal(err)
		}
		if err := ps.Project(evt); err != nil {
			t.Fatal(err)
		}
	}

	// Mark requirement as planned (uses req_id key per ReqStatusPayload)
	plannedEvt := state.NewEvent(state.EventReqPlanned, "planner", "", map[string]any{"req_id": reqID})
	if err := es.Append(plannedEvt); err != nil {
		t.Fatal(err)
	}
	if err := ps.Project(plannedEvt); err != nil {
		t.Fatal(err)
	}

	// 3. Build DAG
	dag := graph.NewDAG()
	for _, s := range stories {
		dag.AddNode(s.ID)
		for _, dep := range s.DependsOn {
			dag.AddEdge(dep, s.ID)
		}
	}

	// 4. Verify wave grouping
	waves, err := graph.GroupByWave(dag)
	if err != nil {
		t.Fatalf("wave grouping: %v", err)
	}
	if len(waves) != 3 {
		t.Fatalf("expected 3 waves, got %d", len(waves))
	}
	if waves[0][0] != "s-1" {
		t.Errorf("wave 0 should be [s-1], got %v", waves[0])
	}

	// 5. Dispatch wave 1
	storyMap := make(map[string]planner.PlannedStory)
	for _, s := range stories {
		storyMap[s.ID] = s
	}

	dispatcher := monitor.NewDispatcher(config.RoutingConfig{
		JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5,
	})

	completed := map[string]bool{}
	wave1, err := dispatcher.DispatchWave(dag, completed, reqID, storyMap, 1)
	if err != nil {
		t.Fatalf("dispatch wave 1: %v", err)
	}
	if len(wave1) != 1 || wave1[0].StoryID != "s-1" {
		t.Fatalf("wave 1 should dispatch s-1 only, got %v", wave1)
	}
	if wave1[0].Role != "junior" {
		t.Errorf("s-1 (complexity 2) should be junior, got %s", wave1[0].Role)
	}

	// 6. Simulate agent completion + pipeline
	mockRunner := git.NewMockRunner()

	// autocommit: git status shows changes, git add, git commit
	mockRunner.AddResponse("M file.go", nil) // git status --porcelain
	mockRunner.AddResponse("", nil)           // git add -A
	mockRunner.AddResponse("", nil)           // git commit

	// diffcheck: merge-base, diff --name-only
	mockRunner.AddResponse("abc123", nil) // git merge-base HEAD origin/main
	mockRunner.AddResponse("db.go", nil)  // git diff --name-only abc123 HEAD

	// review: merge-base, diff, ls-files (+ LLM call)
	mockRunner.AddResponse("abc123", nil)    // git merge-base HEAD origin/main
	mockRunner.AddResponse("+new code", nil) // git diff abc123
	mockRunner.AddResponse("db.go", nil)     // git ls-files

	// qa: git ls-files (detect go.mod), go vet, go test
	mockRunner.AddResponse("go.mod\ndb.go", nil) // git ls-files
	mockRunner.AddResponse("", nil)               // go vet ./...
	mockRunner.AddResponse("ok", nil)             // go test ./... -race

	// rebase: git fetch, git rebase (succeeds)
	mockRunner.AddResponse("", nil) // git fetch origin main
	mockRunner.AddResponse("", nil) // git rebase origin/main

	// merge: git push, gh pr create, gh pr merge
	mockRunner.AddResponse("", nil)                                    // git push -u origin px/s-1
	mockRunner.AddResponse("https://github.com/user/repo/pull/1", nil) // gh pr create
	mockRunner.AddResponse("", nil)                                    // gh pr merge 1 --squash --auto

	// cleanup: worktree remove, branch -D, push --delete
	mockRunner.AddResponse("", nil) // git worktree remove
	mockRunner.AddResponse("", nil) // git branch -D
	mockRunner.AddResponse("", nil) // git push origin --delete

	// Review LLM returns "passed"
	reviewClient := llm.NewReplayClient(
		llm.CompletionResponse{Content: `{"passed": true, "summary": "LGTM", "comments": []}`, Model: "test"},
	)

	pipelineStages := []pipeline.Stage{
		pipeline.NewAutoCommitStage(mockRunner),
		pipeline.NewDiffCheckStage(mockRunner),
		pipeline.NewReviewStage(mockRunner, reviewClient),
		pipeline.NewQAStage(mockRunner),
		pipeline.NewRebaseStage(mockRunner, reviewClient, 10),
		pipeline.NewMergeStage(mockRunner, true),
		pipeline.NewCleanupStage(mockRunner),
	}
	runner := pipeline.NewRunner(pipelineStages, config.PipelineConfig{}, es)

	sc := pipeline.StoryContext{
		StoryID: "s-1", ReqID: reqID, Branch: "px/s-1",
		WorktreePath: "/tmp/wt/s-1", RepoDir: "/tmp/repo", BaseBranch: "main",
	}
	result, err := runner.Run(context.Background(), sc)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	if result != pipeline.StagePassed {
		t.Errorf("expected pipeline passed, got %s", result)
	}

	// 7. Mark story as merged and verify wave 2 unlocks
	mergedEvt := state.NewEvent(state.EventStoryMerged, "merger", "s-1", map[string]any{})
	if err := es.Append(mergedEvt); err != nil {
		t.Fatal(err)
	}
	if err := ps.Project(mergedEvt); err != nil {
		t.Fatal(err)
	}

	completed["s-1"] = true
	wave2, err := dispatcher.DispatchWave(dag, completed, reqID, storyMap, 2)
	if err != nil {
		t.Fatalf("dispatch wave 2: %v", err)
	}
	if len(wave2) != 1 || wave2[0].StoryID != "s-2" {
		t.Fatalf("wave 2 should dispatch s-2, got %v", wave2)
	}

	// 8. Complete s-2, dispatch wave 3
	completed["s-2"] = true
	wave3, err := dispatcher.DispatchWave(dag, completed, reqID, storyMap, 3)
	if err != nil {
		t.Fatalf("dispatch wave 3: %v", err)
	}
	if len(wave3) != 1 || wave3[0].StoryID != "s-3" {
		t.Fatalf("wave 3 should dispatch s-3, got %v", wave3)
	}
	if wave3[0].Role != "intermediate" {
		t.Errorf("s-3 (complexity 5) should be intermediate, got %s", wave3[0].Role)
	}

	// 9. Complete all, verify no more stories to dispatch
	completed["s-3"] = true
	waveN, err := dispatcher.DispatchWave(dag, completed, reqID, storyMap, 4)
	if err != nil {
		t.Fatalf("dispatch wave 4: %v", err)
	}
	if len(waveN) != 0 {
		t.Error("no more stories should be ready")
	}

	// Verify s-1 is in merged state in the projection store
	storedStories, err := ps.ListStories(state.StoryFilter{ReqID: reqID})
	if err != nil {
		t.Fatalf("list stories: %v", err)
	}
	for _, s := range storedStories {
		if s.ID == "s-1" && s.Status != "merged" {
			t.Errorf("s-1 should be merged, got %s", s.Status)
		}
	}
}

// TestBudgetExhaustion_PausesRequirement validates that the budget
// breaker blocks LLM calls when the per-story budget is exceeded
// and that the error is classified as fatal.
func TestBudgetExhaustion_PausesRequirement(t *testing.T) {
	es, ps := setupTestStores(t)

	// Create an LLM client that returns token usage
	client := llm.NewReplayClient(
		llm.CompletionResponse{
			Content:      "result",
			Model:        "claude-sonnet-4-20250514",
			InputTokens:  500000,
			OutputTokens: 200000,
		},
	)

	// Create a ledger with the test DB
	ledger := cost.NewSQLiteLedger(ps.DB(), cost.DefaultPricing)

	// Create breaker with tiny budget so first call's recorded cost exceeds it
	breaker := cost.NewBudgetBreaker(client, ledger, config.BudgetConfig{
		MaxCostPerStoryUSD:       0.001, // tiny budget
		MaxCostPerRequirementUSD: 100.0,
		MaxCostPerDayUSD:         100.0,
		HardStop:                 true,
	}, cost.DefaultPricing, es, cost.BudgetContext{StoryID: "s-1", ReqID: "r-1"})

	// First call should succeed (budget not yet tracked)
	resp, err := breaker.Complete(context.Background(), llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	if resp.Content != "result" {
		t.Errorf("expected 'result', got %q", resp.Content)
	}

	// Second call should be blocked by budget (pre-call check sees recorded cost)
	_, err = breaker.Complete(context.Background(), llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello again"}},
	})
	if err == nil {
		t.Fatal("second call should fail due to budget")
	}

	var budgetErr *llm.BudgetExhaustedError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExhaustedError, got %T: %v", err, err)
	}

	// Verify it's classified as fatal
	if !llm.IsFatalAPIError(err) {
		t.Error("budget exhaustion should be classified as fatal")
	}
}

// TestSessionHealth_DeadDetection validates that a session whose pane
// process has exited is correctly identified as Dead.
func TestSessionHealth_DeadDetection(t *testing.T) {
	mockRunner := git.NewMockRunner()

	// has-session succeeds (session exists)
	mockRunner.AddResponse("", nil)
	// list-panes: pane_dead=1 means process is dead
	mockRunner.AddResponse("12345 1 1", nil)

	result := tmux.SessionHealth(mockRunner, "test-session", "")
	if result.Status != tmux.Dead {
		t.Errorf("expected Dead, got %s", result.Status)
	}
}

// TestSessionHealth_StaleDetection validates that a session whose
// output hash matches the previous check is identified as Stale.
func TestSessionHealth_StaleDetection(t *testing.T) {
	mockRunner := git.NewMockRunner()

	// has-session succeeds
	mockRunner.AddResponse("", nil)
	// list-panes: pane_dead=0 means alive
	mockRunner.AddResponse("12345 0 0", nil)
	// capture-pane returns output that matches previousHash
	mockRunner.AddResponse("same old output", nil)

	// Compute the hash the same way health.go does
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("same old output")))
	result := tmux.SessionHealth(mockRunner, "test-session", hash)
	if result.Status != tmux.Stale {
		t.Errorf("expected Stale, got %s", result.Status)
	}
}
