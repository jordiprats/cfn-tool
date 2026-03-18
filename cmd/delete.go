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

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/spf13/cobra"
)

func DeleteCmd() *cobra.Command {
	var yes bool
	var wait bool
	var retainResources []string
	var cloudcontrolDelete bool
	var dryRun bool

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
  cfn delete my-stack --retain-resource MyBucket --retain-resource MyLogGroup

  # Delete resources via Cloud Control API before deleting the stack
  cfn delete my-stack --cloudcontrol-delete

  # Preview what --cloudcontrol-delete would do without making changes
  cfn delete my-stack --cloudcontrol-delete --dry-run`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runDelete(args[0], yes, wait, retainResources, cloudcontrolDelete, dryRun)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip interactive confirmation")
	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for stack deletion to complete")
	cmd.Flags().StringArrayVar(&retainResources, "retain-resource", []string{}, "Logical resource ID to retain during deletion (repeatable)")
	cmd.Flags().BoolVar(&cloudcontrolDelete, "cloudcontrol-delete", false, "Delete resources via Cloud Control API before deleting the stack")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what --cloudcontrol-delete would do without making changes")

	return cmd
}

func runDelete(stackName string, yes bool, wait bool, retainResources []string, cloudcontrolDelete bool, dryRun bool) {
	if dryRun && !cloudcontrolDelete {
		fatalf("--dry-run requires --cloudcontrol-delete\n")
	}

	if !dryRun && !yes {
		if cloudcontrolDelete {
			if !confirmDelete(fmt.Sprintf("%s (resources will be deleted via Cloud Control first)", stackName)) {
				fatalf("aborted\n")
			}
		} else {
			if !confirmDelete(stackName) {
				fatalf("aborted\n")
			}
		}
	}

	ctx := context.Background()
	cfnClient := mustClient(ctx)

	if cloudcontrolDelete {
		preDeleteResources(ctx, cfnClient, stackName, dryRun)
		if dryRun {
			return
		}
	}

	input := &cloudformation.DeleteStackInput{StackName: &stackName}
	if len(retainResources) > 0 {
		input.RetainResources = retainResources
	}

	if _, err := cfnClient.DeleteStack(ctx, input); err != nil {
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

		out, err := cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: &stackName})
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

func preDeleteResources(ctx context.Context, cfnClient *cloudformation.Client, stackName string, dryRun bool) {
	// List all resources in the stack
	var resources []types.StackResourceSummary
	paginator := cloudformation.NewListStackResourcesPaginator(cfnClient, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			fatalf("failed to list resources for stack %q: %v\n", stackName, err)
		}
		resources = append(resources, output.StackResourceSummaries...)
	}

	// Filter: skip nested stacks and resources without a physical ID
	type target struct {
		logicalID  string
		physicalID string
		typeName   string
	}
	var targets []target
	for _, r := range resources {
		resType := getValue(r.ResourceType)
		if resType == "AWS::CloudFormation::Stack" {
			continue
		}
		pid := getValue(r.PhysicalResourceId)
		if pid == "" {
			continue
		}
		// Only target AWS resources (type starts with "AWS::")
		if !strings.HasPrefix(resType, "AWS::") {
			continue
		}
		targets = append(targets, target{
			logicalID:  getValue(r.LogicalResourceId),
			physicalID: pid,
			typeName:   resType,
		})
	}

	if len(targets) == 0 {
		fmt.Println("No resources to delete via Cloud Control")
		return
	}

	if dryRun {
		fmt.Printf("Dry run: would delete %d resource(s) via Cloud Control API before stack deletion:\n", len(targets))
		for _, t := range targets {
			fmt.Printf("  cloudcontrol delete-resource --type-name %s --identifier %s  (logical: %s)\n", t.typeName, t.physicalID, t.logicalID)
		}
		fmt.Printf("  Then: cloudformation delete-stack --stack-name %s\n", stackName)
		return
	}

	ccClient := mustCloudControlClient(ctx)

	fmt.Printf("Deleting %d resource(s) via Cloud Control API...\n", len(targets))

	type inflight struct {
		target
		requestToken string
	}
	var pending []inflight

	for _, t := range targets {
		fmt.Printf("  Deleting %s %s (%s)...", t.typeName, t.physicalID, t.logicalID)
		out, err := ccClient.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
			TypeName:   &t.typeName,
			Identifier: &t.physicalID,
		})
		if err != nil {
			fmt.Printf(" error: %v\n", err)
			continue
		}
		token := getValue(out.ProgressEvent.RequestToken)
		if token == "" {
			fmt.Printf(" started (no token)\n")
			continue
		}
		fmt.Printf(" started (token: %s)\n", token)
		pending = append(pending, inflight{target: t, requestToken: token})
	}

	// Poll all inflight deletions until they complete
	for len(pending) > 0 {
		time.Sleep(3 * time.Second)
		var still []inflight
		for _, p := range pending {
			status, err := ccClient.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
				RequestToken: &p.requestToken,
			})
			if err != nil {
				fmt.Printf("  %s: failed to poll status: %v\n", p.logicalID, err)
				continue
			}
			opStatus := status.ProgressEvent.OperationStatus
			switch opStatus {
			case cctypes.OperationStatusSuccess:
				fmt.Printf("  %s: deleted\n", p.logicalID)
			case cctypes.OperationStatusFailed:
				msg := getValue(status.ProgressEvent.StatusMessage)
				fmt.Printf("  %s: delete failed: %s\n", p.logicalID, msg)
			case cctypes.OperationStatusCancelComplete:
				fmt.Printf("  %s: cancelled\n", p.logicalID)
			default:
				still = append(still, p)
			}
		}
		pending = still
	}

	fmt.Println("Cloud Control resource deletion complete")
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
