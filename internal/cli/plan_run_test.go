package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tzone85/project-x/internal/llm"
	"github.com/tzone85/project-x/internal/state"
)

// stubLLM returns canned planner JSON responses.
type stubLLM struct {
	resp string
	err  error
}

func (s *stubLLM) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if s.err != nil {
		return llm.CompletionResponse{}, s.err
	}
	return llm.CompletionResponse{Content: s.resp, Model: "stub"}, nil
}

// withStubLLM swaps the package-level llmClientBuilder for a stub that returns
// the given canned response. Restored automatically when the test ends.
func withStubLLM(t *testing.T, resp string) {
	t.Helper()
	prev := llmClientBuilder
	llmClientBuilder = func() llm.Client { return &stubLLM{resp: resp} }
	t.Cleanup(func() { llmClientBuilder = prev })
}

func withStubLLMErr(t *testing.T, err error) {
	t.Helper()
	prev := llmClientBuilder
	llmClientBuilder = func() llm.Client { return &stubLLM{err: err} }
	t.Cleanup(func() { llmClientBuilder = prev })
}

const sampleStoriesJSON = `{
  "stories": [
    {
      "id": "s-1",
      "title": "implement endpoint",
      "description": "add a handler",
      "acceptance_criteria": "200 OK on GET",
      "complexity": 2,
      "owned_files": ["main.go"],
      "wave_hint": "parallel",
      "depends_on": []
    },
    {
      "id": "s-2",
      "title": "add tests",
      "description": "table-driven",
      "acceptance_criteria": "go test passes",
      "complexity": 1,
      "owned_files": ["main_test.go"],
      "wave_hint": "parallel",
      "depends_on": ["s-1"]
    }
  ]
}`

func TestRunPlan_Success(t *testing.T) {
	dir := setupTestApp(t)
	withStubLLM(t, sampleStoriesJSON)
	// Disable validation so the canned response always passes.
	app.config.Planning.MaxStoryComplexity = 0
	app.config.Planning.MaxStoriesPerRequirement = 0
	app.config.Planning.EnforceFileOwnership = false

	reqPath := filepath.Join(dir, "req.txt")
	if err := os.WriteFile(reqPath, []byte("add a /version endpoint"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runPlan(context.Background(), reqPath); err != nil {
			t.Fatalf("runPlan: %v", err)
		}
	})
	if !strings.Contains(out, "Stories: 2") {
		t.Errorf("expected Stories count, got %q", out)
	}
	if !strings.Contains(out, "Run 'px plan --review") {
		t.Errorf("expected review hint, got %q", out)
	}
}

func TestRunPlan_LLMError(t *testing.T) {
	dir := setupTestApp(t)
	withStubLLMErr(t, errors.New("upstream down"))

	reqPath := filepath.Join(dir, "req.txt")
	_ = os.WriteFile(reqPath, []byte("req"), 0o644)

	err := runPlan(context.Background(), reqPath)
	if err == nil || !strings.Contains(err.Error(), "planning failed") {
		t.Errorf("expected planning error, got %v", err)
	}
}

