package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/planner"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newPlanCmd() *cobra.Command {
	var reviewReqID string
	var refineReqID string

	cmd := &cobra.Command{
		Use:   "plan [requirement-file]",
		Short: "Plan: decompose a requirement into stories",
		Long:  "Reads a requirement, uses AI to decompose into atomic stories with dependencies.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reviewReqID != "" {
				return runPlanReview(reviewReqID)
			}
			if refineReqID != "" {
				return runPlanRefine(cmd.Context(), refineReqID)
			}
			if len(args) == 0 {
				return fmt.Errorf("provide a requirement file or use --review/--refine")
			}
			return runPlan(cmd.Context(), args[0])
		},
	}

	cmd.Flags().StringVar(&reviewReqID, "review", "", "review plan for requirement ID")
	cmd.Flags().StringVar(&refineReqID, "refine", "", "refine plan for requirement ID with feedback from stdin")
	return cmd
}

func runPlan(ctx context.Context, file string) error {
	reqText, err := readRequirement(file)
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	techStack := git.ScanTechStack(cwd)
	techInfo := planner.FormatTechStack(techStack)

	client := buildLLMClient()

	p := planner.NewPlanner(client, planner.PlannerConfig{
		MaxStoryComplexity:       app.config.Planning.MaxStoryComplexity,
		MaxStoriesPerRequirement: app.config.Planning.MaxStoriesPerRequirement,
		EnforceFileOwnership:     app.config.Planning.EnforceFileOwnership,
	})

	stories, err := p.Plan(ctx, reqText, techInfo)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	reqID := generateID()

	// Make story IDs globally unique by prefixing with a short req identifier.
	// The LLM generates generic IDs like "s-1", "s-2" which would collide across requirements.
	stories = scopeStoriesForRequirement(reqID, stories)

	reqEvt := state.NewEvent(state.EventReqSubmitted, "user", "", map[string]any{
		"id":          reqID,
		"title":       truncateForTitle(reqText, 80),
		"description": reqText,
		"repo_path":   cwd,
	})
	if err := app.eventStore.Append(reqEvt); err != nil {
		return fmt.Errorf("append req submitted event: %w", err)
	}
	app.projector.Send(reqEvt)

	for _, s := range stories {
		storyEvt := state.NewEvent(state.EventStoryCreated, "planner", s.ID, map[string]any{
			"id":                  s.ID,
			"req_id":              reqID,
			"title":               s.Title,
			"description":         s.Description,
			"acceptance_criteria": s.AcceptanceCriteria,
			"complexity":          s.Complexity,
			"owned_files":         s.OwnedFiles,
			"wave_hint":           s.WaveHint,
			"depends_on":          s.DependsOn,
		})
		if err := app.eventStore.Append(storyEvt); err != nil {
			return fmt.Errorf("append story created event for %s: %w", s.ID, err)
		}
		app.projector.Send(storyEvt)
	}

	plannedEvt := state.NewEvent(state.EventReqPlanned, "planner", "", map[string]any{
		"id": reqID,
	})
	if err := app.eventStore.Append(plannedEvt); err != nil {
		return fmt.Errorf("append req planned event: %w", err)
	}
	app.projector.Send(plannedEvt)

	fmt.Printf("\nRequirement: %s\n", reqID)
	fmt.Printf("Stories: %d\n\n", len(stories))
	printStoryTable(stories)
	fmt.Printf("\nRun 'px plan --review %s' to inspect, or 'px status %s' to view.\n", reqID, reqID)

	return nil
}

func runPlanReview(reqID string) error {
	req, err := app.projStore.GetRequirement(reqID)
	if err != nil {
		return fmt.Errorf("get requirement: %w", err)
	}

	stories, err := app.projStore.ListStories(state.StoryFilter{ReqID: reqID})
	if err != nil {
		return fmt.Errorf("list stories: %w", err)
	}

	if len(stories) == 0 {
		return fmt.Errorf("no stories found for requirement %s", reqID)
	}

	deps, err := app.projStore.ListStoryDeps(reqID)
	if err != nil {
		return fmt.Errorf("list story deps: %w", err)
	}

	// Build a map of storyID -> []dependsOnID for display.
	depMap := buildDepMap(deps)

	fmt.Printf("Requirement: %s\n", req.ID)
	fmt.Printf("Title: %s\n", req.Title)
	fmt.Printf("Status: %s\n", req.Status)
	fmt.Printf("Stories: %d\n\n", len(stories))

	fmt.Printf("%-14s %-30s %4s %-12s %-10s %s\n",
		"ID", "TITLE", "C", "STATUS", "WAVE", "DEPENDS ON")
	fmt.Println(strings.Repeat("-", 90))

	for _, s := range stories {
		depsOnStr := "-"
		if ds, ok := depMap[s.ID]; ok && len(ds) > 0 {
			depsOnStr = strings.Join(ds, ", ")
		}

		title := s.Title
		if len(title) > 28 {
			title = title[:28] + ".."
		}

		id := s.ID
		if len(id) > 12 {
			id = id[:12] + ".."
		}

		fmt.Printf("%-14s %-30s %4d %-12s %-10s %s\n",
			id, title, s.Complexity, s.Status, s.WaveHint, depsOnStr)
	}

	return nil
}

