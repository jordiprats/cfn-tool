## cfn list

List CloudFormation stacks

### Synopsis

List CloudFormation stacks. By default shows active and in-progress stacks.

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
  cfn list my-stack --type AWS::S3::Bucket --property BucketName=foo

```
cfn list [name-filter] [flags]
```

### Options

```
  -A, --all                    Show all stacks (overrides other status filters)
  -C, --complete               Filter complete stacks (*_COMPLETE statuses)
  -D, --deleted                Filter deleted stacks (DELETE_* statuses)
      --desc string            Filter stacks whose description contains this string
  -h, --help                   help for list
  -i, --ignore-case            Use case-insensitive matching for text filters
  -P, --in-progress            Filter in-progress stacks (*_IN_PROGRESS statuses)
  -1, --names-only             Print only stack names, one per line
      --no-desc string         Exclude stacks whose description contains this string
  -p, --property stringArray   Search for resource property (format: key=value or nested.key=value)
  -n, --resource-name string   Search for resource logical ID
  -t, --type string            Search for resource type (e.g., AWS::S3::Bucket)
```

### Options inherited from parent commands

```
      --no-headers      Don't print headers
  -r, --region string   AWS region (uses default if not specified)
```

### SEE ALSO

* [cfn](cfn.md)	 - AWS CloudFormation CLI tool

