package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DescribeCmd() *cobra.Command {
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
