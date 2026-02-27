package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	filterAll        bool
	filterComplete   bool
	filterDeleted    bool
	filterInProgress bool
	ignoreCase       bool
	nameFilter       string
	descContains     string
	descNotContains  string
	namesOnly        bool
	resourceType     string
	resourceName     string
	properties       []string
)

func ListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [name-filter]",
		Short: "List CloudFormation stacks",
		Long: `List CloudFormation stacks. By default shows active and in-progress stacks.

A name filter can be provided as a positional argument.

When resource filters (--type, --resource-name, --property) are specified, 
performs a deep search of stack templates and shows matching resources.

Examples:
  # List all stacks (table view)
  cfn list
  
  # Filter stacks by name
  cfn list my-stack
  
  # Search for stacks containing a specific resource type
  cfn list --type AWS::ServiceCatalog::CloudFormationProvisionedProduct
  
  # Search for stacks containing a resource with a specific logical ID
  cfn list --resource-name MyBucket
  
  # Combine filters
  cfn list my-stack --type AWS::S3::Bucket --property BucketName=foo`,
		Args: cobra.MaximumNArgs(1),
		Run:  runList,
	}

	cmd.Flags().BoolVarP(&filterAll, "all", "A", false, "Show all stacks (overrides other status filters)")
	cmd.Flags().BoolVarP(&filterComplete, "complete", "C", false, "Filter complete stacks (*_COMPLETE statuses)")
	cmd.Flags().BoolVarP(&filterDeleted, "deleted", "D", false, "Filter deleted stacks (DELETE_* statuses)")
	cmd.Flags().BoolVarP(&filterInProgress, "in-progress", "P", false, "Filter in-progress stacks (*_IN_PROGRESS statuses)")
	cmd.Flags().BoolVarP(&ignoreCase, "ignore-case", "i", false, "Use case-insensitive matching for text filters")
	cmd.Flags().StringVar(&descContains, "desc", "", "Filter stacks whose description contains this string")
	cmd.Flags().StringVar(&descNotContains, "no-desc", "", "Exclude stacks whose description contains this string")
	cmd.Flags().BoolVarP(&namesOnly, "names-only", "1", false, "Print only stack names, one per line")
	cmd.Flags().StringVarP(&resourceType, "type", "t", "", "Search for resource type (e.g., AWS::S3::Bucket)")
	cmd.Flags().StringVarP(&resourceName, "resource-name", "n", "", "Search for resource logical ID")
	cmd.Flags().StringArrayVarP(&properties, "property", "p", []string{}, "Search for resource property (format: key=value or nested.key=value)")

	return cmd
}

func runList(cmd *cobra.Command, args []string) {
	// Positional arg is the stack name filter
	if len(args) > 0 {
		nameFilter = args[0]
	}

	ctx := context.Background()
	client := mustClient(ctx)

	// Check if resource search is requested
	isResourceSearch := resourceType != "" || resourceName != "" || len(properties) > 0

	// For resource search, default to all stacks unless user specifies status filters
	statusFilters := buildStatusFilters(filterAll, filterComplete, filterDeleted, filterInProgress)
	if isResourceSearch && !filterAll && !filterComplete && !filterDeleted && !filterInProgress {
		// No status filters specified and doing resource search - search all stacks (including DELETE_COMPLETE)
		statusFilters = nil
	}

	stacks, err := listStacks(ctx, client, statusFilters, nameFilter, descContains, descNotContains, ignoreCase)
	if err != nil {
		fatalf("failed to list stacks: %v\n", err)
	}

	if isResourceSearch {
		runResourceSearch(ctx, client, stacks, namesOnly)
		return
	}

	if namesOnly {
		for _, s := range stacks {
			if s.StackName != nil {
				fmt.Println(*s.StackName)
			}
		}
		return
	}

	if len(stacks) == 0 {
		fmt.Fprintf(os.Stderr, "No stacks found\n")
		os.Exit(1)
	}

	printStacks(noHeaders, stacks)
}

