package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/printers"
)

var (
	filterAll        bool
	filterComplete   bool
	filterDeleted    bool
	filterInProgress bool
	nameFilter       string
	region           string
	noHeaders        bool
	namesOnly        bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cfn-list",
		Args:  cobra.MaximumNArgs(1),
		Short: "List AWS CloudFormation stacks",
		Long:  "List AWS CloudFormation stacks with various filters. By default shows active stacks.",
		Run:   run,
	}

	rootCmd.Flags().BoolVarP(&filterAll, "all", "A", false, "Show all stacks (overrides other status filters)")
	rootCmd.Flags().BoolVarP(&filterComplete, "complete", "c", false, "Filter complete stacks (all *_COMPLETE statuses)")
	rootCmd.Flags().BoolVarP(&filterDeleted, "deleted", "d", false, "Filter deleted stacks (DELETE_* statuses)")
	rootCmd.Flags().BoolVarP(&filterInProgress, "in-progress", "i", false, "Filter in-progress stacks (all *_IN_PROGRESS statuses)")
	rootCmd.Flags().StringVarP(&nameFilter, "name", "n", "", "Filter stacks containing this string in name")
	rootCmd.Flags().StringVarP(&region, "region", "r", "", "AWS region (uses default if not specified)")
	rootCmd.Flags().BoolVar(&noHeaders, "no-headers", false, "Don't print headers")
	rootCmd.Flags().BoolVarP(&namesOnly, "names-only", "1", false, "Print only stack names, one per line")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	if len(args) > 0 && nameFilter == "" {
		nameFilter = args[0]
	} else if len(args) > 0 && nameFilter != "" {
		fmt.Fprintf(os.Stderr, "Error: too many arguments\n")
		cmd.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, func(opts *config.LoadOptions) error {
		if region != "" {
			opts.Region = region
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := cloudformation.NewFromConfig(cfg)

	// Build status filter for AWS API
	statusFilters := buildStatusFilters(filterAll, filterComplete, filterDeleted, filterInProgress)

	// Get stacks from AWS with filters
	stacks, err := listStacks(ctx, client, statusFilters, nameFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list stacks: %v\n", err)
		os.Exit(1)
	}

	// Display results
	if len(stacks) == 0 {
		fmt.Println("No stacks found")
		return
	}

	if namesOnly {
		printStackNames(stacks)
	} else {
		printStacks(noHeaders, stacks)
	}
}

func listStacks(ctx context.Context, client *cloudformation.Client, statusFilters []types.StackStatus, nameFilter string) ([]types.StackSummary, error) {
	var allStacks []types.StackSummary

	input := &cloudformation.ListStacksInput{}

	// Apply status filters if specified
	if len(statusFilters) > 0 {
		input.StackStatusFilter = statusFilters
	}

	paginator := cloudformation.NewListStacksPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		// Apply name filter if specified
		if nameFilter != "" {
			for _, stack := range output.StackSummaries {
				if stack.StackName != nil && contains(*stack.StackName, nameFilter) {
					allStacks = append(allStacks, stack)
				}
			}
		} else {
			allStacks = append(allStacks, output.StackSummaries...)
		}
	}

	return allStacks, nil
}

func buildStatusFilters(all, complete, deleted, inProgress bool) []types.StackStatus {
	var filters []types.StackStatus

	// If --all is specified, return nil to get all stacks
	if all {
		return nil
	}

	// If no specific filters are set, default to active stacks
	if !complete && !deleted && !inProgress {
		filters = append(filters,
			types.StackStatusCreateComplete,
			types.StackStatusUpdateComplete,
			types.StackStatusRollbackComplete,
			types.StackStatusUpdateRollbackComplete,
			types.StackStatusImportComplete,
			types.StackStatusImportRollbackComplete,
		)
		return filters
	}

	if complete {
		filters = append(filters,
			types.StackStatusCreateComplete,
			types.StackStatusDeleteComplete,
			types.StackStatusRollbackComplete,
			types.StackStatusUpdateComplete,
			types.StackStatusUpdateRollbackComplete,
			types.StackStatusImportComplete,
			types.StackStatusImportRollbackComplete,
		)
	}

	if deleted {
		filters = append(filters,
			types.StackStatusDeleteInProgress,
			types.StackStatusDeleteFailed,
			types.StackStatusDeleteComplete,
		)
	}

	if inProgress {
		filters = append(filters,
			types.StackStatusCreateInProgress,
			types.StackStatusDeleteInProgress,
			types.StackStatusRollbackInProgress,
			types.StackStatusUpdateInProgress,
			types.StackStatusUpdateCompleteCleanupInProgress,
			types.StackStatusUpdateRollbackInProgress,
			types.StackStatusUpdateRollbackCompleteCleanupInProgress,
			types.StackStatusReviewInProgress,
			types.StackStatusImportInProgress,
			types.StackStatusImportRollbackInProgress,
		)
	}

	return filters
}

func printStackNames(stacks []types.StackSummary) {
	for _, stack := range stacks {
		if stack.StackName != nil {
			fmt.Println(*stack.StackName)
		}
	}
}

func printStacks(noHeaders bool, stacks []types.StackSummary) {
	// Create a table printer
	printer := printers.NewTablePrinter(printers.PrintOptions{NoHeaders: noHeaders})

	// Create a Table object
	table := &v1.Table{
		ColumnDefinitions: []v1.TableColumnDefinition{
			{Name: "NAME", Type: "string"},
			{Name: "STATUS", Type: "string"},
			{Name: "CREATED", Type: "string"},
			{Name: "DESCRIPTION", Type: "string"},
		},
	}

	// Populate rows with data
	for _, stack := range stacks {
		stackName := getValue(stack.StackName)
		status := string(stack.StackStatus)
		creationTime := ""
		if stack.CreationTime != nil {
			creationTime = stack.CreationTime.Format("2006-01-02 15:04:05")
		}
		description := getValue(stack.TemplateDescription)

		table.Rows = append(table.Rows, v1.TableRow{
			Cells: []interface{}{
				stackName,
				status,
				creationTime,
				description,
			},
		})
	}

	// Print the table
	err := printer.PrintObj(table, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error printing table: %v\n", err)
		os.Exit(1)
	}
}

func getValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}