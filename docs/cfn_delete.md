## cfn delete

Delete a CloudFormation stack

### Synopsis

Delete a CloudFormation stack.

By default this command asks for confirmation and waits until deletion completes.

Examples:
  # Delete a stack (with confirmation)
  cfn delete my-stack

  # Skip prompt in scripts
  cfn delete my-stack --yes

  # Start deletion and return immediately
  cfn delete my-stack --wait=false

  # Keep specific resources during stack deletion
  cfn delete my-stack --retain-resource MyBucket --retain-resource MyLogGroup

  # Delete resources via Cloud Control API before deleting the stack
  cfn delete my-stack --cloudcontrol-delete

  # Preview what --cloudcontrol-delete would do without making changes
  cfn delete my-stack --cloudcontrol-delete --dry-run

```
cfn delete <stack-name> [flags]
```

### Options

```
      --cloudcontrol-delete           Delete resources via Cloud Control API before deleting the stack
      --dry-run                       Show what --cloudcontrol-delete would do without making changes
  -h, --help                          help for delete
      --retain-resource stringArray   Logical resource ID to retain during deletion (repeatable)
  -w, --wait                          Wait for stack deletion to complete (default true)
  -y, --yes                           Skip interactive confirmation
```

### Options inherited from parent commands

```
      --no-headers      Don't print headers
  -r, --region string   AWS region (uses default if not specified)
```

### SEE ALSO

* [cfn](cfn.md)	 - AWS CloudFormation CLI tool

