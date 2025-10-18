# Deployment Targets Management

The `aws-deployer targets` CLI manages multi-account CloudFormation deployment configurations, including where to deploy and how deployments progress through environments.

## Concepts

### Two Environment Types

1. **AWS Deployer Environment (`--env`)**: Which instance of aws-deployer you're using
   - Values: `dev`, `stg`, `prd`
   - Determines which DynamoDB table to read/write
   - Example: `dev-aws-deployer--targets`

2. **Target Environment (`--target-env`)**: The environment being deployed to
   - Values: `dev`, `stg`, `prd`
   - Stored in DynamoDB records
   - Defines the actual deployment destination

### Default vs Repo-Specific Configuration

- **Default configuration**: Applies to all repositories unless overridden
- **Repo-specific configuration**: Overrides default for a particular repository

### Configuration Types

1. **Deployment Targets**: Define which AWS accounts/regions to deploy to
2. **Initial Environment**: The first environment in the deployment progression
3. **Downstream Environments**: Which environments come next after deployment

## Commands

### `config` - Manage Initial Environment

Set or view the initial environment for deployments.

```bash
# Set default initial environment to dev (applies to all repos)
aws-deployer targets config --env dev --default --initial-env dev

# Set repo-specific initial environment to stg
aws-deployer targets config --env prd --repo my-app --initial-env stg

# View default initial environment configuration
aws-deployer targets config --env dev --default

# View repo-specific initial environment configuration
aws-deployer targets config --env prd --repo my-app
```

**Fallback behavior**:
- Checks repo-specific config first
- Falls back to default config if not set
- Ultimate fallback is `dev`

### `set` - Set Deployment Targets

Configure deployment targets and downstream environments.

```bash
# Set default targets for dev environment
aws-deployer targets set --env dev --target-env dev --default \
  --accounts "123456789012,987654321098" \
  --regions "us-east-1,us-west-2"

# Set repo targets with downstream environment (dev flows to stg)
aws-deployer targets set --env dev --target-env dev --repo my-app \
  --accounts "123456789012" \
  --regions "us-east-1,us-west-2" \
  --downstream-env "stg"

# Set staging targets with downstream to production
aws-deployer targets set --env stg --target-env stg --repo my-app \
  --accounts "123456789012" \
  --regions "us-east-1,us-west-2,eu-west-1" \
  --downstream-env "prd"

# Set production targets (no further promotion)
aws-deployer targets set --env prd --target-env prd --repo my-app \
  --accounts "123456789012" \
  --regions "us-east-1,us-west-2,eu-west-1,ap-southeast-1"

# Use JSON for complex targets
aws-deployer targets set --env dev --target-env dev --default \
  --targets-json '[{"account_ids":["123456789012"],"regions":["us-east-1","us-west-2"]}]'

# Overwrite existing targets
aws-deployer targets set --env dev --target-env dev --repo my-app \
  --accounts "123456789012" \
  --regions "us-east-1" \
  --overwrite
```

**Protection**:
- Requires `--overwrite` flag to replace existing targets
- Prevents accidental configuration changes

### `list` - List Deployment Targets

View deployment targets across all or specific environments.

```bash
# List all target environments for default targets
aws-deployer targets list --env dev

# List all target environments for a repo (shows progression)
aws-deployer targets list --env prd --repo my-app

# List specific target environment
aws-deployer targets list --env stg --target-env stg --repo my-app

# List as JSON for scripting
aws-deployer targets list --env dev --json

# List specific target environment as JSON
aws-deployer targets list --env dev --target-env dev --repo my-app --json
```

**Automatic fallback**:
- When querying repo-specific targets, automatically falls back to defaults
- Clearly indicates when default targets are being used
- Shows downstream environments when configured

### `delete` - Delete Deployment Targets

Remove deployment target configurations.

```bash
# Delete default targets for dev environment
aws-deployer targets delete --env dev --target-env dev --default

# Delete repo-specific targets
aws-deployer targets delete --env prd --target-env prd --repo my-app

# Delete without confirmation prompt
aws-deployer targets delete --env dev --target-env dev --repo my-app --force
```

**Safety**:
- Shows summary before deletion
- Requires confirmation unless `--force` is specified

## Data Model

### DynamoDB Schema

**Table**: `{env}-aws-deployer--targets` (e.g., `dev-aws-deployer--targets`)

**Partition Key (PK)**: Repository name (use `"$"` for default)

**Sort Key (SK)**:
- Environment name (`dev`, `stg`, `prd`) for deployment targets
- `"$"` for configuration record (initial env)

**Attributes**:

| Field | Type | Used When | Description |
|-------|------|-----------|-------------|
| `targets` | Target[] | SK is env | Account/region combinations to deploy to |
| `downstream_env` | string[] | SK is env | Next environments in deployment flow |
| `initial_env` | string | SK is "$" | Starting environment for deployments |

### Target Structure

```json
{
  "account_ids": ["123456789012", "987654321098"],
  "regions": ["us-east-1", "us-west-2"]
}
```

Each target represents a Cartesian product of accounts and regions. Multiple targets can be specified for complex deployment scenarios.

