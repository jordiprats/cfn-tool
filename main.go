package main

import (
	"cfn/cmd"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	region    string
	noHeaders bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cfn",
		Short: "AWS CloudFormation CLI tool",
		Long:  "Inspect and manage AWS CloudFormation stacks",
		PersistentPreRun: func(command *cobra.Command, args []string) {
			cmd.SetGlobalFlags(region, noHeaders)
		},
	}

	rootCmd.PersistentFlags().StringVarP(&region, "region", "r", "", "AWS region (uses default if not specified)")
	rootCmd.PersistentFlags().BoolVar(&noHeaders, "no-headers", false, "Don't print headers")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.AddCommand(
		cmd.ListCmd(),
		cmd.EventsCmd(),
		cmd.DescribeCmd(),
		cmd.OutputsCmd(),
		cmd.ResourcesCmd(),
		cmd.DriftCmd(),
		cmd.TailCmd(),
		cmd.TemplateCmd(),
		cmd.ValidateCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
