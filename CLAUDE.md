# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AWS Deployer is a serverless CloudFormation deployment automation system built in Go. It monitors S3 for build artifacts and orchestrates deployments via AWS Step Functions, supporting both single-account and multi-account (StackSet-based) deployments.

## Development Commands

### Building and Testing

```bash
# Build all Lambda functions (creates build/ directory with .zip files)
make build

# Run tests
make test

# Run benchmarks (requires Docker for DynamoDB Local)
make bench

# Build the CLI tool
make build-cli
```

### Running Locally

```bash
# Local server with live reload (requires gin)
gin -i -p 4000 -a 4001 -t . -d internal/lambda/server -- --env "${ENV:=dev}" serve

# Or without gin
go run internal/lambda/server/main.go --env dev serve --disable-auth --disable-ssm

# Frontend development
make frontend-dev
```

### Running Individual Tests

```bash
# Run tests for a specific package
go test ./internal/dao/builddao/... -v

# Run specific test function
go test ./internal/dao/builddao/... -v -run TestDAO_Create

# Skip integration tests (unit tests only)
go test -short ./internal/dao/builddao/... -v

# Integration tests require DynamoDB Local:
docker run -p 8000:8000 amazon/dynamodb-local
DYNAMODB_ENDPOINT=http://localhost:8000 go test ./internal/dao/builddao/... -v
```

### Deployment

```bash
# Full deployment (infrastructure + Lambda functions)
make deploy

# Deploy to specific environment
ENVIRONMENT=staging make deploy

# Update Lambda code only (faster than full deploy)
make update-lambda-code

# Show current version
make show-version

# Generate new version for next deployment
make clean-version
```

## Architecture

### Deployment Modes

The system supports two deployment modes via the `DEPLOYMENT_MODE` parameter:

1. **Single-account mode**: Deploys CloudFormation stacks directly to a single AWS account
   - Step Function: `{env}-aws-deployer-deployment`
   - Defined in: `step-function-definition.json`

2. **Multi-account mode**: Deploys via CloudFormation StackSets across multiple accounts/regions
   - Step Function: `{env}-aws-deployer-multi-account-deployment`
   - Defined in: `multi-account-step-function-definition.json`
   - See `MULTI.md` for detailed workflow documentation

### Key Components

**Dependency Injection (`internal/di/`)**
- Uses uber/dig for dependency injection
- Core providers registered in `internal/di/dig.go`
- Environment-specific configuration via `internal/di/options.go`
- All Lambda functions and services use DI for initialization

