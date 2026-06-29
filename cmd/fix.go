package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
)

func FixCmd() *cobra.Command {
	var roleARN string
	var drift bool

	cmd := &cobra.Command{
		Use:   "fix <stack-name>",
		Short: "Fix a stuck UPDATE_ROLLBACK_FAILED stack",
		Long: `Automates the continue-update-rollback process for stacks stuck in
UPDATE_ROLLBACK_FAILED. If the stack contains Service Catalog provisioned products
with stuck nested stacks, fixes them bottom-up automatically.

Use --drift to run drift detection after the fix completes, showing what's out of
sync from skipped resources.

Examples:
  cfn fix orch-b-default-nodegroup
  cfn fix orch-b-default-nodegroup --drift
  cfn fix orch-b-default-nodegroup --role-arn arn:aws:iam::123456789012:role/my-role`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runFix(args[0], roleARN, drift)
		},
	}

	cmd.Flags().StringVar(&roleARN, "role-arn", "", "IAM role ARN for CloudFormation to assume")
	cmd.Flags().BoolVar(&drift, "drift", false, "Run drift detection after fix completes")

	return cmd
}

func runFix(stackName string, roleARN string, drift bool) {
	ctx := context.Background()
	client := mustClient(ctx)

	// Verify stack is in UPDATE_ROLLBACK_FAILED
	descOut, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
	if err != nil {
		fatalf("failed to describe stack %q: %v\n", stackName, err)
	}
	if len(descOut.Stacks) == 0 {
		fatalf("stack %q not found\n", stackName)
	}
	parentStack := descOut.Stacks[0]
	if parentStack.StackStatus != types.StackStatusUpdateRollbackFailed {
		fatalf("stack %q is in %s, expected UPDATE_ROLLBACK_FAILED\n", stackName, parentStack.StackStatus)
	}

	// Find SC provisioned products in UPDATE_FAILED state
	var scResources []types.StackResourceSummary
	paginator := cloudformation.NewListStackResourcesPaginator(client, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			fatalf("failed to list resources for stack %q: %v\n", stackName, err)
		}
		for _, r := range output.StackResourceSummaries {
			if getValue(r.ResourceType) == "AWS::ServiceCatalog::CloudFormationProvisionedProduct" &&
				r.ResourceStatus == types.ResourceStatusUpdateFailed {
				scResources = append(scResources, r)
			}
		}
	}

	if len(scResources) == 0 {
		fixContinueRollback(ctx, client, stackName, roleARN)
		if drift {
			runDrift(stackName, true)
		}
		printStackStatus(ctx, client, stackName)
		return
	}

	// For each SC product, find and fix the underlying stack
	for _, scRes := range scResources {
		ppID := getValue(scRes.PhysicalResourceId)
		logicalID := getValue(scRes.LogicalResourceId)
		fmt.Fprintf(os.Stderr, "Found stuck SC product: %s (provisioned product: %s)\n", logicalID, ppID)

		if ppID == "" {
			fmt.Fprintf(os.Stderr, "  No physical resource ID — skipping\n")
			continue
		}

		// Find the underlying SC stack by searching for stacks containing the pp-id
		innerStacks, err := listStacks(ctx, client, nil, ppID, "", "", false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to find inner stack for %s: %v\n", ppID, err)
			continue
		}

		var innerStack *types.StackSummary
		for i, s := range innerStacks {
			if s.StackStatus == types.StackStatusUpdateRollbackFailed {
				innerStack = &innerStacks[i]
				break
			}
		}

		if innerStack == nil {
			fmt.Fprintf(os.Stderr, "  No inner stack in UPDATE_ROLLBACK_FAILED found for %s\n", ppID)
			continue
		}

		innerName := getValue(innerStack.StackName)
		fmt.Fprintf(os.Stderr, "  Inner stack: %s (UPDATE_ROLLBACK_FAILED)\n", innerName)

		// Show failed resources in the inner stack
		showFailedResources(ctx, client, innerName)

		// Attempt continue-update-rollback on inner stack
		fmt.Fprintf(os.Stderr, "\n  Attempting continue-update-rollback on %s...\n", innerName)
		success := attemptContinueRollback(ctx, client, innerName, nil, roleARN)
		if !success {
			// Retry skipping all UPDATE_FAILED resources
			failedIDs := getFailedResourceIDs(ctx, client, innerName)
			if len(failedIDs) == 0 {
				fatalf("\n  Inner stack %s failed to roll back and no skippable resources found.\n", innerName)
			}
			fmt.Fprintf(os.Stderr, "  Retrying, skipping: %s\n", strings.Join(failedIDs, ", "))
			success = attemptContinueRollback(ctx, client, innerName, failedIDs, roleARN)
			if !success {
				fatalf("\n  Inner stack %s failed to roll back even after skipping resources.\n", innerName)
			}
		}
		fmt.Fprintf(os.Stderr, "  Inner stack %s rolled back successfully.\n\n", innerName)

		if drift {
			fmt.Fprintf(os.Stderr, "  Running drift detection on %s...\n", innerName)
			runDrift(innerName, true)
		}
	}

	// Now fix the parent stack
	fmt.Fprintf(os.Stderr, "Attempting continue-update-rollback on parent stack %s...\n", stackName)
	fixContinueRollback(ctx, client, stackName, roleARN)

	// Fixing the parent may re-break inner stacks — fix them again if needed
	for _, scRes := range scResources {
		ppID := getValue(scRes.PhysicalResourceId)
		if ppID == "" {
			continue
		}
		innerStacks, err := listStacks(ctx, client, nil, ppID, "", "", false)
		if err != nil {
			continue
		}
		for _, s := range innerStacks {
			if s.StackStatus != types.StackStatusUpdateRollbackFailed {
				continue
			}
			innerName := getValue(s.StackName)
			fmt.Fprintf(os.Stderr, "\nInner stack %s is stuck again, fixing...\n", innerName)
			showFailedResources(ctx, client, innerName)
			fmt.Fprintf(os.Stderr, "  Attempting continue-update-rollback on %s...\n", innerName)
			success := attemptContinueRollback(ctx, client, innerName, nil, roleARN)
			if !success {
				failedIDs := getFailedResourceIDs(ctx, client, innerName)
				if len(failedIDs) == 0 {
					fmt.Fprintf(os.Stderr, "  Failed and no skippable resources found.\n")
					continue
				}
				fmt.Fprintf(os.Stderr, "  Retrying, skipping: %s\n", strings.Join(failedIDs, ", "))
				success = attemptContinueRollback(ctx, client, innerName, failedIDs, roleARN)
				if !success {
					fmt.Fprintf(os.Stderr, "  Failed even after skipping.\n")
				}
			}
			if success {
				fmt.Fprintf(os.Stderr, "  Inner stack %s rolled back successfully.\n", innerName)
			}
		}
	}

	// Show final status of all stacks
	fmt.Fprintf(os.Stderr, "\nFinal stack status:\n")
	printStackStatus(ctx, client, stackName)
	for _, scRes := range scResources {
		ppID := getValue(scRes.PhysicalResourceId)
		if ppID == "" {
			continue
		}
		innerStacks, err := listStacks(ctx, client, nil, ppID, "", "", false)
		if err != nil {
			continue
		}
		for _, s := range innerStacks {
			printStackStatus(ctx, client, getValue(s.StackName))
		}
	}
}

