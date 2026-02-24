package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ValidateCmd() *cobra.Command {
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
