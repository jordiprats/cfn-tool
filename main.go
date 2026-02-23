package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	descContains     string
	descNotContains  string
	region           string
	noHeaders        bool
	namesOnly        bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cfn",
		Short: "AWS CloudFormation CLI tool",
		Long:  "Inspect and manage AWS CloudFormation stacks",
	}

	rootCmd.PersistentFlags().StringVarP(&region, "region", "r", "", "AWS region (uses default if not specified)")
	rootCmd.PersistentFlags().BoolVar(&noHeaders, "no-headers", false, "Don't print headers")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.AddCommand(
		listCmd(),
		eventsCmd(),
		describeCmd(),
		outputsCmd(),
		resourcesCmd(),
		driftCmd(),
		tailCmd(),
		templateCmd(),
		validateCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func listCmd() *cobra.Command {
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

// ---------------------------------------------------------------------------
// events
// ---------------------------------------------------------------------------

func eventsCmd() *cobra.Command {
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

// ---------------------------------------------------------------------------
// describe
// ---------------------------------------------------------------------------

func describeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe <stack-name>",
		Short: "Show full metadata for a CloudFormation stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runDescribe(args[0])
		},
	}
}

func runDescribe(stackName string) {
	ctx := context.Background()
	client := mustClient(ctx)

	output, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		fatalf("failed to describe stack %q: %v\n", stackName, err)
	}
	if len(output.Stacks) == 0 {
		fatalf("stack %q not found\n", stackName)
	}

	stack := output.Stacks[0]

	// Basic info
	fmt.Printf("Name:                  %s\n", getValue(stack.StackName))
	fmt.Printf("Stack ID:              %s\n", getValue(stack.StackId))
	fmt.Printf("Status:                %s\n", string(stack.StackStatus))
	if stack.StackStatusReason != nil {
		fmt.Printf("Status Reason:         %s\n", *stack.StackStatusReason)
	}
	fmt.Printf("Created:               %s\n", stack.CreationTime.Format("2006-01-02 15:04:05"))
	if stack.LastUpdatedTime != nil {
		fmt.Printf("Last Updated:          %s\n", stack.LastUpdatedTime.Format("2006-01-02 15:04:05"))
	}
	if stack.Description != nil {
		fmt.Printf("Description:           %s\n", *stack.Description)
	}
	fmt.Printf("Termination Protected: %v\n", stack.EnableTerminationProtection)
	if stack.RoleARN != nil {
		fmt.Printf("IAM Role:              %s\n", *stack.RoleARN)
	}
	if stack.DriftInformation != nil {
		fmt.Printf("Drift Status:          %s\n", string(stack.DriftInformation.StackDriftStatus))
	}

	// Parameters
	if len(stack.Parameters) > 0 {
		fmt.Println("\nParameters:")
		table := makeTable([]string{"KEY", "VALUE", "RESOLVED VALUE"})
		for _, p := range stack.Parameters {
			resolved := ""
			if p.ResolvedValue != nil {
				resolved = *p.ResolvedValue
			}
			val := getValue(p.ParameterValue)
			if aws.ToBool(p.UsePreviousValue) {
				val = "<use-previous-value>"
			}
			table.Rows = append(table.Rows, v1.TableRow{
				Cells: []interface{}{getValue(p.ParameterKey), val, resolved},
			})
		}
		mustPrint(table)
	}

	// Outputs
	if len(stack.Outputs) > 0 {
		fmt.Println("\nOutputs:")
		table := makeTable([]string{"KEY", "VALUE", "EXPORT NAME", "DESCRIPTION"})
		for _, o := range stack.Outputs {
			table.Rows = append(table.Rows, v1.TableRow{
				Cells: []interface{}{
					getValue(o.OutputKey),
					getValue(o.OutputValue),
					getValue(o.ExportName),
					getValue(o.Description),
				},
			})
		}
		mustPrint(table)
	}

	// Tags
	if len(stack.Tags) > 0 {
		fmt.Println("\nTags:")
		table := makeTable([]string{"KEY", "VALUE"})
		for _, t := range stack.Tags {
			table.Rows = append(table.Rows, v1.TableRow{
				Cells: []interface{}{getValue(t.Key), getValue(t.Value)},
			})
		}
		mustPrint(table)
	}

	// Capabilities
	if len(stack.Capabilities) > 0 {
		fmt.Print("\nCapabilities: ")
		for i, c := range stack.Capabilities {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(string(c))
		}
		fmt.Println()
	}
}

// ---------------------------------------------------------------------------
// outputs
// ---------------------------------------------------------------------------

func outputsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outputs <stack-name>",
		Short: "Show outputs for a CloudFormation stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runOutputs(args[0])
		},
	}
}

func runOutputs(stackName string) {
	ctx := context.Background()
	client := mustClient(ctx)

	output, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		fatalf("failed to describe stack %q: %v\n", stackName, err)
	}
	if len(output.Stacks) == 0 {
		fatalf("stack %q not found\n", stackName)
	}

	outputs := output.Stacks[0].Outputs
	if len(outputs) == 0 {
		fmt.Println("No outputs found")
		return
	}

	table := makeTable([]string{"KEY", "VALUE", "EXPORT NAME", "DESCRIPTION"})
	for _, o := range outputs {
		table.Rows = append(table.Rows, v1.TableRow{
			Cells: []interface{}{
				getValue(o.OutputKey),
				getValue(o.OutputValue),
				getValue(o.ExportName),
				getValue(o.Description),
			},
		})
	}
	mustPrint(table)
}

// ---------------------------------------------------------------------------
// resources
// ---------------------------------------------------------------------------

func resourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resources <stack-name>",
		Short: "List physical resources in a CloudFormation stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runResources(args[0])
		},
	}
}

func runResources(stackName string) {
	ctx := context.Background()
	client := mustClient(ctx)

	var all []types.StackResourceSummary
	paginator := cloudformation.NewListStackResourcesPaginator(client, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			fatalf("failed to list resources for stack %q: %v\n", stackName, err)
		}
		all = append(all, output.StackResourceSummaries...)
	}

	if len(all) == 0 {
		fmt.Println("No resources found")
		return
	}

	table := makeTable([]string{"LOGICAL ID", "PHYSICAL ID", "TYPE", "STATUS", "DRIFT"})
	for _, r := range all {
		drift := ""
		if r.DriftInformation != nil {
			drift = string(r.DriftInformation.StackResourceDriftStatus)
		}
		table.Rows = append(table.Rows, v1.TableRow{
			Cells: []interface{}{
				getValue(r.LogicalResourceId),
				getValue(r.PhysicalResourceId),
				getValue(r.ResourceType),
				string(r.ResourceStatus),
				drift,
			},
		})
	}
	mustPrint(table)
}

// ---------------------------------------------------------------------------
// drift
// ---------------------------------------------------------------------------

func driftCmd() *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:   "drift <stack-name>",
		Short: "Detect and show drift for a CloudFormation stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runDrift(args[0], wait)
		},
	}

	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for drift detection to complete")

	return cmd
}

func runDrift(stackName string, wait bool) {
	ctx := context.Background()
	client := mustClient(ctx)

	// Initiate detection
	initOut, err := client.DetectStackDrift(ctx, &cloudformation.DetectStackDriftInput{
		StackName: &stackName,
	})
	if err != nil {
		fatalf("failed to initiate drift detection for %q: %v\n", stackName, err)
	}

	detectionID := getValue(initOut.StackDriftDetectionId)
	fmt.Printf("Drift detection started (ID: %s)\n", detectionID)

	if !wait {
		fmt.Println("Use --wait to poll for results automatically.")
		return
	}

	// Poll until complete
	fmt.Print("Waiting")
	for {
		time.Sleep(3 * time.Second)
		fmt.Print(".")

		status, err := client.DescribeStackDriftDetectionStatus(ctx, &cloudformation.DescribeStackDriftDetectionStatusInput{
			StackDriftDetectionId: &detectionID,
		})
		if err != nil {
			fatalf("\nfailed to get drift status: %v\n", err)
		}

		switch status.DetectionStatus {
		case types.StackDriftDetectionStatusDetectionComplete:
			fmt.Println()
			printDriftResults(ctx, client, stackName, status)
			return
		case types.StackDriftDetectionStatusDetectionFailed:
			fmt.Println()
			fatalf("drift detection failed: %s\n", getValue(status.DetectionStatusReason))
		}
		// DETECTION_IN_PROGRESS — keep polling
	}
}