func fixContinueRollback(ctx context.Context, client *cloudformation.Client, stackName string, roleARN string) {
	showFailedResources(ctx, client, stackName)
	success := attemptContinueRollback(ctx, client, stackName, nil, roleARN)
	if !success {
		failedIDs := getFailedResourceIDs(ctx, client, stackName)
		if len(failedIDs) == 0 {
			fatalf("\nStack %s failed to roll back and no skippable resources found.\n", stackName)
		}
		fmt.Fprintf(os.Stderr, "  Retrying, skipping: %s\n", strings.Join(failedIDs, ", "))
		success = attemptContinueRollback(ctx, client, stackName, failedIDs, roleARN)
		if !success {
			fatalf("\nStack %s failed to roll back even after skipping resources.\n", stackName)
		}
	}
	fmt.Printf("Stack %q rollback complete.\n", stackName)
}

func attemptContinueRollback(ctx context.Context, client *cloudformation.Client, stackName string, skip []string, roleARN string) bool {
	input := &cloudformation.ContinueUpdateRollbackInput{
		StackName: &stackName,
	}
	if len(skip) > 0 {
		input.ResourcesToSkip = skip
	}
	if roleARN != "" {
		input.RoleARN = &roleARN
	}

	if _, err := client.ContinueUpdateRollback(ctx, input); err != nil {
		fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		return false
	}

	// Poll until complete or failed
	for {
		time.Sleep(3 * time.Second)
		fmt.Fprint(os.Stderr, ".")

		out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n  Failed to check status: %v\n", err)
			return false
		}
		if len(out.Stacks) == 0 {
			fmt.Fprintf(os.Stderr, "\n  Stack not found\n")
			return false
		}

		switch out.Stacks[0].StackStatus {
		case types.StackStatusUpdateRollbackComplete:
			fmt.Fprintln(os.Stderr)
			return true
		case types.StackStatusUpdateRollbackFailed:
			fmt.Fprintln(os.Stderr)
			return false
		}
	}
}

func getFailedResourceIDs(ctx context.Context, client *cloudformation.Client, stackName string) []string {
	var ids []string
	paginator := cloudformation.NewListStackResourcesPaginator(client, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil
		}
		for _, r := range output.StackResourceSummaries {
			if r.ResourceStatus == types.ResourceStatusUpdateFailed {
				ids = append(ids, getValue(r.LogicalResourceId))
			}
		}
	}
	return ids
}

func printStackStatus(ctx context.Context, client *cloudformation.Client, stackName string) {
	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %-60s %s\n", stackName, "UNKNOWN")
		return
	}
	if len(out.Stacks) == 0 {
		fmt.Fprintf(os.Stderr, "  %-60s %s\n", stackName, "NOT FOUND")
		return
	}
	fmt.Fprintf(os.Stderr, "  %-60s %s\n", stackName, out.Stacks[0].StackStatus)
}

func showFailedResources(ctx context.Context, client *cloudformation.Client, stackName string) {
	paginator := cloudformation.NewListStackResourcesPaginator(client, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})
	var found bool
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return
		}
		for _, r := range output.StackResourceSummaries {
			if r.ResourceStatus == types.ResourceStatusUpdateFailed {
				if !found {
					fmt.Fprintf(os.Stderr, "  Resources in UPDATE_FAILED state:\n")
					found = true
				}
				reason := getValue(r.ResourceStatusReason)
				if reason != "" {
					fmt.Fprintf(os.Stderr, "    %s (%s) — %s\n", getValue(r.LogicalResourceId), getValue(r.ResourceType), reason)
				} else {
					fmt.Fprintf(os.Stderr, "    %s (%s)\n", getValue(r.LogicalResourceId), getValue(r.ResourceType))
				}
			}
		}
	}
}
