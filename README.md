# AWS Deployer

A Go-based serverless application that automatically deploys CloudFormation templates using AWS Step Functions when
artifacts are uploaded to S3.

## Local Server

```bash
gin -i -p 4000 -a 4001 -t . -d internal/lambda/server -- --env "${ENV:=dev}" serve
```

## Configuration Management

AWS Deployer uses **AWS Systems Manager Parameter Store** for centralized configuration management. This allows updating configuration values (like allowed email addresses or deployment modes) without redeploying Lambda functions.

### Parameter Store Structure

Configuration parameters are stored at: `/{env}/aws-deployer/*`

Example parameters:
```
/dev/aws-deployer/state-machine-arn
/dev/aws-deployer/multi-account-state-machine-arn
/dev/aws-deployer/deployment-mode
/dev/aws-deployer/s3-bucket
/dev/aws-deployer/allowed-email
/dev/aws-deployer/session-token-secret-name
/dev/aws-deployer/custom-domain
/dev/aws-deployer/api-gateway-id
```

**Table names are NOT stored in Parameter Store** - they are derived from the environment name using the pattern `{env}-aws-deployer--{table-type}`. For example:
- `dev-aws-deployer--builds`
- `dev-aws-deployer--targets`
- `dev-aws-deployer--deployments`
- `dev-aws-deployer--locks`

### Viewing Configuration

```bash
# View all configuration for dev environment
aws ssm get-parameters-by-path --path "/dev/aws-deployer" --recursive

# View a specific parameter
aws ssm get-parameter --name "/dev/aws-deployer/allowed-email"

# View configuration values only
aws ssm get-parameters-by-path --path "/dev/aws-deployer" --recursive --query 'Parameters[].{Name:Name,Value:Value}' --output table
```

### Updating Configuration

Configuration can be updated without Lambda redeployment:

```bash
# Update allowed email for console access
aws ssm put-parameter --name "/dev/aws-deployer/allowed-email" --value "admin@example.com" --overwrite

# Change deployment mode
aws ssm put-parameter --name "/dev/aws-deployer/deployment-mode" --value "multi" --overwrite
```

Changes take effect on the next Lambda cold start or can be applied immediately by forcing a new container.

### Local Development

For local development, you can choose between using Parameter Store or environment variables:

**Option 1: Use Parameter Store (default)**
```bash
# Requires AWS credentials and parameters configured in AWS
export ENV=dev
go run internal/lambda/server/main.go serve
```

**Option 2: Use Environment Variables**
```bash
# No AWS connection required for Parameter Store
export DISABLE_SSM=true
export ENV=dev
export STATE_MACHINE_ARN=arn:aws:states:us-east-1:123456789012:stateMachine:dev-aws-deployer-deployment
export S3_BUCKET_NAME=lmvtfy-github-artifacts
# ... set other config as needed ...
go run internal/lambda/server/main.go serve --disable-auth
```

**Option 3: Using CLI Flag**
```bash
export ENV=dev
# Set other environment variables...
go run internal/lambda/server/main.go serve --disable-ssm --disable-auth
```

### Environment Variables Reference

**Always Required:**
- `ENV` or `ENVIRONMENT` - Determines parameter path and table names

**System Variables (set by AWS):**
- `AWS_LAMBDA_RUNTIME_API` - Present in Lambda environment
- `AWS_REGION` - AWS region

**Development Flags:**
- `DISABLE_SSM=true` - Use environment variables instead of Parameter Store
- `DISABLE_AUTH=true` - Disable authentication (local development only)

**When DISABLE_SSM=true:**
- `STATE_MACHINE_ARN` - Single-account deployment state machine ARN
- `MULTI_ACCOUNT_STATE_MACHINE_ARN` - Multi-account deployment state machine ARN (if using multi-account mode)
- `DEPLOYMENT_MODE` - `single` or `multi` (defaults to `single`)
- `S3_BUCKET_NAME` - S3 bucket for GitHub artifacts
- `ALLOWED_EMAIL` - Email address for authorization (optional)
- `SESSION_TOKEN_SECRET_NAME` - Secrets Manager secret name (optional, has default)
- `CUSTOM_DOMAIN` - Custom domain for API Gateway (optional)
- `API_GATEWAY_ID` - API Gateway ID (optional)