func printDriftResults(ctx context.Context, client *cloudformation.Client, stackName string, status *cloudformation.DescribeStackDriftDetectionStatusOutput) {
	fmt.Printf("\nStack drift status: %s\n", string(status.StackDriftStatus))
	fmt.Printf("Drifted resources:  %d\n\n",
		aws.ToInt32(status.DriftedStackResourceCount),
	)

	// List drifted resources
	var drifted []types.StackResourceDrift
	paginator := cloudformation.NewDescribeStackResourceDriftsPaginator(client, &cloudformation.DescribeStackResourceDriftsInput{
		StackName: &stackName,
		StackResourceDriftStatusFilters: []types.StackResourceDriftStatus{
			types.StackResourceDriftStatusModified,
			types.StackResourceDriftStatusDeleted,
		},
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			fatalf("failed to list drifted resources: %v\n", err)
		}
		drifted = append(drifted, output.StackResourceDrifts...)
	}

	if len(drifted) == 0 {
		fmt.Println("No drifted resources.")
		return
	}

	table := makeTable([]string{"LOGICAL ID", "TYPE", "DRIFT STATUS", "PROPERTY DIFFS"})
	for _, d := range drifted {
		diffs := fmt.Sprintf("%d properties", len(d.PropertyDifferences))
		table.Rows = append(table.Rows, v1.TableRow{
			Cells: []interface{}{
				getValue(d.LogicalResourceId),
				getValue(d.ResourceType),
				string(d.StackResourceDriftStatus),
				diffs,
			},
		})
	}
	mustPrint(table)

	// Show property-level detail
	for _, d := range drifted {
		if len(d.PropertyDifferences) == 0 {
			continue
		}
		fmt.Printf("\n%s (%s):\n", getValue(d.LogicalResourceId), getValue(d.ResourceType))
		for _, diff := range d.PropertyDifferences {
			fmt.Printf("  %-40s %s\n", getValue(diff.PropertyPath), string(diff.DifferenceType))
			fmt.Printf("    Expected: %s\n", getValue(diff.ExpectedValue))
			fmt.Printf("    Actual:   %s\n", getValue(diff.ActualValue))
		}
	}
}

// ---------------------------------------------------------------------------
// tail
// ---------------------------------------------------------------------------

func tailCmd() *cobra.Command {
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

// ---------------------------------------------------------------------------
// template
// ---------------------------------------------------------------------------

func templateCmd() *cobra.Command {
	var pretty bool

	cmd := &cobra.Command{
		Use:   "template <stack-name>",
		Short: "Fetch and print the deployed template for a stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runTemplate(args[0], pretty)
		},
	}

	cmd.Flags().BoolVarP(&pretty, "pretty", "p", false, "Pretty-print JSON templates")

	return cmd
}

func runTemplate(stackName string, pretty bool) {
	ctx := context.Background()
	client := mustClient(ctx)

	output, err := client.GetTemplate(ctx, &cloudformation.GetTemplateInput{
		StackName:     &stackName,
		TemplateStage: types.TemplateStageOriginal,
	})
	if err != nil {
		fatalf("failed to get template for stack %q: %v\n", stackName, err)
	}

	body := getValue(output.TemplateBody)

	if pretty {
		// Attempt JSON pretty-print; fall through to raw output if it's YAML.
		var raw interface{}
		if err := json.Unmarshal([]byte(body), &raw); err == nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(raw)
			return
		}
	}

	fmt.Print(body)
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <template-file>",
		Short: "Validate a CloudFormation template file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runValidate(args[0])
		},
	}
}

func runValidate(templateFile string) {
	data, err := os.ReadFile(templateFile)
	if err != nil {
		fatalf("failed to read template file %q: %v\n", templateFile, err)
	}

	ctx := context.Background()
	client := mustClient(ctx)

	body := string(data)
	output, err := client.ValidateTemplate(ctx, &cloudformation.ValidateTemplateInput{
		TemplateBody: &body,
	})
	if err != nil {
		fatalf("template validation failed: %v\n", err)
	}

	fmt.Println("Template is valid ✓")

	if output.Description != nil {
		fmt.Printf("Description: %s\n", *output.Description)
	}

	if len(output.Parameters) > 0 {
		fmt.Println("\nParameters:")
		table := makeTable([]string{"KEY", "DEFAULT VALUE", "NO ECHO", "DESCRIPTION"})
		for _, p := range output.Parameters {
			table.Rows = append(table.Rows, v1.TableRow{
				Cells: []interface{}{
					getValue(p.ParameterKey),
					getValue(p.DefaultValue),
					fmt.Sprintf("%v", aws.ToBool(p.NoEcho)),
					getValue(p.Description),
				},
			})
		}
		mustPrint(table)
	}

	if len(output.Capabilities) > 0 {
		fmt.Print("\nRequired Capabilities: ")
		for i, c := range output.Capabilities {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Print(string(c))
		}
		fmt.Println()
	}

	if output.CapabilitiesReason != nil && *output.CapabilitiesReason != "" {
		fmt.Printf("Capabilities Reason: %s\n", *output.CapabilitiesReason)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

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
