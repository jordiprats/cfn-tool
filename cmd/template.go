package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
)

func TemplateCmd() *cobra.Command {
	var pretty bool

	cmd := &cobra.Command{
		Use:   "template <stack-name>",
		Short: "Fetch and print the deployed template for a stack",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runTemplate(args[0], pretty)
		},
	}

	cmd.Flags().BoolVarP(&pretty, "pretty", "p", false, "Pretty-print JSON templates")

	return cmd
}

func runTemplate(stackName string, pretty bool) {
	ctx := context.Background()
	client := mustClient(ctx)

	output, err := client.GetTemplate(ctx, &cloudformation.GetTemplateInput{
		StackName:     &stackName,
		TemplateStage: types.TemplateStageOriginal,
	})
	if err != nil {
		fatalf("failed to get template for stack %q: %v\n", stackName, err)
	}

	body := getValue(output.TemplateBody)

	if pretty {
		// Attempt JSON pretty-print; fall through to raw output if it's YAML.
		var raw interface{}
		if err := json.Unmarshal([]byte(body), &raw); err == nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(raw)
			return
		}
	}

	fmt.Print(body)
}
