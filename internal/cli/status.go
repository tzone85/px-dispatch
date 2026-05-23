package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [req-id]",
		Short: "Show requirement and story status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return showRequirementDetail(args[0])
			}
			return showAllRequirements()
		},
	}
}

// showAllRequirements lists all non-archived requirements with story counts grouped by status.
func showAllRequirements() error {
	reqs, err := app.projStore.ListRequirements(state.ReqFilter{ExcludeArchived: true})
	if err != nil {
		return fmt.Errorf("list requirements: %w", err)
	}

	if len(reqs) == 0 {
		fmt.Println("No requirements found.")
		return nil
	}

	for _, req := range reqs {
		stories, err := app.projStore.ListStories(state.StoryFilter{ReqID: req.ID})
		if err != nil {
			return fmt.Errorf("list stories for req %s: %w", req.ID, err)
		}

		statusCounts := countByStatus(stories)

		fmt.Printf("[%s] %s  status=%s  stories=%d",
			req.ID, req.Title, req.Status, len(stories))

		if len(statusCounts) > 0 {
			fmt.Printf(" (")
			first := true
			for status, count := range statusCounts {
				if !first {
					fmt.Printf(", ")
				}
				fmt.Printf("%s:%d", status, count)
				first = false
			}
			fmt.Printf(")")
		}
		fmt.Println()
	}

	return nil
}

// showRequirementDetail shows one requirement with all its stories.
func showRequirementDetail(reqID string) error {
	req, err := app.projStore.GetRequirement(reqID)
	if err != nil {
		return fmt.Errorf("get requirement: %w", err)
	}

	fmt.Printf("Requirement: %s\n", req.ID)
	fmt.Printf("Title:       %s\n", req.Title)
	fmt.Printf("Status:      %s\n", req.Status)
	fmt.Printf("Repo:        %s\n", req.RepoPath)
	fmt.Printf("Created:     %s\n", req.CreatedAt)

	if req.Description != "" {
		fmt.Printf("Description: %s\n", req.Description)
	}

	stories, err := app.projStore.ListStories(state.StoryFilter{ReqID: reqID})
	if err != nil {
		return fmt.Errorf("list stories: %w", err)
	}

	fmt.Printf("\nStories (%d):\n", len(stories))
	if len(stories) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, st := range stories {
		agentInfo := ""
		if st.AgentID != "" {
			agentInfo = fmt.Sprintf("  agent=%s", st.AgentID)
		}
		fmt.Printf("  [%s] %s  status=%s  complexity=%d%s\n",
			st.ID, st.Title, st.Status, st.Complexity, agentInfo)
	}

	return nil
}

// countByStatus counts stories grouped by their status.
func countByStatus(stories []state.Story) map[string]int {
	counts := make(map[string]int)
	for _, st := range stories {
		counts[st.Status]++
	}
	return counts
}
