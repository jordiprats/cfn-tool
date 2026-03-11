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
	"github.com/aws/smithy-go"
	"github.com/spf13/cobra"
)

func DeleteCmd() *cobra.Command {
	var yes bool
	var wait bool
	var retainResources []string

	cmd := &cobra.Command{
		Use:     "delete <stack-name>",
		Aliases: []string{"rm", "del"},
		Short:   "Delete a CloudFormation stack",
		Long: `Delete a CloudFormation stack.

By default this command asks for confirmation and waits until deletion completes.

Examples:
  # Delete a stack (with confirmation)
  cfn delete my-stack

  # Skip prompt in scripts
  cfn delete my-stack --yes

  # Start deletion and return immediately
  cfn delete my-stack --wait=false

  # Keep specific resources during stack deletion
  cfn delete my-stack --retain-resource MyBucket --retain-resource MyLogGroup`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runDelete(args[0], yes, wait, retainResources)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip interactive confirmation")
	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for stack deletion to complete")
	cmd.Flags().StringArrayVar(&retainResources, "retain-resource", []string{}, "Logical resource ID to retain during deletion (repeatable)")

	return cmd
}

func runDelete(stackName string, yes bool, wait bool, retainResources []string) {
	if !yes {
		if !confirmDelete(stackName) {
			fatalf("aborted\n")
		}
	}

	ctx := context.Background()
	client := mustClient(ctx)

	input := &cloudformation.DeleteStackInput{StackName: &stackName}
	if len(retainResources) > 0 {
		input.RetainResources = retainResources
	}

	if _, err := client.DeleteStack(ctx, input); err != nil {
		fatalf("failed to delete stack %q: %v\n", stackName, err)
	}

	fmt.Printf("Deletion started for stack %q\n", stackName)

	if !wait {
		fmt.Println("Use --wait to poll for completion automatically.")
		return
	}

	fmt.Print("Waiting")
	for {
		time.Sleep(3 * time.Second)
		fmt.Print(".")

		out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
		if err != nil {
			if isStackNotFound(err) {
				fmt.Printf("\nStack %q deleted\n", stackName)
				return
			}
			fatalf("\nfailed to check deletion status for %q: %v\n", stackName, err)
		}

		if len(out.Stacks) == 0 {
			fmt.Printf("\nStack %q deleted\n", stackName)
			return
		}

		stack := out.Stacks[0]
		switch stack.StackStatus {
		case types.StackStatusDeleteComplete:
			fmt.Printf("\nStack %q deleted\n", stackName)
			return
		case types.StackStatusDeleteFailed:
			fatalf("\ndelete failed for stack %q: %s\n", stackName, getValue(stack.StackStatusReason))
		}
	}
}

func confirmDelete(stackName string) bool {
	fmt.Fprintf(os.Stderr, "Delete stack %q? Type 'yes' to confirm: ", stackName)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(input), "yes")
}

func isStackNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	if apiErr.ErrorCode() != "ValidationError" {
		return false
	}

	return strings.Contains(strings.ToLower(apiErr.ErrorMessage()), "does not exist")
}
