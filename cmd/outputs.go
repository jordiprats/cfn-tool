package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OutputsCmd() *cobra.Command {
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
