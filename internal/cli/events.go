package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/state"
)

func newEventsCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "events",
		Short: "List recent events",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := app.eventStore.List(state.EventFilter{Limit: limit})
			if err != nil {
				return fmt.Errorf("list events: %w", err)
			}

			for _, evt := range events {
				ts := evt.Timestamp
				if len(ts) > 19 {
					ts = ts[:19]
				}
				fmt.Printf("[%s] %s agent=%s story=%s\n",
					ts, evt.Type, evt.AgentID, evt.StoryID)
			}

			if len(events) == 0 {
				fmt.Println("No events found.")
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "maximum events to show")
	return cmd
}
