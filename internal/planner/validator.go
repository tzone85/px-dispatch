package planner

import (
	"fmt"

	"github.com/tzone85/px-dispatch/internal/graph"
)

// Validate checks a list of planned stories for quality issues.
// It returns a slice of human-readable issue descriptions. An empty
// slice means the plan is valid.
func Validate(stories []PlannedStory, cfg PlannerConfig) []string {
	var issues []string

	if len(stories) == 0 {
		return []string{"plan contains no stories"}
	}

	// Check story count limit.
	if cfg.MaxStoriesPerRequirement > 0 && len(stories) > cfg.MaxStoriesPerRequirement {
		issues = append(issues, fmt.Sprintf(
			"too many stories: %d exceeds maximum of %d",
			len(stories), cfg.MaxStoriesPerRequirement,
		))
	}

	// Build ID set for dependency validation and duplicate detection.
	idSet := make(map[string]bool, len(stories))
	for _, s := range stories {
		if s.ID == "" {
			continue
		}
		if idSet[s.ID] {
			issues = append(issues, fmt.Sprintf("duplicate story ID: %q", s.ID))
		}
		idSet[s.ID] = true
	}

	// Validate each story's required fields and constraints.
	for _, s := range stories {
		issues = append(issues, validateStoryFields(s, cfg)...)
		issues = append(issues, validateStoryDependencies(s, idSet)...)
	}

	// Check file ownership overlap.
	if cfg.EnforceFileOwnership {
		issues = append(issues, validateFileOwnership(stories)...)
	}

	// Check for cyclic dependencies using the graph package.
	issues = append(issues, validateNoCycles(stories)...)

	return issues
}

// validateStoryFields checks that a single story has all required fields
// and that its values are within acceptable bounds.
func validateStoryFields(s PlannedStory, cfg PlannerConfig) []string {
	var issues []string

	if s.ID == "" {
		issues = append(issues, "story has missing ID")
	}
	if s.Title == "" {
		issues = append(issues, fmt.Sprintf("story %q has missing title", s.ID))
	}
	if s.Description == "" {
		issues = append(issues, fmt.Sprintf("story %q has missing description", s.ID))
	}
	if s.AcceptanceCriteria == "" {
		issues = append(issues, fmt.Sprintf("story %q has missing acceptance criteria", s.ID))
	}
	if s.Complexity < 1 {
		issues = append(issues, fmt.Sprintf("story %q has invalid complexity %d (must be >= 1)", s.ID, s.Complexity))
	}
	if cfg.MaxStoryComplexity > 0 && s.Complexity > cfg.MaxStoryComplexity {
		issues = append(issues, fmt.Sprintf(
			"story %q has complexity %d exceeding maximum of %d",
			s.ID, s.Complexity, cfg.MaxStoryComplexity,
		))
	}

	return issues
}

// validateStoryDependencies checks that each dependency references a known story.
func validateStoryDependencies(s PlannedStory, idSet map[string]bool) []string {
	var issues []string

	for _, dep := range s.DependsOn {
		if !idSet[dep] {
			issues = append(issues, fmt.Sprintf(
				"story %q depends on unknown story %q",
				s.ID, dep,
			))
		}
	}

	return issues
}

// validateFileOwnership checks that no file is owned by more than one story.
func validateFileOwnership(stories []PlannedStory) []string {
	var issues []string

	fileOwners := make(map[string]string) // file -> first owner story ID
	for _, s := range stories {
		for _, f := range s.OwnedFiles {
			if owner, exists := fileOwners[f]; exists {
				issues = append(issues, fmt.Sprintf(
					"file %q is owned by both story %q and story %q",
					f, owner, s.ID,
				))
			} else {
				fileOwners[f] = s.ID
			}
		}
	}

	return issues
}

// validateNoCycles builds a DAG from story dependencies and checks for cycles.
func validateNoCycles(stories []PlannedStory) []string {
	dag := graph.NewDAG()

	for _, s := range stories {
		if s.ID == "" {
			continue
		}
		dag.AddNode(s.ID)
	}

	for _, s := range stories {
		if s.ID == "" {
			continue
		}
		for _, dep := range s.DependsOn {
			// AddEdge(from, to) means "to depends on from".
			// Here dep must complete before s, so dep -> s.
			dag.AddEdge(dep, s.ID)
		}
	}

	if dag.HasCycle() {
		return []string{"dependency graph contains a cycle"}
	}

	return nil
}
