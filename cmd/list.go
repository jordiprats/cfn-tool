package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	filterAll        bool
	filterComplete   bool
	filterDeleted    bool
	filterInProgress bool
	nameFilter       string
	descContains     string
	descNotContains  string
	namesOnly        bool
)

func ListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [name-filter]",
		Short: "List CloudFormation stacks",
		Long: `List CloudFormation stacks. By default shows active and in-progress stacks.

A name filter can be provided as a positional argument or via --name.`,
		Args: cobra.MaximumNArgs(1),
		Run:  runList,
	}

	cmd.Flags().BoolVarP(&filterAll, "all", "A", false, "Show all stacks (overrides other status filters)")
	cmd.Flags().BoolVarP(&filterComplete, "complete", "c", false, "Filter complete stacks (*_COMPLETE statuses)")
	cmd.Flags().BoolVarP(&filterDeleted, "deleted", "d", false, "Filter deleted stacks (DELETE_* statuses)")
	cmd.Flags().BoolVarP(&filterInProgress, "in-progress", "i", false, "Filter in-progress stacks (*_IN_PROGRESS statuses)")
	cmd.Flags().StringVarP(&nameFilter, "name", "n", "", "Filter stacks whose name contains this string (case-insensitive)")
	cmd.Flags().StringVar(&descContains, "desc", "", "Filter stacks whose description contains this string (case-insensitive)")
	cmd.Flags().StringVar(&descNotContains, "no-desc", "", "Exclude stacks whose description contains this string (case-insensitive)")
	cmd.Flags().BoolVarP(&namesOnly, "names-only", "1", false, "Print only stack names, one per line")

	return cmd
}

func runList(cmd *cobra.Command, args []string) {
	// Positional arg is a shorthand for --name
	if len(args) > 0 && nameFilter == "" {
		nameFilter = args[0]
	} else if len(args) > 0 && nameFilter != "" {
		fatalf("Error: name filter specified both as argument and --name flag\n")
	}

	ctx := context.Background()
	client := mustClient(ctx)

	statusFilters := buildStatusFilters(filterAll, filterComplete, filterDeleted, filterInProgress)
	stacks, err := listStacks(ctx, client, statusFilters, nameFilter, descContains, descNotContains)
	if err != nil {
		fatalf("failed to list stacks: %v\n", err)
	}

	if namesOnly {
		for _, s := range stacks {
			if s.StackName != nil {
				fmt.Println(*s.StackName)
			}
		}
		return
	}

	if len(stacks) == 0 {
		fmt.Fprintf(os.Stderr, "No stacks found\n")
		os.Exit(1)
	}

	printStacks(noHeaders, stacks)
}
