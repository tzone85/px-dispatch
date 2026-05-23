// Package monitor provides wave-based dispatching and execution of stories
// within the px-dispatch multi-agent development system. The Dispatcher
// determines which stories are ready based on DAG dependencies and assigns
// them to agent roles. The Executor creates isolated worktrees and spawns
// runtime sessions.
package monitor

import (
	"fmt"
	"sort"

	"github.com/oklog/ulid/v2"
	"github.com/tzone85/px-dispatch/internal/agent"
	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/graph"
	"github.com/tzone85/px-dispatch/internal/planner"
)

// Assignment represents a story dispatched to an agent with its role,
// branch, and session metadata.
type Assignment struct {
	ReqID       string
	StoryID     string
	AgentID     string
	Role        agent.Role
	Branch      string
	SessionName string
	Wave        int
}

// Dispatcher determines which stories are ready and assigns them to roles
// based on complexity thresholds from the routing configuration.
type Dispatcher struct {
	routing config.RoutingConfig
}

// NewDispatcher creates a Dispatcher with the given routing configuration.
func NewDispatcher(routing config.RoutingConfig) *Dispatcher {
	return &Dispatcher{routing: routing}
}

// DispatchWave finds stories whose dependencies are all completed,
// assigns roles based on complexity, and returns assignments.
// Returns nil (not an error) when no stories are ready.
func (d *Dispatcher) DispatchWave(
	dag *graph.DAG,
	completed map[string]bool,
	reqID string,
	stories map[string]planner.PlannedStory,
	waveNumber int,
) ([]Assignment, error) {
	ready := graph.ReadyNodes(dag, completed)
	if len(ready) == 0 {
		return nil, nil
	}

	sort.Strings(ready) // deterministic ordering

	assignments := make([]Assignment, 0, len(ready))
	for _, storyID := range ready {
		story, ok := stories[storyID]
		if !ok {
			continue
		}

		role := agent.RoleForComplexity(story.Complexity, d.routing)
		agentID := generateULID()
		branch := fmt.Sprintf("px/%s", storyID)
		sessionName := fmt.Sprintf("px-%s", storyID)

		assignments = append(assignments, Assignment{
			ReqID:       reqID,
			StoryID:     storyID,
			AgentID:     agentID,
			Role:        role,
			Branch:      branch,
			SessionName: sessionName,
			Wave:        waveNumber,
		})
	}

	return assignments, nil
}

// generateULID produces a new ULID string for agent identification.
func generateULID() string {
	return ulid.Make().String()
}