func runPlanRefine(ctx context.Context, reqID string) error {
	fmt.Println("Enter feedback for re-planning (Ctrl+D when done):")

	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read feedback: %w", err)
	}

	feedback := strings.Join(lines, "\n")
	if strings.TrimSpace(feedback) == "" {
		return fmt.Errorf("no feedback provided")
	}

	req, err := app.projStore.GetRequirement(reqID)
	if err != nil {
		return fmt.Errorf("get requirement: %w", err)
	}

	client := buildLLMClient()
	p := planner.NewPlanner(client, planner.PlannerConfig{
		MaxStoryComplexity:       app.config.Planning.MaxStoryComplexity,
		MaxStoriesPerRequirement: app.config.Planning.MaxStoriesPerRequirement,
		EnforceFileOwnership:     app.config.Planning.EnforceFileOwnership,
	})

	combined := req.Description + "\n\nFeedback:\n" + feedback
	stories, err := p.Plan(ctx, combined, "")
	if err != nil {
		return fmt.Errorf("re-planning failed: %w", err)
	}
	stories = scopeStoriesForRequirement(reqID, stories)

	if err := app.projStore.ArchiveStoriesByReq(reqID); err != nil {
		return fmt.Errorf("archive old stories: %w", err)
	}

	for _, s := range stories {
		storyEvt := state.NewEvent(state.EventStoryCreated, "planner", s.ID, map[string]any{
			"id":                  s.ID,
			"req_id":              reqID,
			"title":               s.Title,
			"description":         s.Description,
			"acceptance_criteria": s.AcceptanceCriteria,
			"complexity":          s.Complexity,
			"owned_files":         s.OwnedFiles,
			"wave_hint":           s.WaveHint,
			"depends_on":          s.DependsOn,
		})
		if err := app.eventStore.Append(storyEvt); err != nil {
			return fmt.Errorf("append story created event for %s: %w", s.ID, err)
		}
		app.projector.Send(storyEvt)
	}

	resumeEvt := state.NewEvent(state.EventReqPlanned, "planner", "", map[string]any{
		"id": reqID,
	})
	if err := app.eventStore.Append(resumeEvt); err != nil {
		return fmt.Errorf("append req planned event: %w", err)
	}
	app.projector.Send(resumeEvt)

	fmt.Printf("Refined plan: %d stories\n", len(stories))
	printStoryTable(stories)

	return nil
}

// readRequirement reads requirement text from a file path, or from stdin if path is "-".
func readRequirement(file string) (string, error) {
	if file == "-" {
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		text := strings.Join(lines, "\n")
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("no requirement text provided on stdin")
		}
		return text, nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read requirement file %q: %w", file, err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("requirement file %q is empty", file)
	}

	return text, nil
}

// printStoryTable prints a formatted summary table of planned stories.
func printStoryTable(stories []planner.PlannedStory) {
	fmt.Printf("%-8s %-35s %4s %-10s %s\n", "ID", "TITLE", "C", "WAVE", "DEPENDS ON")
	fmt.Println(strings.Repeat("-", 80))

	for _, s := range stories {
		title := s.Title
		if len(title) > 33 {
			title = title[:33] + ".."
		}

		id := s.ID
		if len(id) > 6 {
			id = id[:6] + ".."
		}

		depsStr := "-"
		if len(s.DependsOn) > 0 {
			depsStr = strings.Join(s.DependsOn, ", ")
		}

		fmt.Printf("%-8s %-35s %4d %-10s %s\n",
			id, title, s.Complexity, s.WaveHint, depsStr)
	}
}

// generateID returns a new ULID string.
func generateID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// truncateForTitle truncates a string to at most max characters, trimming at a word boundary
// when possible. Newlines are collapsed to spaces.
func truncateForTitle(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	if len(s) <= max {
		return s
	}

	truncated := s[:max]
	// Try to cut at a word boundary.
	if idx := strings.LastIndex(truncated, " "); idx > 0 {
		truncated = truncated[:idx]
	}
	return truncated + "..."
}

// buildDepMap converts a flat list of StoryDep edges into a map of storyID -> []dependsOnID.
func buildDepMap(deps []state.StoryDep) map[string][]string {
	m := make(map[string][]string)
	for _, d := range deps {
		m[d.StoryID] = append(m[d.StoryID], d.DependsOnID)
	}
	return m
}

func scopeStoriesForRequirement(reqID string, stories []planner.PlannedStory) []planner.PlannedStory {
	prefix := reqID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	return scopeStoryIDs(stories, prefix)
}

// scopeStoryIDs prefixes all story IDs and their dependency references with
// a unique prefix to prevent ID collisions across requirements.
// e.g., "s-1" becomes "01KM5R-s-1"
func scopeStoryIDs(stories []planner.PlannedStory, prefix string) []planner.PlannedStory {
	// Build old→new ID mapping
	idMap := make(map[string]string, len(stories))
	for _, s := range stories {
		idMap[s.ID] = prefix + "-" + s.ID
	}

	// Create new stories with scoped IDs and updated dependency references
	scoped := make([]planner.PlannedStory, len(stories))
	for i, s := range stories {
		newDeps := make([]string, len(s.DependsOn))
		for j, dep := range s.DependsOn {
			if mapped, ok := idMap[dep]; ok {
				newDeps[j] = mapped
			} else {
				newDeps[j] = prefix + "-" + dep
			}
		}
		scoped[i] = planner.PlannedStory{
			ID:                 idMap[s.ID],
			Title:              s.Title,
			Description:        s.Description,
			AcceptanceCriteria: s.AcceptanceCriteria,
			Complexity:         s.Complexity,
			OwnedFiles:         s.OwnedFiles,
			WaveHint:           s.WaveHint,
			DependsOn:          newDeps,
		}
	}
	return scoped
}