func runResourceSearch(ctx context.Context, client *cloudformation.Client, stacks []types.StackSummary, namesOnly bool) {
	// Parse property filters
	propertyFilters := make(map[string]string)
	for _, prop := range properties {
		parts := strings.SplitN(prop, "=", 2)
		if len(parts) != 2 {
			fatalf("invalid property format %q, expected key=value\n", prop)
		}
		propertyFilters[parts[0]] = parts[1]
	}

	if len(stacks) == 0 {
		fmt.Fprintf(os.Stderr, "No stacks to search\n")
		os.Exit(1)
	}

	// Build search message (only show if not in names-only mode)
	if !namesOnly {
		searchMsg := fmt.Sprintf("Searching %d stacks for", len(stacks))
		if resourceName != "" && resourceType != "" {
			searchMsg += fmt.Sprintf(" resource %q of type %q", resourceName, resourceType)
		} else if resourceName != "" {
			searchMsg += fmt.Sprintf(" resource %q", resourceName)
		} else if resourceType != "" {
			searchMsg += fmt.Sprintf(" resources of type %q", resourceType)
		} else {
			searchMsg += " resources"
		}
		if len(propertyFilters) > 0 {
			searchMsg += " with properties:"
			for key, value := range propertyFilters {
				searchMsg += fmt.Sprintf(" %s=%q", key, value)
			}
		}
		searchMsg += "..."
		fmt.Fprintf(os.Stderr, "%s\n", searchMsg)
	}

	// Find stacks with matching resources
	var matchingStackSummaries []types.StackSummary
	for _, stack := range stacks {
		if stack.StackName == nil {
			continue
		}

		hasMatch, err := searchStackTemplate(ctx, client, *stack.StackName, resourceType, resourceName, propertyFilters, ignoreCase)
		if err != nil {
			// Skip stacks we can't access
			continue
		}

		if hasMatch {
			matchingStackSummaries = append(matchingStackSummaries, stack)
		}
	}

	// Clear the "Searching..." line (only if we showed it)
	if !namesOnly {
		fmt.Fprintf(os.Stderr, "\033[1A\033[2K")
	}

	if len(matchingStackSummaries) == 0 {
		if !namesOnly {
			fmt.Printf("No stacks found containing")
			if resourceName != "" && resourceType != "" {
				fmt.Printf(" resource %q of type %q", resourceName, resourceType)
			} else if resourceName != "" {
				fmt.Printf(" resource %q", resourceName)
			} else if resourceType != "" {
				fmt.Printf(" resources of type %q", resourceType)
			}
			if len(propertyFilters) > 0 {
				fmt.Printf(" with properties:")
				for key, value := range propertyFilters {
					fmt.Printf(" %s=%q", key, value)
				}
			}
			fmt.Println()
		}
		os.Exit(1)
	}

	// Print results using the same format as regular list
	if namesOnly {
		for _, stack := range matchingStackSummaries {
			if stack.StackName != nil {
				fmt.Println(*stack.StackName)
			}
		}
	} else {
		printStacks(noHeaders, matchingStackSummaries)
	}
}

func searchStackTemplate(ctx context.Context, client *cloudformation.Client, stackName, resType, resName string, propertyFilters map[string]string, ignoreCase bool) (bool, error) {
	// Get template
	output, err := client.GetTemplate(ctx, &cloudformation.GetTemplateInput{
		StackName:     &stackName,
		TemplateStage: types.TemplateStageOriginal,
	})
	if err != nil {
		return false, err
	}

	body := getValue(output.TemplateBody)
	if body == "" {
		return false, fmt.Errorf("empty template")
	}

	// Parse template (try JSON first, then YAML)
	var template map[string]interface{}
	if err := json.Unmarshal([]byte(body), &template); err != nil {
		// Try YAML
		if err := yaml.Unmarshal([]byte(body), &template); err != nil {
			return false, fmt.Errorf("failed to parse template: %v", err)
		}
	}

	// Search for resources
	resources, ok := template["Resources"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	for logicalID, resourceData := range resources {
		// Check resource name first (cheapest check) if specified
		if resName != "" && !containsWithCase(logicalID, resName, ignoreCase) {
			continue
		}

		resourceMap, ok := resourceData.(map[string]interface{})
		if !ok {
			continue
		}

		// Check resource type second if specified
		if resType != "" {
			currentType, ok := resourceMap["Type"].(string)
			if !ok || !equalsWithCase(currentType, resType, ignoreCase) {
				continue
			}
		}

		// Check if properties match
		if len(propertyFilters) > 0 {
			properties, ok := resourceMap["Properties"].(map[string]interface{})
			if !ok {
				continue
			}

			matched, _ := checkProperties(properties, propertyFilters, ignoreCase)
			if !matched {
				continue
			}
		}

		// Found a match
		return true, nil
	}

	return false, nil
}

func checkProperties(properties map[string]interface{}, filters map[string]string, ignoreCase bool) (bool, map[string]interface{}) {
	matchedProps := make(map[string]interface{})

	for key, expectedValue := range filters {
		// Handle nested properties (e.g., "Versioning.Status")
		value := getNestedProperty(properties, key, ignoreCase)
		if value == nil {
			return false, nil
		}

		// Convert value to string for comparison
		valueStr := fmt.Sprintf("%v", value)
		if !equalsWithCase(valueStr, expectedValue, ignoreCase) {
			return false, nil
		}

		matchedProps[key] = value
	}

	return true, matchedProps
}

func getNestedProperty(properties map[string]interface{}, path string, ignoreCase bool) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = properties

	for _, part := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}

		if ignoreCase {
			found := false
			for key, value := range currentMap {
				if strings.EqualFold(key, part) {
					current = value
					found = true
					break
				}
			}
			if !found {
				return nil
			}
			continue
		}

		val, exists := currentMap[part]
		if !exists {
			return nil
		}

		current = val
	}

	return current
}
