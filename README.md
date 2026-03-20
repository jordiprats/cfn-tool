# cfn

A user-friendly command-line tool for inspecting and managing AWS CloudFormation stacks.

## Features

- 📋 **List stacks** with flexible filtering options
- 🔍 **Search templates** by resource type and properties
- 📊 **View stack details** including parameters, outputs, tags, and resources
- 🗑️ **Delete stacks** safely with confirmation and optional wait
- 🔄 **Monitor events** in real-time with tail functionality
- 🔎 **Detect drift** and view detailed drift information
- ✅ **Validate templates** before deployment
- 📄 **Export templates** from live stacks

## Installation

### From Source

```bash
git clone https://github.com/yourusername/cfn-list.git
cd cfn-list
go build -o cfn
```

### Using Go Install

```bash
go install github.com/yourusername/cfn-list@latest
```

## Quick Start

```bash
# List active stacks
cfn list

# Search for specific resources
cfn list --type AWS::S3::Bucket

# Monitor a deployment
cfn tail my-stack
```

## Commands

### `cfn list` - List Stacks

List CloudFormation stacks with filtering options. [Documentation](./docs/cfn_list.md)

```bash
cfn list                          # Active and in-progress stacks
cfn list --all                    # All stacks
cfn list my-app                   # Filter by name
cfn list --complete               # Only completed stacks
cfn list --desc "production"      # Filter by description
cfn list --names-only             # Names only (pipeable)

# Search for resources in templates
cfn list --type AWS::S3::Bucket   # Search active stacks for S3 buckets
cfn list --type AWS::S3::Bucket --all  # Search all stacks
cfn list --type AWS::ServiceCatalog::CloudFormationProvisionedProduct \
  --property ProductName=IAMRole \
  --property ProvisioningArtifactName=3.0.0
```

### `cfn describe` - Stack Details

View comprehensive stack information. [Documentation](./docs/cfn_describe.md)

```bash
cfn describe my-stack             # Full details including parameters, outputs, tags
```

### `cfn delete` - Delete Stack

Delete a stack safely with confirmation and optional wait. [Documentation](./docs/cfn_delete.md)

```bash
cfn delete my-stack               # Confirm and wait for completion
cfn delete my-stack --yes         # Non-interactive (script-friendly)
cfn delete my-stack --wait=false  # Trigger delete and return immediately
```

### `cfn events` - Stack Events

List stack events. [Documentation](./docs/cfn_events.md)

```bash
cfn events my-stack               # All events
cfn events my-stack --limit 10    # Last 10 events
cfn events my-stack --failed      # Show only failure events (root cause analysis)
```

### `cfn tail` - Stream Events

Monitor stack events in real-time. [Documentation](./docs/cfn_tail.md)

```bash
cfn tail my-stack                 # Default 5-second interval
cfn tail my-stack --interval 10   # Custom interval
```

### `cfn parameters` - Stack Parameters

Show stack parameters. [Documentation](./docs/cfn_parameters.md)

```bash
cfn parameters my-stack
```

### `cfn outputs` - Stack Outputs

Show stack outputs. [Documentation](./docs/cfn_outputs.md)

```bash
cfn outputs my-stack
```

### `cfn resources` - Stack Resources

List physical resources in a stack. [Documentation](./docs/cfn_resources.md)

```bash
cfn resources my-stack
```

### `cfn drift` - Drift Detection

Detect configuration drift. [Documentation](./docs/cfn_drift.md)

```bash
cfn drift my-stack                # Detect and wait
cfn drift my-stack --wait=false   # Initiate only
```

### `cfn template` - Template Operations

Get and validate templates. [Documentation](./docs/cfn_template.md)

```bash
cfn template my-stack             # Get deployed template
cfn template my-stack --pretty    # Pretty-print JSON
cfn validate template.yaml        # Validate local template
```

## Global Options

- `-r, --region <region>` - AWS region (defaults to configured region)
- `--no-headers` - Omit table headers

## Configuration

Uses standard AWS credential configuration:
- Credentials from `~/.aws/credentials`
- `AWS_PROFILE` environment variable
- IAM role credentials (EC2/ECS/Lambda)

## Common Workflows

**Find stacks with specific Service Catalog products:**
```bash
cfn list --type AWS::ServiceCatalog::CloudFormationProvisionedProduct \
  --property ProductName=IAMRole \
  --property ProvisioningArtifactName=3.0.0
```

**Monitor deployment:**
```bash
cfn tail my-stack
```

**Bulk drift detection:**
```bash
cfn list --desc production --names-only | while read stack; do
  cfn drift "$stack"
done
```

**Export all templates:**
```bash
cfn list --names-only | while read stack; do
  cfn template "$stack" > "templates/${stack}.yaml"
done
```

**Bulk delete stacks (non-interactive):**
```bash
# Delete stacks whose name contains "preview"
cfn list preview --names-only | while read stack; do
  cfn delete "$stack" --yes
done
```

## Full Documentation

### `cfn` - Main Command

Root command and global options. [Documentation](./docs/cfn.md)

### `cfn list` - List Stacks

List CloudFormation stacks with filtering options, including deep search by resource type and properties. [Documentation](./docs/cfn_list.md)

### `cfn describe` - Stack Details

View comprehensive stack information. [Documentation](./docs/cfn_describe.md)

### `cfn delete` - Delete Stack

Delete stacks with confirmation and optional wait behavior. [Documentation](./docs/cfn_delete.md)

### `cfn events` - Stack Events

List stack events with optional failure filtering. [Documentation](./docs/cfn_events.md)

### `cfn tail` - Stream Events

Monitor stack events in real-time. [Documentation](./docs/cfn_tail.md)

### `cfn parameters` - Stack Parameters

Show stack parameters. [Documentation](./docs/cfn_parameters.md)

### `cfn outputs` - Stack Outputs

Show stack outputs. [Documentation](./docs/cfn_outputs.md)

### `cfn resources` - Stack Resources

List physical resources in a stack. [Documentation](./docs/cfn_resources.md)

### `cfn drift` - Drift Detection

Detect configuration drift. [Documentation](./docs/cfn_drift.md)

### `cfn template` - Get Template

Get deployed templates from live stacks. [Documentation](./docs/cfn_template.md)

### `cfn validate` - Validate Template

Validate CloudFormation templates. [Documentation](./docs/cfn_validate.md)

