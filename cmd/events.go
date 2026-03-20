package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
)

func EventsCmd() *cobra.Command {
	var limit int
	var failed bool

	cmd := &cobra.Command{
		Use:   "events <stack-name>",
		Short: "List events for a CloudFormation stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runEvents(args[0], limit, failed)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "Maximum number of events to show (0 = all)")
	cmd.Flags().BoolVarP(&failed, "failed", "f", false, "Show only failure events (root cause analysis)")

	return cmd
}

func runEvents(stackName string, limit int, failed bool) {
	ctx := context.Background()
	client := mustClient(ctx)

	events, err := listEvents(ctx, client, stackName, limit)
	if err != nil {
		fatalf("failed to list events for stack %q: %v\n", stackName, err)
	}

	if failed {
		events = filterFailedEvents(events)
	}

	if len(events) == 0 {
		if failed {
			fmt.Println("No failure events found")
		} else {
			fmt.Println("No events found")
		}
		return
	}

	printEvents(noHeaders, events)
}

func filterFailedEvents(events []types.StackEvent) []types.StackEvent {
	var filtered []types.StackEvent
	for _, e := range events {
		status := string(e.ResourceStatus)
		if strings.HasSuffix(status, "_FAILED") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