**Data Access Layer (`internal/dao/`)**
- Uses [savaki/ddb/v2](https://github.com/savaki/ddb) for DynamoDB operations
- All DAOs follow consistent patterns with type-safe keys
- Unix epoch timestamps (int64) instead of time.Time
- See `internal/dao/builddao/README.md` for DAO patterns

**Build Tracking (`internal/dao/builddao/`)**
- Primary key: `{repo}/{env}` (PK type), KSUID (SK)
- "Latest magic records": Automatic denormalization for efficient latest-build queries
  - Uses `pk=latest/{env}` and `sk={repo}/{env}`
  - Created/updated automatically on every `UpdateStatus()` call
  - Enables querying latest builds without GSI
- Build ID format: `{repo}/{env}:{ksuid}` (ID type)
- Always use `builddao.NewPK(repo, env)` and `builddao.BuildID(pk, sk)` for type safety

**Multi-Account Deployments (`internal/dao/targetdao/`, `internal/dao/deploymentdao/`, `internal/dao/lockdao/`)**
- `targetdao`: Manages deployment targets (accounts/regions) with default fallback
- `deploymentdao`: Tracks per-account/region deployment status
- `lockdao`: Distributed lock to prevent concurrent deployments to same repo/env
- See `DEPLOYMENT_TARGETS.md` for CLI usage

**Step Functions Orchestration (`internal/orchestrator/`)**
- Starts Step Function executions for deployments
- Atomically updates build status to IN_PROGRESS with execution ARN
- Execution naming: `{repo}-{env}-{ksuid}` (e.g., `myapp-dev-{ksuid}`)

**GraphQL API (`internal/gql/`)**
- Schema: `internal/gql/schema.graphqls`
- Resolver: `internal/gql/resolver.go`
- Frontend types generated via `make frontend-codegen`

### Data Flow

1. **Trigger**: GitHub Actions uploads params file to S3 at `s3://{bucket}/{repo}/{branch}/{version}/`
   - `cloudformation-params.json` - triggers deployment
   - `container-images.json` (optional) for Docker image promotion
2. **S3 Lambda** (`internal/lambda/s3-trigger`): Creates PENDING build record with KSUID
3. **DynamoDB Stream** → **trigger-build Lambda**: Starts Step Function execution
   - Execution name: `{repo}-{env}-{ksuid}`
4. **Step Function**: Orchestrates deployment (single or multi-account)
   - **promote-images**: Promotes Docker images if `container-images.json` exists
5. **Status Updates**: Lambda functions update build status throughout workflow
6. **GraphQL/Frontend**: Queries build records for UI display

### Standard CloudFormation Parameters

All CloudFormation templates deployed via AWS Deployer should accept these standard parameters:

- `Env` - Environment name (dev, staging, prod)
- `Version` - Build version in format `{build_number}.{commit_hash}`
- `S3Bucket` - Artifacts bucket name
- `S3Prefix` - S3 path to artifacts in format `{repo}/{branch}/{version}`

**File naming convention:**
- Template: `cloudformation.template`
- Parameters: `cloudformation-params.json`
- Environment overrides: `cloudformation-params.{env}.json`

### Docker Image Promotion (Optional)

AWS Deployer can promote Docker images from a source ECR registry to target ECR registries before CloudFormation deployment. This is useful for multi-stage deployments where images are built in a CI account and promoted to deployment accounts.

**To enable:** Include a `deploy-manifest.json` file in your S3 artifacts:

```json
{
  "images": [
    {"repository": "myapp/api", "tag": "1.0.0-abc123"},
    {"repository": "myapp/worker", "tag": "1.0.0-abc123"}
  ]
}
```

- `repository`: ECR repository name (must exist in both source and target accounts)
- `tag`: Image tag to promote

**Behavior:**
- If `deploy-manifest.json` is missing, the step is skipped (no-op)
- Cross-account: Layers are checked for availability, missing layers are downloaded from source and uploaded to target
- Same-account: Only the manifest is copied (layers share the same backing store)
- Idempotent: already-existing images are skipped without error
- In multi-account mode, images are promoted to each target account in parallel (max 5 concurrent)

**Workflow position:**
- Single-account: Runs after signature verification, before CloudFormation deployment
- Multi-account: Runs after fetching targets, before initializing deployments

**Stack naming:** `{env}-{repo}` (e.g., `dev-myapp`)

### Environment Configuration

**Parameter Store Pattern**: `/{env}/aws-deployer/{parameter-name}`

Core parameters:
- `state-machine-arn`: Single-account deployment Step Function
- `multi-account-state-machine-arn`: Multi-account deployment Step Function
- `deployment-mode`: `single` or `multi`
- `s3-bucket`: Artifacts bucket
- `allowed-email`: Authorization (optional)

**Table Naming**: `{env}-aws-deployer--{table-type}` (not in Parameter Store)
- `{env}-aws-deployer--builds`
- `{env}-aws-deployer--targets`
- `{env}-aws-deployer--deployments`
- `{env}-aws-deployer--locks`

**Local Development Flags**:
- `DISABLE_SSM=true`: Use environment variables instead of Parameter Store
- `DISABLE_AUTH=true`: Disable OAuth authentication

## Code Patterns

### Type-Safe Keys and IDs

Always use type-safe constructors instead of string formatting:

```go
// ✅ Correct - type-safe PK and ID construction
pk := builddao.NewPK(repo, env)
id := builddao.BuildID(pk, sk)
record, err := dao.Find(ctx, id)

// ❌ Incorrect - error-prone string manipulation
pk := fmt.Sprintf("%s/%s", repo, env)
id := fmt.Sprintf("%s:%s", pk, sk)
```

### Timestamps

Use Unix epoch (int64) for all timestamps:

```go
// ✅ Correct - Unix epoch
record.CreatedAt = time.Now().Unix()

// ❌ Incorrect - time.Time
record.CreatedAt = time.Now()
```

### Lambda Function Structure

All Lambda functions follow this pattern:

```go
func main() {
    logger := di.ProvideLogger()
    ctx := logger.WithContext(context.Background())

    container, err := di.New(env, di.WithProviders(...))
    if err != nil {
        logger.Fatal().Err(err).Msg("dependency injection failed")
    }

    handler := di.MustGet[HandlerType](container)
    lambda.Start(handler)
}
```

### Using the Orchestrator

```go
orchestrator := orchestrator.New(sfnClient, stateMachineArn, dao)

input := orchestrator.StepFunctionInput{
    Repo:       "myapp",
    Env:        "dev",
    SK:         ksuid.New().String(),
    Branch:     "main",
    Version:    "1.2.3",
    CommitHash: "abc123",
    S3Bucket:   "artifacts-bucket",
    S3Key:      "myapp/main/1.2.3/",
}

executionArn, err := orchestrator.StartExecution(ctx, input)
```

## Testing Strategy

- **Unit tests**: Test key types, business logic, no external dependencies
- **Integration tests**: Require DynamoDB Local via Docker
- Use `builddao.NewPK()` and `builddao.BuildID()` in tests for consistency
- Each integration test creates a unique table with KSUID and cleans up automatically
- Frontend uses Vite with TypeScript and React

## Multi-Account Deployments

The multi-account workflow uses 9 Lambda functions orchestrated by a Step Function:

1. **acquire-lock**: Distributed lock with retry (max 5 minutes)
2. **fetch-targets**: Query deployment targets from DynamoDB
3. **promote-images**: Promote Docker images to each target account (parallel, max 5 concurrent)
4. **initialize-deployments**: Create PENDING deployment records
5. **create-stackset**: Create/update CloudFormation StackSet template
6. **deploy-stack-instances**: Deploy instances to target accounts/regions
7. **check-stackset-status**: Poll until complete (15s intervals), update deployment records
8. **aggregate-results**: Summarize success/failure, update build status
9. **release-lock**: Release distributed lock

Error handling: All failures release the lock before exiting.

See `MULTI.md` for complete workflow documentation including failure modes and retry logic.

## GraphQL Schema

Frontend types are generated from `internal/gql/schema.graphqls`:

```bash
# Copy schema to frontend and generate TypeScript types
make frontend-codegen

# Or manually:
cp internal/gql/schema.graphqls frontend/schema.graphqls
cd frontend && npm run codegen
```

## CLI Tools

**aws-deployer CLI** (`cmd/aws-deployer/`):
- `setup-aws`: Configure AWS accounts for multi-account deployments
- `setup-github`: Create GitHub OIDC roles and secrets for CI/CD
- `targets`: Manage deployment targets and environment progression

See `DEPLOYMENT_TARGETS.md` for targets CLI documentation.

## Important File Paths

- Lambda functions: `internal/lambda/`
  - Server: `internal/lambda/server/` (GraphQL API)
  - Single-account: `internal/lambda/step-functions/`
  - Multi-account: `internal/lambda/step-functions/multi-account/`
- DAOs: `internal/dao/{builddao,targetdao,deploymentdao,lockdao}/`
- DI setup: `internal/di/`
- GraphQL: `internal/gql/`
- Frontend: `frontend/src/`
- CloudFormation: `cloudformation.template`
- Step Functions: `step-function-definition.json`, `multi-account-step-function-definition.json`