## Example Workflow

### 1. Configure Initial Environment

```bash
# Set default initial environment to dev for all repos
aws-deployer targets config --env dev --default --initial-env dev
```

### 2. Set Up Deployment Progression

```bash
# Dev environment targets (flows to stg)
aws-deployer targets set --env dev --target-env dev --default \
  --accounts "123456789012" \
  --regions "us-east-1" \
  --downstream-env "stg"

# Staging environment targets (flows to prd)
aws-deployer targets set --env stg --target-env stg --default \
  --accounts "123456789012" \
  --regions "us-east-1,us-west-2" \
  --downstream-env "prd"

# Production environment targets (no downstream)
aws-deployer targets set --env prd --target-env prd --default \
  --accounts "123456789012" \
  --regions "us-east-1,us-west-2,eu-west-1"
```

### 3. Override for Specific Repository

```bash
# my-critical-app starts in staging, not dev
aws-deployer targets config --env dev --repo my-critical-app --initial-env stg

# my-critical-app has different targets in production
aws-deployer targets set --env prd --target-env prd --repo my-critical-app \
  --accounts "123456789012,987654321098" \
  --regions "us-east-1,us-west-2,eu-west-1,ap-southeast-1"
```

### 4. View Configuration

```bash
# View all default targets across environments
aws-deployer targets list --env dev

# View specific repo targets across environments
aws-deployer targets list --env prd --repo my-app

# View initial environment configuration
aws-deployer targets config --env dev --repo my-app
```

## Deployment Progression Example

With the following configuration:

```bash
# Initial env: dev
aws-deployer targets config --env dev --default --initial-env dev

# dev → stg
aws-deployer targets set --env dev --target-env dev --default \
  --accounts "123456789012" --regions "us-east-1" --promotion-path "stg"

# stg → prd
aws-deployer targets set --env stg --target-env stg --default \
  --accounts "123456789012" --regions "us-east-1,us-west-2" --promotion-path "prd"

# prd (final)
aws-deployer targets set --env prd --target-env prd --default \
  --accounts "123456789012" --regions "us-east-1,us-west-2,eu-west-1"
```

When listing all environments:

```
aws-deployer targets list --env dev
```

Output shows progression:

```
Default deployment targets across environments
================================================================================

Target Environment: dev
Downstream: dev → stg

Total deployments: 1
Targets by account:
  Account: 123456789012
    Regions: us-east-1

--------------------------------------------------------------------------------

Target Environment: stg
Downstream: stg → prd

Total deployments: 2
Targets by account:
  Account: 123456789012
    Regions: us-east-1, us-west-2

--------------------------------------------------------------------------------

Target Environment: prd

Total deployments: 3
Targets by account:
  Account: 123456789012
    Regions: us-east-1, us-west-2, eu-west-1
```

## Advanced Use Cases

### Multiple Account Groups

Use JSON to define complex deployment patterns:

```bash
aws-deployer targets set --env prd --target-env prd --default \
  --targets-json '[
    {
      "account_ids": ["123456789012"],
      "regions": ["us-east-1", "us-west-2"]
    },
    {
      "account_ids": ["987654321098"],
      "regions": ["eu-west-1", "ap-southeast-1"]
    }
  ]'
```

This creates 6 total deployments:
- Account 123456789012: us-east-1, us-west-2
- Account 987654321098: eu-west-1, ap-southeast-1

### Branching Deployment Flow

Deploy to multiple downstream environments simultaneously:

```bash
# Dev can flow to both stg and prd
aws-deployer targets set --env dev --target-env dev --repo experimental-app \
  --accounts "123456789012" \
  --regions "us-east-1" \
  --downstream-env "stg,prd"
```

### Environment-Specific AWS Deployer Instances

Use different AWS deployer instances per environment:

```bash
# Configure dev aws-deployer instance
aws-deployer targets set --env dev --target-env dev --default \
  --accounts "123456789012" --regions "us-east-1"

# Configure production aws-deployer instance (separate table)
aws-deployer targets set --env prd --target-env prd --default \
  --accounts "987654321098" --regions "us-east-1,us-west-2,eu-west-1"
```

## Data Storage

All configuration is stored in DynamoDB tables named `{env}-aws-deployer--targets`.

### Configuration Record (SK="$")
```
PK: "$" or "repo-name"
SK: "$"
InitialEnv: "dev"
```

### Environment Targets Record
```
PK: "$" or "repo-name"
SK: "dev" | "stg" | "prd"
Targets: [...]
DownstreamEnv: ["stg"] or ["prd"] or []
```

## Tips

1. **Start with defaults**: Configure default targets for all repos, then override for specific repos as needed

2. **View before setting**: Use `list` to see current configuration before making changes

3. **Use --overwrite carefully**: The flag is required to prevent accidental overwrites

4. **JSON for complex scenarios**: Use `--targets-json` for complex multi-account/region setups

5. **Test in dev first**: Configure and test in dev aws-deployer before replicating to stg/prd

6. **Document downstream flows**: Clearly define how deployments flow through environments