## Architecture

The system consists of the following components:

1. **S3 Trigger Lambda**: Monitors S3 bucket for `cloudformation-params.json` files and triggers Step Function
2. **Step Function**: Orchestrates the deployment workflow with the following steps:
    - Deploy CloudFormation stack (includes S3 download, status update, and deployment)
    - Monitor stack status until completion
3. **DynamoDB Table**: Tracks build status and metadata
4. **Lambda Functions**:
    - `deploy-cloudformation`: Combined function that downloads S3 content, updates build status, and deploys stack
    - `check-stack-status`: Monitors CloudFormation stack progress
    - `update-build-status`: Updates build status in DynamoDB

## Workflow

1. When a `cloudformation-params.json` file is uploaded to `s3://lmvtfy-github-artifacts/{repo}/{version}/`, it triggers
   the S3 Lambda
2. The S3 Lambda:
    - Parses the S3 path to extract repo and version information
    - Generates a new KSUID for the build
    - Creates a build record in DynamoDB with status `PENDING`
    - Starts a Step Function execution named `{repo}-{env}-{ksuid}`
3. The Step Function calls the `deploy-cloudformation` Lambda which:
    - Downloads all files from the S3 directory and parses parameters
    - Updates build status to `IN_PROGRESS` in DynamoDB
    - Creates or updates the CloudFormation stack
4. Stack deployment is monitored every 15 seconds until completion or failure
5. Final build status (`SUCCESS` or `FAILED`) is updated in DynamoDB

## Directory Structure

```
s3://lmvtfy-github-artifacts/{repo}/{version}/
├── cloudformation-params.json  # Parameters for the stack
├── cloudformation.template     # CloudFormation template
└── ... (other files)
```

### Version Format

The version follows the format: `{build_number}.{commit_hash}`

Example: `123.abcdef` where `123` is the build number and `abcdefghijkl` is the commit3 hash.

## Build Status Tracking

The DynamoDB table `dev-aws-deployer--builds` stores build information with a composite key structure:

### Build ID Format

Build IDs follow the format: `{repo}/{env}:{ksuid}`

Example: `my-app/dev:2HFj3kLmNoPqRsTuVwXy`

- `{repo}`: Repository name (e.g., `my-app`)
- `{env}`: Environment name (e.g., `dev`, `staging`, `prod`)
- `{ksuid}`: K-Sortable Unique Identifier (chronologically sortable unique ID)

### DynamoDB Table Structure

- **Primary Key (pk)**: `{repo}/{env}` (e.g., `my-app/dev`)
- **Sort Key (sk)**: KSUID (e.g., `2HFj3kLmNoPqRsTuVwXy`)
- **Attributes**:
    - `repo`: Repository name
    - `env`: Environment name
    - `build_number`: Build number from version (original value, not KSUID)
    - `branch`: Git branch name
    - `version`: Full version string ({build_number}.{commit_hash})
    - `commit_hash`: Git commit hash
    - `status`: Build status (see below)
    - `stack_name`: CloudFormation stack name
    - `start_time`: Build start timestamp
    - `end_time`: Build end timestamp (optional)
    - `error_msg`: Error message for failed builds (optional)

### Build Statuses

- `PENDING`: Build record created, waiting to start
- `IN_PROGRESS`: Step Function is running
- `SUCCESS`: CloudFormation stack deployed successfully
- `FAILED`: Deployment failed

### KSUID

KSUIDs (K-Sortable Unique Identifiers) provide several benefits:

- **Chronologically sortable**: Earlier builds have smaller KSUIDs
- **Collision-free**: Globally unique without coordination
- **URL-safe**: Base62 encoding with no special characters
- **Fixed length**: Always 27 characters
- **Timestamp embedded**: Contains creation time in first 4 bytes

## API Endpoints

The server provides both API endpoints and CLI commands for accessing build information:

### HTTP API

