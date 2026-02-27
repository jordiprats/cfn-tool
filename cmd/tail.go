package cmd

import (
	"context"
	"errors"
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

	cmd.Flags().IntVarP(&interval, "interval", "s", 5, "Polling interval in seconds")

	return cmd
}

func runTail(stackName string, interval time.Duration) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := mustClient(ctx)

	// Seed: remember the timestamp of the most recent event so we only show new ones.
	var since time.Time
	var initialEvent *types.StackEvent
	seenEventIDs := make(map[string]struct{})
	{
		events, err := listEvents(ctx, client, stackName, 1)
		if err != nil {
			fatalf("failed to get initial events: %v\n", err)
		}
		if len(events) > 0 && events[0].Timestamp != nil {
			initialEvent = &events[0]
			since = *events[0].Timestamp
			if id := getValue(events[0].EventId); id != "" {
				seenEventIDs[id] = struct{}{}
			}
		}
	}

	fmt.Printf("Tailing events for stack %q (Ctrl-C to stop)...\n\n", stackName)
	if !noHeaders {
		fmt.Printf("%-22s %-40s %-45s %-30s %s\n", "TIMESTAMP", "LOGICAL ID", "TYPE", "STATUS", "REASON")
		fmt.Printf("%-22s %-40s %-45s %-30s %s\n",
			"──────────────────────", "────────────────────────────────────────",
			"─────────────────────────────────────────────", "──────────────────────────────", "──────")
	}

	if initialEvent != nil {
		ts := initialEvent.Timestamp.Format("2006-01-02 15:04:05")
		fmt.Printf("%-22s %-40s %-45s %-30s %s\n",
			ts,
			truncate(getValue(initialEvent.LogicalResourceId), 40),
			truncate(getValue(initialEvent.ResourceType), 45),
			truncate(string(initialEvent.ResourceStatus), 30),
			getValue(initialEvent.ResourceStatusReason),
		)
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
				if ctx.Err() != nil || errors.Is(err, context.Canceled) {
					continue
				}
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}

			// Events are newest-first; collect those newer than `since`.
			// Include equal-timestamp events when their EventId hasn't been seen yet.
			var newEvents []types.StackEvent
			for _, e := range events {
				if e.Timestamp == nil {
					continue
				}

				if e.Timestamp.After(since) {
					newEvents = append(newEvents, e)
					continue
				}

				if e.Timestamp.Equal(since) {
					if id := getValue(e.EventId); id != "" {
						if _, exists := seenEventIDs[id]; !exists {
							newEvents = append(newEvents, e)
						}
					}
				}
			}

			for i := len(newEvents) - 1; i >= 0; i-- {
				e := newEvents[i]
				if id := getValue(e.EventId); id != "" {
					seenEventIDs[id] = struct{}{}
				}
				ts := ""
				if e.Timestamp != nil {
					ts = e.Timestamp.Format("2006-01-02 15:04:05")
					if e.Timestamp.After(since) {
						since = *e.Timestamp
					}
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
