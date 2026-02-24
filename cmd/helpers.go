package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/printers"
)

var (
	region    string
	noHeaders bool
)

// SetGlobalFlags sets the global flags that are used across commands
func SetGlobalFlags(r string, nh bool) {
	region = r
	noHeaders = nh
}

func mustClient(ctx context.Context) *cloudformation.Client {
	cfg, err := config.LoadDefaultConfig(ctx, func(opts *config.LoadOptions) error {
		if region != "" {
			opts.Region = region
		}
		return nil
	})
	if err != nil {
		fatalf("failed to load AWS config: %v\n", err)
	}
	return cloudformation.NewFromConfig(cfg)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func listStacks(ctx context.Context, client *cloudformation.Client, statusFilters []types.StackStatus, nameFilter, descContains, descNotContains string) ([]types.StackSummary, error) {
	var all []types.StackSummary

	input := &cloudformation.ListStacksInput{}
	if len(statusFilters) > 0 {
		input.StackStatusFilter = statusFilters
	}

	paginator := cloudformation.NewListStacksPaginator(client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, stack := range output.StackSummaries {
			if nameFilter != "" && (stack.StackName == nil || !strings.Contains(strings.ToLower(*stack.StackName), strings.ToLower(nameFilter))) {
				continue
			}
			desc := strings.ToLower(getValue(stack.TemplateDescription))
			if descContains != "" && !strings.Contains(desc, strings.ToLower(descContains)) {
				continue
			}
			if descNotContains != "" && strings.Contains(desc, strings.ToLower(descNotContains)) {
				continue
			}
			all = append(all, stack)
		}
	}
	return all, nil
}

func listEvents(ctx context.Context, client *cloudformation.Client, stackName string, limit int) ([]types.StackEvent, error) {
	var all []types.StackEvent

	paginator := cloudformation.NewDescribeStackEventsPaginator(client, &cloudformation.DescribeStackEventsInput{
		StackName: &stackName,
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, output.StackEvents...)
		if limit > 0 && len(all) >= limit {
			return all[:limit], nil
		}
	}
	return all, nil
}

func buildStatusFilters(all, complete, deleted, inProgress bool) []types.StackStatus {
	// --all returns nil which means the AWS API default (everything except DELETE_COMPLETE)
	if all {
		return nil
	}

	// No specific flags: default to active + in-progress (most useful day-to-day view)
	if !complete && !deleted && !inProgress {
		return []types.StackStatus{
			types.StackStatusCreateComplete,
			types.StackStatusUpdateComplete,
			types.StackStatusRollbackComplete,
			types.StackStatusUpdateRollbackComplete,
			types.StackStatusImportComplete,
			types.StackStatusImportRollbackComplete,
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
		}
	}

	var filters []types.StackStatus

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

func makeTable(columns []string) *v1.Table {
	table := &v1.Table{}
	for _, c := range columns {
		table.ColumnDefinitions = append(table.ColumnDefinitions, v1.TableColumnDefinition{
			Name: c, Type: "string",
		})
	}
	return table
}

func mustPrint(table *v1.Table) {
	printer := printers.NewTablePrinter(printers.PrintOptions{NoHeaders: noHeaders})
	if err := printer.PrintObj(table, os.Stdout); err != nil {
		fatalf("error printing table: %v\n", err)
	}
}

func printStacks(noHdrs bool, stacks []types.StackSummary) {
	table := makeTable([]string{"NAME", "STATUS", "CREATED", "DESCRIPTION"})
	for _, stack := range stacks {
		ts := ""
		if stack.CreationTime != nil {
			ts = stack.CreationTime.Format("2006-01-02 15:04:05")
		}
		table.Rows = append(table.Rows, v1.TableRow{
			Cells: []interface{}{
				getValue(stack.StackName),
				string(stack.StackStatus),
				ts,
				getValue(stack.TemplateDescription),
			},
		})
	}
	printer := printers.NewTablePrinter(printers.PrintOptions{NoHeaders: noHdrs})
	if err := printer.PrintObj(table, os.Stdout); err != nil {
		fatalf("error printing table: %v\n", err)
	}
}

func printEvents(noHdrs bool, events []types.StackEvent) {
	table := makeTable([]string{"TIMESTAMP", "LOGICAL ID", "TYPE", "STATUS", "REASON"})
	for _, e := range events {
		ts := ""
		if e.Timestamp != nil {
			ts = e.Timestamp.Format("2006-01-02 15:04:05")
		}
		table.Rows = append(table.Rows, v1.TableRow{
			Cells: []interface{}{
				ts,
				getValue(e.LogicalResourceId),
				getValue(e.ResourceType),
				string(e.ResourceStatus),
				getValue(e.ResourceStatusReason),
			},
		})
	}
	printer := printers.NewTablePrinter(printers.PrintOptions{NoHeaders: noHdrs})
	if err := printer.PrintObj(table, os.Stdout); err != nil {
		fatalf("error printing table: %v\n", err)
	}
}

func getValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