- **GET /api/health**: Health check endpoint
- **GET /api/builds?repo={repo}&env={env}**: List all builds for a repository/environment
- **GET /api/builds/{repo}/{env}/{ksuid}**: Get a specific build by KSUID

### CLI Commands

```bash
# List builds for a repository and environment
./bin/server list-builds --repo my-app --env dev

# Get a specific build
./bin/server get-build --repo my-app --env dev --ksuid 2HFj3kLmNoPqRsTuVwXy

# Start local development server
./bin/server serve --port 8080
```

## Creating GitHub OIDC Roles for CI/CD

The `aws-deployer setup-github` command automates the creation of IAM roles for GitHub Actions OIDC authentication, providing
secure, credential-free access to AWS resources.

### Why This Tool Exists

GitHub Actions workflows need AWS credentials to upload build artifacts to S3. This tool uses **OpenID Connect (OIDC)
** - the modern, recommended approach that eliminates long-lived credentials:

1. **No long-lived credentials**: Uses OIDC tokens instead of AWS access keys, eliminating the risk of leaked secrets
2. **Scoped IAM roles**: Each repository gets a dedicated IAM role with minimal permissions (only S3 access to their
   specific path)
3. **Automatic OIDC provider setup**: Creates and configures the GitHub OIDC identity provider in your AWS account
4. **Secure secret management**: Stores only the role ARN in GitHub secrets (not sensitive credentials)
5. **Short-lived tokens**: GitHub Actions receives temporary AWS credentials that expire after 1 hour
6. **Audit trail**: All IAM roles follow a consistent naming convention and permission model

### Prerequisites

Before using this tool, you need:

1. A GitHub Personal Access Token (PAT) with `repo` scope stored in AWS Secrets Manager as:
   ```json
   {
     "github_pat": "ghp_xxxxxxxxxxxxx"
   }
   ```
2. AWS credentials with permissions to:
    - Create IAM users and policies
    - Create IAM access keys
    - Read from Secrets Manager
3. The GitHub repository where secrets will be created

### Usage

```bash
# Build the tool
make build

# Create a GitHub OIDC role and provision secrets
aws-deployer setup-github \
  --role-name github-actions-myrepo \
  --repo owner/repository-name \
  --bucket lmvtfy-github-artifacts \
  --github-token-secret github/pat-token

# Or use short flags and environment variables
export GITHUB_REPO=owner/repository-name
export S3_ARTIFACT_BUCKET=lmvtfy-github-artifacts
export GITHUB_TOKEN_SECRET=github/pat-token

aws-deployer setup-github -n github-actions-myrepo
```

### What It Does

1. **Creates/verifies OIDC provider**: Ensures the GitHub OIDC identity provider exists in your AWS account
2. **Creates IAM role**: Creates an IAM role with a trust policy that allows GitHub Actions from your specific
   repository
3. **Attaches scoped policy**: Grants S3 permissions limited to `s3://bucket/repo/*`:
    - `s3:PutObject` on `arn:aws:s3:::bucket/repo/*`
    - `s3:ListBucket` with prefix condition for `repo/*`
4. **Fetches GitHub PAT**: Retrieves GitHub token from AWS Secrets Manager
5. **Creates GitHub secret**: Automatically creates encrypted repository secret:
    - `AWS_ROLE_ARN`

### Example Output

```
✓ IAM role github-actions-myrepo created successfully
✓ Role ARN: arn:aws:iam::123456789012:role/github-actions-myrepo
✓ IAM policy grants S3 access to: lmvtfy-github-artifacts/repository-name/*
✓ Trust policy allows GitHub Actions from: owner/repository-name
✓ GitHub secret created in: owner/repository-name
  - AWS_ROLE_ARN

🔐 Using OIDC authentication (no long-lived credentials needed)
```

### CLI Flags

