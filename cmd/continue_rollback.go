package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
)

func ContinueRollbackCmd() *cobra.Command {
	var skip []string
	var roleARN string
	var yes bool
	var wait bool

	cmd := &cobra.Command{
		Use:     "continue-rollback <stack-name>",
		Aliases: []string{"cr"},
		Short:   "Continue update rollback for a stack in UPDATE_ROLLBACK_FAILED state",
		Long: `Continue rolling back a stack that is stuck in UPDATE_ROLLBACK_FAILED state.

Lists resources in UPDATE_FAILED state (eligible for skipping) before proceeding.
Use --skip to skip specific resources that cannot be rolled back.

Examples:
  # Continue rollback (shows failed resources and prompts)
  cfn continue-rollback my-stack

  # Skip a problematic resource
  cfn continue-rollback my-stack --skip MyBucket

  # Skip multiple resources
  cfn continue-rollback my-stack --skip MyBucket --skip MyTable

  # Skip confirmation
  cfn continue-rollback my-stack --skip MyBucket --yes

  # Use a specific IAM role
  cfn continue-rollback my-stack --role-arn arn:aws:iam::123456789012:role/my-role`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runContinueRollback(args[0], skip, roleARN, yes, wait)
		},
	}

	cmd.Flags().StringArrayVar(&skip, "skip", []string{}, "Logical resource ID to skip during rollback (repeatable)")
	cmd.Flags().StringVar(&roleARN, "role-arn", "", "IAM role ARN for CloudFormation to assume")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip interactive confirmation")
	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for rollback to complete")

	return cmd
}

func runContinueRollback(stackName string, skip []string, roleARN string, yes bool, wait bool) {
	ctx := context.Background()
	client := mustClient(ctx)

	// List resources in UPDATE_FAILED state
	var failedResources []types.StackResourceSummary
	paginator := cloudformation.NewListStackResourcesPaginator(client, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			fatalf("failed to list resources for stack %q: %v\n", stackName, err)
		}
		for _, r := range output.StackResourceSummaries {
			if r.ResourceStatus == types.ResourceStatusUpdateFailed {
				failedResources = append(failedResources, r)
			}
		}
	}

	if len(failedResources) > 0 {
		fmt.Fprintf(os.Stderr, "Resources in UPDATE_FAILED state (eligible for --skip):\n")
		for _, r := range failedResources {
			reason := getValue(r.ResourceStatusReason)
			if reason != "" {
				fmt.Fprintf(os.Stderr, "  %s (%s) — %s\n", getValue(r.LogicalResourceId), getValue(r.ResourceType), reason)
			} else {
				fmt.Fprintf(os.Stderr, "  %s (%s)\n", getValue(r.LogicalResourceId), getValue(r.ResourceType))
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	if len(skip) > 0 {
		fmt.Fprintf(os.Stderr, "Will skip: %s\n", strings.Join(skip, ", "))
	}

	if !yes {
		fmt.Fprintf(os.Stderr, "Continue update rollback for stack %q? Type 'yes' to confirm: ", stackName)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fatalf("failed to read input: %v\n", err)
		}
		if !strings.EqualFold(strings.TrimSpace(input), "yes") {
			fatalf("aborted\n")
		}
	}

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
		fatalf("failed to continue update rollback for stack %q: %v\n", stackName, err)
	}

	fmt.Printf("Continue update rollback started for stack %q\n", stackName)

	if !wait {
		return
	}

	fmt.Print("Waiting")
	for {
		time.Sleep(3 * time.Second)
		fmt.Print(".")

		out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
		if err != nil {
			fatalf("\nfailed to check rollback status for %q: %v\n", stackName, err)
		}

		if len(out.Stacks) == 0 {
			fatalf("\nstack %q not found\n", stackName)
		}

		stack := out.Stacks[0]
		switch stack.StackStatus {
		case types.StackStatusUpdateRollbackComplete:
			fmt.Printf("\nStack %q rollback complete\n", stackName)
			return
		case types.StackStatusUpdateRollbackFailed:
			fatalf("\nrollback failed again for stack %q: %s\n", stackName, getValue(stack.StackStatusReason))
		}
	}
}
