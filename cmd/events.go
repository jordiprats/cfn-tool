package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func EventsCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "events <stack-name>",
		Short: "List events for a CloudFormation stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runEvents(args[0], limit)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "Maximum number of events to show (0 = all)")

	return cmd
}

func runEvents(stackName string, limit int) {
	ctx := context.Background()
	client := mustClient(ctx)

	events, err := listEvents(ctx, client, stackName, limit)
	if err != nil {
		fatalf("failed to list events for stack %q: %v\n", stackName, err)
	}

	if len(events) == 0 {
		fmt.Println("No events found")
		return
	}

	printEvents(noHeaders, events)
}
