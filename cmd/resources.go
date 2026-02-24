package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ResourcesCmd() *cobra.Command {
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
