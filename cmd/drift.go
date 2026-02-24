package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DriftCmd() *cobra.Command {
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
