package monitor

import (
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/graph"
	"github.com/tzone85/px-dispatch/internal/planner"
)

func TestDispatcher_DispatchWave_RootNodes(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")
	dag.AddNode("s-2")
	dag.AddNode("s-3")
	dag.AddEdge("s-1", "s-3") // s-3 depends on s-1

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Title: "Setup DB", Complexity: 2},
		"s-2": {ID: "s-2", Title: "Add config", Complexity: 3},
		"s-3": {ID: "s-3", Title: "Add API", Complexity: 5},
	}
	completed := map[string]bool{}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, err := d.DispatchWave(dag, completed, "req-1", stories, 1)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	// s-1 and s-2 should be dispatched (no deps), s-3 should wait
	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}
}

func TestDispatcher_DispatchWave_AfterCompletion(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")
	dag.AddNode("s-2")
	dag.AddEdge("s-1", "s-2")

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Complexity: 2},
		"s-2": {ID: "s-2", Complexity: 5},
	}
	completed := map[string]bool{"s-1": true}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, _ := d.DispatchWave(dag, completed, "req-1", stories, 2)
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].StoryID != "s-2" {
		t.Errorf("expected s-2, got %s", assignments[0].StoryID)
	}
}

func TestDispatcher_RoleAssignment(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")
	dag.AddNode("s-2")

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Complexity: 2}, // junior
		"s-2": {ID: "s-2", Complexity: 8}, // senior
	}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, _ := d.DispatchWave(dag, map[string]bool{}, "req-1", stories, 1)

	roleMap := make(map[string]string)
	for _, a := range assignments {
		roleMap[a.StoryID] = string(a.Role)
	}
	if roleMap["s-1"] != "junior" {
		t.Errorf("s-1 (complexity 2) should be junior, got %s", roleMap["s-1"])
	}
	if roleMap["s-2"] != "senior" {
		t.Errorf("s-2 (complexity 8) should be senior, got %s", roleMap["s-2"])
	}
}

func TestDispatcher_NoneReady(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-0")
	dag.AddNode("s-1")
	dag.AddNode("s-2")
	dag.AddEdge("s-0", "s-1")
	dag.AddEdge("s-0", "s-2")

	stories := map[string]planner.PlannedStory{
		"s-0": {ID: "s-0", Complexity: 2},
		"s-1": {ID: "s-1", Complexity: 2},
		"s-2": {ID: "s-2", Complexity: 3},
	}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)
	assignments, _ := d.DispatchWave(dag, map[string]bool{}, "req-1", stories, 1)

	// Only s-0 should be ready (it has no deps)
	if len(assignments) != 1 || assignments[0].StoryID != "s-0" {
		t.Errorf("expected only s-0 ready, got %v", assignments)
	}
}

func TestDispatcher_EmptyDAG(t *testing.T) {
	dag := graph.NewDAG()
	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, err := d.DispatchWave(dag, map[string]bool{}, "req-1", nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments for empty DAG, got %d", len(assignments))
	}
}

func TestDispatcher_AllCompleted(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")
	dag.AddNode("s-2")

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Complexity: 2},
		"s-2": {ID: "s-2", Complexity: 3},
	}
	completed := map[string]bool{"s-1": true, "s-2": true}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, err := d.DispatchWave(dag, completed, "req-1", stories, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments when all completed, got %d", len(assignments))
	}
}

func TestDispatcher_IntermediateRoleAssignment(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Complexity: 5}, // intermediate (between junior max 3 and intermediate max 5)
	}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, _ := d.DispatchWave(dag, map[string]bool{}, "req-1", stories, 1)
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].Role != "intermediate" {
		t.Errorf("s-1 (complexity 5) should be intermediate, got %s", assignments[0].Role)
	}
}

func TestDispatcher_BranchAndSessionNaming(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Complexity: 2},
	}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, _ := d.DispatchWave(dag, map[string]bool{}, "req-1", stories, 1)
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}

	a := assignments[0]
	if a.Branch != "px/s-1" {
		t.Errorf("expected branch px/s-1, got %s", a.Branch)
	}
	if a.SessionName != "px-s-1" {
		t.Errorf("expected session name px-s-1, got %s", a.SessionName)
	}
	if a.Wave != 1 {
		t.Errorf("expected wave 1, got %d", a.Wave)
	}
	if a.AgentID == "" {
		t.Error("expected non-empty agent ID")
	}
}

func TestDispatcher_SkipsMissingStory(t *testing.T) {
	dag := graph.NewDAG()
	dag.AddNode("s-1")
	dag.AddNode("s-2") // s-2 is in DAG but not in stories map

	stories := map[string]planner.PlannedStory{
		"s-1": {ID: "s-1", Complexity: 2},
	}

	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	d := NewDispatcher(cfg)

	assignments, err := d.DispatchWave(dag, map[string]bool{}, "req-1", stories, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only s-1 should be dispatched; s-2 is in DAG but not in stories
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].StoryID != "s-1" {
		t.Errorf("expected s-1, got %s", assignments[0].StoryID)
	}
}