func TestRunPlan_ReadFileError(t *testing.T) {
	setupTestApp(t)
	if err := runPlan(context.Background(), "/no/such/file"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRunPlan_EventStoreClosed_FailsOnAppend(t *testing.T) {
	dir := setupTestApp(t)
	withStubLLM(t, sampleStoriesJSON)
	app.config.Planning.MaxStoryComplexity = 0
	app.config.Planning.MaxStoriesPerRequirement = 0
	app.config.Planning.EnforceFileOwnership = false

	reqPath := filepath.Join(dir, "req.txt")
	if err := os.WriteFile(reqPath, []byte("add /version"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Close the event store so every Append fails — covers the append-error
	// branch in runPlan.
	if err := app.eventStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	err := runPlan(context.Background(), reqPath)
	if err == nil {
		t.Error("expected append-error from closed event store")
	}
}

func TestRunPlanRefine_EventStoreClosed(t *testing.T) {
	setupTestApp(t)
	withStubLLM(t, sampleStoriesJSON)
	app.config.Planning.MaxStoryComplexity = 0
	app.config.Planning.MaxStoriesPerRequirement = 0
	app.config.Planning.EnforceFileOwnership = false

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-EVT", "title": "t", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}

	rPipe, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rPipe
	t.Cleanup(func() { os.Stdin = orig })
	go func() { _, _ = w.WriteString("redo it\n"); w.Close() }()

	if err := app.eventStore.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := runPlanRefine(context.Background(), "R-EVT"); err == nil {
		t.Error("expected error from closed event store during refine")
	}
}

func TestRunPlanReview_RequirementNotFound(t *testing.T) {
	setupTestApp(t)
	if err := runPlanReview("MISSING-REQ"); err == nil {
		t.Error("expected error for missing requirement")
	}
}

func TestRunPlanReview_NoStories(t *testing.T) {
	setupTestApp(t)
	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-X", "title": "t", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := runPlanReview("R-X"); err == nil || !strings.Contains(err.Error(), "no stories") {
		t.Errorf("expected no stories error, got %v", err)
	}
}

func TestRunPlanReview_Success(t *testing.T) {
	setupTestApp(t)

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-RV", "title": "title", "description": "d", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}

	// Add 2 stories, one with a long title and id to exercise the truncation
	// branches.
	for _, s := range []map[string]any{
		{
			"id": "S-RVA", "req_id": "R-RV",
			"title":               strings.Repeat("L", 50),
			"description":         "d",
			"acceptance_criteria": "a",
			"complexity":          1,
			"owned_files":         []string{},
			"wave_hint":           "parallel",
			"depends_on":          []string{},
		},
		{
			"id": "S-RV-LONG-ID-ABC", "req_id": "R-RV",
			"title":               "short",
			"description":         "d",
			"acceptance_criteria": "a",
			"complexity":          2,
			"owned_files":         []string{},
			"wave_hint":           "sequential",
			"depends_on":          []string{"S-RVA"},
		},
	} {
		evt := state.NewEvent(state.EventStoryCreated, "planner", s["id"].(string), s)
		if err := app.projStore.Project(evt); err != nil {
			t.Fatalf("project story: %v", err)
		}
	}

	out := captureStdout(t, func() {
		if err := runPlanReview("R-RV"); err != nil {
			t.Fatalf("runPlanReview: %v", err)
		}
	})
	if !strings.Contains(out, "R-RV") {
		t.Errorf("expected requirement id, got %q", out)
	}
	if !strings.Contains(out, "Stories: 2") {
		t.Errorf("expected story count, got %q", out)
	}
	if !strings.Contains(out, "..") {
		t.Errorf("expected truncation ellipsis, got %q", out)
	}
}

func TestRunPlanRefine_EmptyFeedback(t *testing.T) {
	setupTestApp(t)
	// Stdin will close immediately so scanner finds no feedback.
	r, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	w.Close()

	captureStdout(t, func() {
		err := runPlanRefine(context.Background(), "ANY")
		if err == nil || !strings.Contains(err.Error(), "no feedback") {
			t.Errorf("expected no-feedback error, got %v", err)
		}
	})
}

func TestRunPlanRefine_MissingRequirement(t *testing.T) {
	setupTestApp(t)
	r, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	go func() { _, _ = w.WriteString("more strict tests please\n"); w.Close() }()

	captureStdout(t, func() {
		err := runPlanRefine(context.Background(), "DOES-NOT-EXIST")
		if err == nil || !strings.Contains(err.Error(), "get requirement") {
			t.Errorf("expected requirement-not-found error, got %v", err)
		}
	})
}

func TestRunPlanRefine_Success(t *testing.T) {
	setupTestApp(t)
	withStubLLM(t, sampleStoriesJSON)
	app.config.Planning.MaxStoryComplexity = 0
	app.config.Planning.MaxStoriesPerRequirement = 0
	app.config.Planning.EnforceFileOwnership = false

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-REF", "title": "orig", "description": "original desc", "repo_path": ".",
	})
	if err := app.projStore.Project(r); err != nil {
		t.Fatalf("project: %v", err)
	}

	rPipe, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rPipe
	t.Cleanup(func() { os.Stdin = orig })
	go func() { _, _ = w.WriteString("smaller stories please\n"); w.Close() }()

	out := captureStdout(t, func() {
		if err := runPlanRefine(context.Background(), "R-REF"); err != nil {
			t.Fatalf("runPlanRefine: %v", err)
		}
	})
	if !strings.Contains(out, "Refined plan") {
		t.Errorf("expected refined-plan output, got %q", out)
	}
}

func TestRunPlanRefine_LLMError(t *testing.T) {
	setupTestApp(t)
	withStubLLMErr(t, errors.New("boom"))

	r := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id": "R-LLE", "title": "t", "description": "d", "repo_path": ".",
	})
	_ = app.projStore.Project(r)

	rPipe, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = rPipe
	t.Cleanup(func() { os.Stdin = orig })
	go func() { _, _ = w.WriteString("more tests\n"); w.Close() }()

	err := runPlanRefine(context.Background(), "R-LLE")
	if err == nil || !strings.Contains(err.Error(), "re-planning failed") {
		t.Errorf("expected re-planning error, got %v", err)
	}
}

func TestPlanCmd_ReviewFlag(t *testing.T) {
	setupTestApp(t)
	cmd := newPlanCmd()
	cmd.SetArgs([]string{"--review", "NONEXISTENT"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for nonexistent requirement under --review")
	}
}

func TestPlanCmd_RefineFlag(t *testing.T) {
	setupTestApp(t)

	// Refine reads stdin; close it immediately so we hit the "no feedback" path.
	r, w, _ := os.Pipe()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	w.Close()

	cmd := newPlanCmd()
	cmd.SetArgs([]string{"--refine", "ANY"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected error from refine with empty stdin")
	}
}