| Flag                    | Short | Environment Variable  | Description                           |
|-------------------------|-------|-----------------------|---------------------------------------|
| `--role-name`           | `-n`  | `GITHUB_ROLE_NAME`    | IAM role name to create               |
| `--repo`                | `-r`  | `GITHUB_REPO`         | Repository in format `owner/repo`     |
| `--bucket`              | `-b`  | `S3_ARTIFACT_BUCKET`  | S3 artifact bucket name               |
| `--github-token-secret` | `-t`  | `GITHUB_TOKEN_SECRET` | Path to GitHub PAT in Secrets Manager |

### Using OIDC in GitHub Actions

After running this tool, your GitHub Actions workflow can use OIDC authentication:

```yaml
name: Deploy
on: [ push ]

# Required for OIDC token
permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
          aws-region: us-east-1

      - name: Upload to S3
        run: |
          aws s3 cp ./artifacts/ s3://lmvtfy-github-artifacts/my-repo/${{ github.run_number }}/ --recursive
```

**Key Differences from Access Keys:**

- No `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY` needed
- Credentials are temporary (expire after 1 hour)
- No risk of leaked long-lived credentials
- Requires `permissions: id-token: write` in workflow

## Usage

### Prerequisites

- Go 1.24+
- AWS CLI configured
- AWS credentials with appropriate permissions

### Building

```bash
# Install dependencies
make deps

# Build all Lambda functions
make build

# Run tests
make test
```

### Deployment

The deployment process automatically generates a version timestamp and uploads Lambda packages to S3 before deployment.

```bash
# Upload Lambda packages to S3 (with auto-generated version)
make upload-to-s3

# Deploy infrastructure (includes upload-to-s3)
make deploy-infrastructure

# Update Lambda function code only (from S3)
make update-lambda-code

# Configure S3 bucket notification only
make configure-s3-notification

# Full deployment (infrastructure + S3 notification)
make deploy

# Deploy to specific environment
ENVIRONMENT=staging make deploy

# Deploy with custom version
VERSION=20240102-1430 make deploy
```

**Version Management**:

- Versions are auto-generated as `YYYYMMDD-HHMMSS` (e.g., `20240102-143045`)
- Version is stored in `.version` file and remains consistent within a session
- Lambda packages are uploaded to `s3://lmvtfy-github-artifacts/aws-deployer/{version}/`
- Generate new version: `make clean-version && make deploy`
- Override version: `VERSION=custom-version make deploy`
- Check current version: `make show-version`

**Note**: The S3 bucket (`lmvtfy-github-artifacts`) must exist before deployment. The bucket notification is configured
separately as the final step.

### Monitoring

```bash
# View S3 trigger Lambda logs
make logs-s3-trigger

# List Step Function executions
make logs-step-function

# Describe CloudFormation stack
make describe-stack
```

### Example CloudFormation Parameters File

```json
{
  "ParameterKey1": "ParameterValue1",
  "ParameterKey2": "ParameterValue2",
  "Environment": "dev"
}
```

## IAM Permissions

The Lambda functions require the following permissions:

- **S3**: `GetObject`, `ListBucket` on the artifacts bucket
- **CloudFormation**: `CreateStack`, `UpdateStack`, `DescribeStacks`, `DescribeStackEvents`
- **DynamoDB**: `GetItem`, `PutItem`, `UpdateItem` on the builds table
- **Step Functions**: `StartExecution` on the deployment state machine
- **IAM**: Various permissions for CloudFormation to manage resources

## Error Handling

- All Lambda functions include comprehensive error handling
- Failed deployments are tracked in DynamoDB with error messages
- Step Function includes retry logic and failure handling
- CloudFormation stack events are logged for debugging failed deployments

## Development

The project follows standard Go project structure:

- `cmd/`: CLI tools and utilities
- `internal/lambda/`: Lambda functions
- `internal/models/`: Data models
- `internal/services/`: Business logic services
- `infrastructure.yml`: CloudFormation template for the infrastructure
- `step-function-definition.json`: Step Function state machine definition

### Testing

```bash
make test
```

### Cleaning

```bash
make clean
```

## Stack Naming Convention

CloudFormation stacks are named using the pattern: `${ENV}-{repo}`

Examples:

- `dev-my-app` (for repo "my-app" in dev environment)
- `prod-api-service` (for repo "api-service" in prod environment)