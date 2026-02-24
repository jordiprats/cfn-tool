package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
)

func TailCmd() *cobra.Command {
	var interval int

	cmd := &cobra.Command{
		Use:   "tail <stack-name>",
		Short: "Stream stack events in real time (Ctrl-C to stop)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runTail(args[0], time.Duration(interval)*time.Second)
		},
	}

	cmd.Flags().IntVarP(&interval, "interval", "i", 5, "Polling interval in seconds")

	return cmd
}

func runTail(stackName string, interval time.Duration) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := mustClient(ctx)

	// Seed: remember the timestamp of the most recent event so we only show new ones.
	var since time.Time
	{
		events, err := listEvents(ctx, client, stackName, 1)
		if err != nil {
			fatalf("failed to get initial events: %v\n", err)
		}
		if len(events) > 0 && events[0].Timestamp != nil {
			since = *events[0].Timestamp
		}
	}

	fmt.Printf("Tailing events for stack %q (Ctrl-C to stop)...\n\n", stackName)
	if !noHeaders {
		fmt.Printf("%-22s %-40s %-45s %-30s %s\n", "TIMESTAMP", "LOGICAL ID", "TYPE", "STATUS", "REASON")
		fmt.Printf("%-22s %-40s %-45s %-30s %s\n",
			"──────────────────────", "────────────────────────────────────────",
			"─────────────────────────────────────────────", "──────────────────────────────", "──────")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopped.")
			return
		case <-ticker.C:
			events, err := listEvents(ctx, client, stackName, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}

			// Events are newest-first; collect those newer than `since`, then print oldest-first.
			var newEvents []types.StackEvent
			for _, e := range events {
				if e.Timestamp != nil && e.Timestamp.After(since) {
					newEvents = append(newEvents, e)
				}
			}

			for i := len(newEvents) - 1; i >= 0; i-- {
				e := newEvents[i]
				ts := ""
				if e.Timestamp != nil {
					ts = e.Timestamp.Format("2006-01-02 15:04:05")
					since = *e.Timestamp
				}
				fmt.Printf("%-22s %-40s %-45s %-30s %s\n",
					ts,
					truncate(getValue(e.LogicalResourceId), 40),
					truncate(getValue(e.ResourceType), 45),
					truncate(string(e.ResourceStatus), 30),
					getValue(e.ResourceStatusReason),
				)
			}
		}
	}
}
