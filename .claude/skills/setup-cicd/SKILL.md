# Setup CI/CD for AWS Deployer

This skill automates the setup of CI/CD for projects that will be deployed using AWS Deployer.

## Overview

You will help set up a complete CI/CD pipeline that:
- Auto-detects the project's programming language
- Creates or validates CloudFormation templates
- Generates GitHub Actions workflows
- Configures AWS authentication and S3 artifact uploads

## Step 1: Detect Project Language

Check for the following files to determine the project language:

- **Go**: `go.mod`, `main.go`, or `*.go` files
- **Rust**: `Cargo.toml`, `Cargo.lock`, or `*.rs` files
- **Node.js/TypeScript**: `package.json`, `tsconfig.json`
- **Python**: `requirements.txt`, `setup.py`, `pyproject.toml`

If multiple languages are detected, ask the user which is the primary language for deployment.

## Step 2: Determine Deployment Mode

Ask the user: **"Is this for single-account or multi-account deployment?"**

- **Single-account**: Deploys CloudFormation stacks directly to a single AWS account. Requires `Env` parameter.
- **Multi-account**: Deploys via CloudFormation StackSets across multiple accounts/regions. Does NOT use `Env` parameter.

Store this choice as it affects subsequent steps.

## Step 3: Gather Configuration

Collect the following information from environment variables or by prompting the user:

1. **S3_ARTIFACT_BUCKET**: The S3 bucket for storing build artifacts
   - Try: `echo $S3_ARTIFACT_BUCKET` or check GitHub secrets
   - If not found, prompt: "What is your S3 artifacts bucket name?"

2. **ENV** (single-account mode only): The environment name (dev, staging, prod)
   - Try: `echo $ENV`
   - If not found, prompt: "What environment is this for? (dev/staging/prod)"

3. **Repository info**:
   - Owner and repo name for GitHub OIDC setup (e.g., "owner/repo")
   - Derive from: `git remote get-url origin` or prompt user

## Step 4: Check/Create CloudFormation Template

### Check for existing template

Look for `cloudformation.template` in the project root.

### If template exists:

Verify it has the required parameters:
- `Version` (Type: String) - Build version
- `S3Bucket` (Type: String) - S3 bucket name
- `S3Prefix` (Type: String) - S3 path prefix
- `Env` (Type: String) - **Only for single-account mode**

If any are missing, add them to the Parameters section while preserving existing content.

### If template does NOT exist:

Create a minimal CloudFormation template with the required parameters based on deployment mode:

**For single-account mode:**
```yaml
AWSTemplateFormatVersion: '2010-09-09'
Description: 'Application deployment via AWS Deployer'

Parameters:
  Env:
    Type: String
    Description: Environment name (dev, staging, prod)

  Version:
    Type: String
    Description: Build version (e.g., 123.abc456)

  S3Bucket:
    Type: String
    Description: S3 bucket containing build artifacts

  S3Prefix:
    Type: String
    Description: S3 key prefix for artifacts (e.g., myapp/main/123.abc456/)

Resources:
  # Add your AWS resources here
  PlaceholderResource:
    Type: AWS::CloudFormation::WaitConditionHandle
```

**For multi-account mode:**
```yaml
AWSTemplateFormatVersion: '2010-09-09'
Description: 'Application deployment via AWS Deployer (multi-account)'

Parameters:
  Version:
    Type: String
    Description: Build version (e.g., 123.abc456)

  S3Bucket:
    Type: String
    Description: S3 bucket containing build artifacts

  S3Prefix:
    Type: String
    Description: S3 key prefix for artifacts (e.g., myapp/main/123.abc456/)

Resources:
  # Add your AWS resources here
  PlaceholderResource:
    Type: AWS::CloudFormation::WaitConditionHandle
```

Inform the user they'll need to add actual AWS resources to the template.

## Step 5: Generate GitHub Actions Workflow

Create `.github/workflows/deploy.yml` with the following structure:

### Language-specific build steps:

**Go:**
```yaml
- name: Build Go binaries
  run: |
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bootstrap .
    cd build && zip -r ../app.zip .
```

**Rust:**
```yaml
- name: Install Rust toolchain
  uses: actions-rs/toolchain@v1
  with:
    toolchain: stable
    target: x86_64-unknown-linux-musl

- name: Build Rust binary
  run: |
    cargo build --release --target x86_64-unknown-linux-musl
    mkdir -p build
    cp target/x86_64-unknown-linux-musl/release/bootstrap build/
    cd build && zip -r ../app.zip .
```

**Node.js/TypeScript:**
```yaml
- name: Install dependencies
  run: npm ci

- name: Build application
  run: npm run build

- name: Package application
  run: |
    mkdir -p build
    cp -r dist/* build/
    cp package.json package-lock.json build/
    cd build && npm ci --production && zip -r ../app.zip .
```

**Python:**
```yaml
- name: Install dependencies
  run: |
    pip install -r requirements.txt -t build/
    cp -r *.py build/

- name: Package application
  run: cd build && zip -r ../app.zip .
```

### Complete workflow template:

```yaml
name: Deploy to AWS

on:
  push:
    branches:
      - main
      - develop

env:
  S3_ARTIFACT_BUCKET: ${{ secrets.S3_ARTIFACT_BUCKET }}
  AWS_REGION: us-east-1

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
          aws-region: ${{ env.AWS_REGION }}

      # [INSERT LANGUAGE-SPECIFIC BUILD STEPS HERE]

      - name: Generate version
        id: version
        run: |
          VERSION="${{ github.run_number }}.${GITHUB_SHA:0:6}"
          echo "version=$VERSION" >> $GITHUB_OUTPUT
          echo "Version: $VERSION"

      - name: Get repository name
        id: repo
        run: |
          REPO_NAME=$(basename $(git rev-parse --show-toplevel))
          echo "name=$REPO_NAME" >> $GITHUB_OUTPUT
          echo "Repository: $REPO_NAME"

      - name: Generate CloudFormation parameters
        run: |
          BRANCH_NAME=${GITHUB_REF#refs/heads/}
          S3_PREFIX="${{ steps.repo.outputs.name }}/${BRANCH_NAME}/${{ steps.version.outputs.version }}"

          cat > cloudformation-params.json <<EOF
          {
            "Parameters": {
              "Version": "${{ steps.version.outputs.version }}",
              "S3Bucket": "${{ env.S3_ARTIFACT_BUCKET }}",
              "S3Prefix": "${S3_PREFIX}/"
            }
          }
          EOF

          cat cloudformation-params.json

      - name: Upload artifacts to S3
        run: |
          BRANCH_NAME=${GITHUB_REF#refs/heads/}
          S3_PATH="s3://${{ env.S3_ARTIFACT_BUCKET }}/${{ steps.repo.outputs.name }}/${BRANCH_NAME}/${{ steps.version.outputs.version }}"

          # Upload application artifacts (all .zip files)
          aws s3 cp app.zip "$S3_PATH/" --region ${{ env.AWS_REGION }}

          # Upload CloudFormation template
          aws s3 cp cloudformation.template "$S3_PATH/" --region ${{ env.AWS_REGION }}

          # Upload CloudFormation parameters (this triggers deployment)
          aws s3 cp cloudformation-params.json "$S3_PATH/" --region ${{ env.AWS_REGION }}

          echo "Deployed to: $S3_PATH"
```

**For single-account mode**, add the `Env` parameter to cloudformation-params.json:

```yaml
- name: Generate CloudFormation parameters
  run: |
    BRANCH_NAME=${GITHUB_REF#refs/heads/}
    S3_PREFIX="${{ steps.repo.outputs.name }}/${BRANCH_NAME}/${{ steps.version.outputs.version }}"

    # Determine environment from branch
    if [ "$BRANCH_NAME" = "main" ]; then
      ENV="prod"
    elif [ "$BRANCH_NAME" = "staging" ]; then
      ENV="staging"
    else
      ENV="dev"
    fi

    cat > cloudformation-params.json <<EOF
    {
      "Parameters": {
        "Env": "$ENV",
        "Version": "${{ steps.version.outputs.version }}",
        "S3Bucket": "${{ env.S3_ARTIFACT_BUCKET }}",
        "S3Prefix": "${S3_PREFIX}/"
      }
    }
    EOF

    cat cloudformation-params.json
```

## Step 6: AWS IAM Setup Instructions

Inform the user they need to set up AWS IAM roles for GitHub Actions OIDC authentication:

```bash
# Run from the aws-deployer repository root:
go run ./cmd/aws-deployer github \
  --repo OWNER/REPOSITORY \
  --bucket S3_ARTIFACT_BUCKET \
  --github-token-secret github/pat-token
```

Replace:
- `OWNER/REPOSITORY` with their GitHub repo (e.g., "myorg/myapp")
- `S3_ARTIFACT_BUCKET` with their S3 bucket name
- `github/pat-token` with the Secrets Manager secret containing their GitHub PAT

This command will:
1. Create an IAM role for GitHub OIDC authentication
2. Grant the role permissions to upload to S3
3. Output the role ARN to add as `AWS_ROLE_ARN` GitHub secret

## Step 7: Configure GitHub Secrets

Instruct the user to add these secrets to their GitHub repository (Settings → Secrets and variables → Actions):

**Required:**
- `AWS_ROLE_ARN`: The IAM role ARN from the `aws-deployer github` command
- `S3_ARTIFACT_BUCKET`: The S3 bucket name for artifacts

**Optional:**
- `AWS_REGION`: Override default region (defaults to us-east-1)

## Step 8: Environment-Specific Parameters (Optional, Single-Account Only)

For single-account deployments, explain that users can create environment-specific parameter files:

Create `cloudformation-params.dev.json`, `cloudformation-params.staging.json`, or `cloudformation-params.prod.json` to override base parameters for specific environments.

These files are merged with the base `cloudformation-params.json`, allowing environment-specific customization while keeping the workflow simple.

## Summary

Once complete, summarize what was created/modified:
1. ✅ CloudFormation template (`cloudformation.template`)
2. ✅ GitHub workflow (`.github/workflows/deploy.yml`)
3. ✅ Deployment mode: [single-account/multi-account]
4. ✅ Build steps for: [detected language]

**Next steps for the user:**
1. Run `aws-deployer github` command to create IAM roles
2. Add GitHub secrets (`AWS_ROLE_ARN`, `S3_ARTIFACT_BUCKET`)
3. Push code to trigger the workflow
4. Monitor deployment in AWS Deployer console

## Important Notes

- The version format is `{github.run_number}.{first 6 chars of commit SHA}`
- S3 path structure: `{repo-basename}/{branch}/{version}/`
- Uploading `cloudformation-params.json` triggers the AWS Deployer
- All `.zip` files in the build should be uploaded to S3
- The CloudFormation template must accept the standard parameters
