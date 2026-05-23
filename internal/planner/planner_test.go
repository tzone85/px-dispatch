package planner

import (
	"context"
	"testing"

	"github.com/tzone85/px-dispatch/internal/llm"
)

// mockStoryResponse returns a valid JSON response for story decomposition.
func mockStoryResponse() string {
	return `{
		"stories": [
			{
				"id": "s-1",
				"title": "Setup database schema",
				"description": "Create the initial database tables",
				"acceptance_criteria": "Tables exist and migrations run",
				"complexity": 3,
				"owned_files": ["migrations/001.sql", "internal/db/schema.go"],
				"wave_hint": "sequential",
				"depends_on": []
			},
			{
				"id": "s-2",
				"title": "Implement user API",
				"description": "REST endpoints for user CRUD",
				"acceptance_criteria": "GET/POST/PUT/DELETE users work",
				"complexity": 5,
				"owned_files": ["internal/api/users.go", "internal/api/users_test.go"],
				"wave_hint": "parallel",
				"depends_on": ["s-1"]
			}
		]
	}`
}

func TestPlanner_DecomposesRequirement(t *testing.T) {
	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: mockStoryResponse(), Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{})

	stories, err := p.Plan(context.Background(), "Build a user management system", "")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("expected 2 stories, got %d", len(stories))
	}
	if stories[0].Title != "Setup database schema" {
		t.Errorf("expected first story title, got %q", stories[0].Title)
	}
	if len(stories[1].DependsOn) != 1 || stories[1].DependsOn[0] != "s-1" {
		t.Errorf("expected s-2 depends on s-1, got %v", stories[1].DependsOn)
	}
}

func TestPlanner_HandlesInvalidJSON(t *testing.T) {
	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: "not json", Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{})

	_, err := p.Plan(context.Background(), "Build something", "")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestPlanner_RetriesAfterInvalidJSON(t *testing.T) {
	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: "I think these stories look good.", Model: "test"},
		llm.CompletionResponse{Content: mockStoryResponse(), Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{})

	stories, err := p.Plan(context.Background(), "Build something", "")
	if err != nil {
		t.Fatalf("plan after parse retry: %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("expected 2 stories after retry, got %d", len(stories))
	}
}

func TestPlanner_IncludesTechStackContext(t *testing.T) {
	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: mockStoryResponse(), Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{})

	stories, err := p.Plan(context.Background(), "Build auth", "Go, Cobra, SQLite")
	if err != nil {
		t.Fatalf("plan with tech stack: %v", err)
	}
	if len(stories) == 0 {
		t.Error("expected stories")
	}
}

func TestPlanner_ParsesRawArray(t *testing.T) {
	rawArray := `[
		{
			"id": "s-1",
			"title": "Single story",
			"description": "A single task",
			"acceptance_criteria": "It works",
			"complexity": 2,
			"owned_files": ["main.go"],
			"wave_hint": "sequential",
			"depends_on": []
		}
	]`
	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: rawArray, Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{})

	stories, err := p.Plan(context.Background(), "Simple task", "")
	if err != nil {
		t.Fatalf("parse raw array: %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}
}

func TestPlanner_TwoPassReplanOnValidationIssues(t *testing.T) {
	// First response has a story with missing title (will fail validation).
	// Second response is valid.
	badResponse := `{
		"stories": [
			{
				"id": "s-1",
				"title": "",
				"description": "Missing title",
				"acceptance_criteria": "ok",
				"complexity": 3,
				"owned_files": ["a.go"],
				"depends_on": []
			}
		]
	}`
	goodResponse := mockStoryResponse()

	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: badResponse, Model: "test"},
		llm.CompletionResponse{Content: goodResponse, Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{
		MaxStoryComplexity:       8,
		MaxStoriesPerRequirement: 15,
		EnforceFileOwnership:     true,
	})

	stories, err := p.Plan(context.Background(), "Build something", "")
	if err != nil {
		t.Fatalf("two-pass plan: %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("expected 2 stories from second pass, got %d", len(stories))
	}
}

func TestPlanner_TwoPassStillFailsAfterSecondAttempt(t *testing.T) {
	// Both responses have validation issues.
	badResponse := `{
		"stories": [
			{
				"id": "s-1",
				"title": "",
				"description": "Missing title",
				"acceptance_criteria": "ok",
				"complexity": 3,
				"owned_files": ["a.go"],
				"depends_on": []
			}
		]
	}`

	client := llm.NewReplayClient(
		llm.CompletionResponse{Content: badResponse, Model: "test"},
		llm.CompletionResponse{Content: badResponse, Model: "test"},
	)
	p := NewPlanner(client, PlannerConfig{
		MaxStoryComplexity:       8,
		MaxStoriesPerRequirement: 15,
		EnforceFileOwnership:     true,
	})

	_, err := p.Plan(context.Background(), "Build something", "")
	if err == nil {
		t.Fatal("expected error after two failed validation passes")
	}
}
