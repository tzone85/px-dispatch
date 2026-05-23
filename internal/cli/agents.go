package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List agent status",
		RunE: func(cmd *cobra.Command, args []string) error {
			agents, err := app.projStore.ListAgents(state.AgentFilter{})
			if err != nil {
				return fmt.Errorf("list agents: %w", err)
			}

			if len(agents) == 0 {
				fmt.Println("No agents found.")
				return nil
			}

			fmt.Printf("%-14s %-12s %-14s %-10s %-14s %s\n",
				"ID", "ROLE", "MODEL", "STATUS", "STORY", "SESSION")

			for _, a := range agents {
				id := a.ID
				if len(id) > 12 {
					id = id[:12] + ".."
				}

				story := a.CurrentStoryID
				if story == "" {
					story = "-"
				}
				if len(story) > 12 {
					story = story[:12] + ".."
				}

				session := a.SessionName
				if session == "" {
					session = "-"
				}

				fmt.Printf("%-14s %-12s %-14s %-10s %-14s %s\n",
					id, a.Type, a.Model, a.Status, story, session)
			}

			return nil
		},
	}
}
