package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ParametersCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "parameters <stack-name>",
		Aliases: []string{"params", "param"},
		Short:   "Show parameters for a CloudFormation stack",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runParameters(args[0])
		},
	}
}

func runParameters(stackName string) {
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

	params := output.Stacks[0].Parameters
	if len(params) == 0 {
		fmt.Println("No parameters found")
		return
	}

	table := makeTable([]string{"KEY", "VALUE", "RESOLVED VALUE"})
	for _, p := range params {
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
