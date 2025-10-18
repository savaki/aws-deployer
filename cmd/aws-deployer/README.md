# aws-deployer CLI

Unified command-line tool for AWS Deployer - manage multi-account deployments, GitHub OIDC setup, and deployment targets.

## Directory Structure

```
cmd/aws-deployer/
├── main.go              # Entry point (only .go file in root)
└── commands/            # Command implementations
    ├── setup_aws.go     # AWS multi-account setup
    ├── setup_github.go  # GitHub OIDC configuration
    └── targets.go       # Deployment target management
```

## Quick Start

### Run without building:
```bash
# From project root - any of these work!
go run ./cmd/aws-deployer --help
go run cmd/aws-deployer/main.go --help  # Now works! (main.go is standalone)
cd cmd/aws-deployer && go run main.go --help

# Run specific commands
go run ./cmd/aws-deployer aws --help
go run ./cmd/aws-deployer github --help
go run ./cmd/aws-deployer targets --help
```

### Build and run:
```bash
# Build from project root
make build-cli

# Or build manually
cd cmd/aws-deployer
go build -o aws-deployer .

# Run the binary
./cmd/aws-deployer/aws-deployer --help
```

## Available Commands

### `aws` - Multi-account AWS setup
Configure IAM roles for CloudFormation StackSets across multiple AWS accounts.

**Subcommands:**
- `setup-deployer` - Create administration and execution roles in deployer account
- `setup-target` - Create StackSet execution role in target accounts
- `verify` - Verify cross-account access is configured correctly
- `teardown` - Remove StackSet execution role from account

**Example:**
```bash
go run ./cmd/aws-deployer aws setup-deployer --deployer-account 111111111111
go run ./cmd/aws-deployer aws setup-target --deployer-account 111111111111
go run ./cmd/aws-deployer aws verify --deployer-account 111111111111 --target-account 222222222222
```

### `github` - GitHub OIDC role setup
Create IAM roles for GitHub Actions OIDC authentication.

**Example:**
```bash
go run ./cmd/aws-deployer github \
  --repo owner/repository \
  --bucket my-artifacts-bucket \
  --github-token-secret github/pat-token
```

### `targets` - Manage deployment targets
Configure which AWS accounts and regions to deploy to.

**Subcommands:**
- `set` - Set deployment targets
- `list` - List deployment targets
- `config` - Manage initial environment configuration
- `delete` - Delete deployment targets

**Examples:**
```bash
# Set default targets
go run ./cmd/aws-deployer targets set \
  --env dev \
  --target-env dev \
  --default \
  --accounts "123456789012,987654321098" \
  --regions "us-east-1,us-west-2"

# List targets
go run ./cmd/aws-deployer targets list --env dev

# Set repo-specific targets
go run ./cmd/aws-deployer targets set \
  --env prd \
  --target-env prd \
  --repo my-app \
  --accounts "123456789012" \
  --regions "us-east-1,us-west-2,eu-west-1"
```

## Why This Structure?

✅ **Benefits:**

1. **`main.go` is standalone** - The only `.go` file in the root directory
   - You can now run `go run cmd/aws-deployer/main.go` successfully
   - No more "undefined" errors from missing package files

2. **Clean separation** - Command logic is in the `commands/` subpackage
   - Main package: Just the entry point and wiring
   - Commands package: All command implementations

3. **Flexible execution** - All these work:
   ```bash
   go run ./cmd/aws-deployer              # Run package
   go run cmd/aws-deployer/main.go        # Run single file
   cd cmd/aws-deployer && go run main.go  # From directory
   ```

4. **Better organization** - Related code grouped in subpackages
