# Multi-Account AWS Deployer Installation Guide

This guide walks you through setting up AWS Deployer in multi-account mode using CloudFormation StackSets to deploy applications across multiple AWS accounts from a central deployer account.

## Overview

In multi-account mode, AWS Deployer:
- Runs in a **deployer account** (management/central account)
- Deploys CloudFormation stacks to **target accounts** via StackSets
- Manages deployment state and targets in DynamoDB
- Provides a web console for managing deployments

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Deployer Account (111111111111)                             │
│                                                              │
│ ├─ aws-deployer CloudFormation Stack (multi mode)           │
│ │  ├─ DynamoDB: Builds, Targets, Deployments, Locks        │
│ │  ├─ Step Functions: Multi-account state machine          │
│ │  ├─ Lambda: Trigger, orchestration, console server       │
│ │  └─ API Gateway: Web console                             │
│ │                                                           │
│ ├─ S3 Artifacts Bucket                                      │
│ │  └─ Org-wide read policy                                 │
│ │                                                           │
│ └─ IAM: AWSCloudFormationStackSetAdministrationRole        │
└─────────────────────────────────────────────────────────────┘
                        │
                        │ AssumeRole
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Target Account 1 (222222222222)                             │
│ └─ IAM: AWSCloudFormationStackSetExecutionRole             │
│    └─ Trust: Deployer account administration role          │
└─────────────────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────────────────┐
│ Target Account 2 (333333333333)                             │
│ └─ IAM: AWSCloudFormationStackSetExecutionRole             │
│    └─ Trust: Deployer account administration role          │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

### 1. AWS Organization Setup
- AWS Organization configured with a management/deployer account
- Organization ID (required for S3 bucket policy)
- Target accounts in the organization

### 2. Deployer Account Requirements
- AWS credentials with CloudFormation, IAM, DynamoDB, S3, Lambda, API Gateway permissions
- S3 bucket for Lambda artifacts: `s3://your-artifacts-bucket/`
- Deployer account ID (e.g., `111111111111`)

### 3. Target Account Requirements
- AWS credentials for each target account (IAM admin permissions)
- Target account IDs (e.g., `222222222222`, `333333333333`)

### 4. OAuth Provider Setup
Choose one:
- **Google Cloud Identity Platform (CIAM)**
  - GCP project with Identity Platform enabled
  - OAuth 2.0 client credentials
- **Auth0**
  - Auth0 tenant and application configured
  - OAuth 2.0 client credentials

### 5. Tools Required
- AWS CLI configured
- Go 1.21+ (for building setup CLI)
- Make (for deployment)

---

## Step 1: Configure S3 Bucket for Organization-Wide Access

The artifacts S3 bucket must allow all accounts in your organization to read CloudFormation templates.

### Create S3 Bucket Policy

```bash
# Get your organization ID
aws organizations describe-organization --query 'Organization.Id' --output text

# Example output: o-xxxxxxxxxx
```

Create a bucket policy file `s3-bucket-policy.json`:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowOrganizationReadAccess",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:GetObjectVersion"
      ],
      "Resource": "arn:aws:s3:::your-artifacts-bucket/*",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalOrgID": "o-xxxxxxxxxx"
        }
      }
    }
  ]
}
```

Apply the policy:

```bash
aws s3api put-bucket-policy \
  --bucket your-artifacts-bucket \
  --policy file://s3-bucket-policy.json
```

---

## Step 2: Set Up IAM Roles in Target Accounts

Each target account needs a StackSet execution role that trusts the deployer account.

### Build the Setup CLI

```bash
cd cmd/aws-deployer
go build -o aws-deployer .
```

### Configure Target Accounts

Run the setup command for each target account. You must use credentials for the target account.

```bash
# Configure AWS credentials for target account
export AWS_PROFILE=target-account-1  # or use AWS_ACCESS_KEY_ID/SECRET

# Run setup
./aws-deployer setup-aws setup-target \
  --deployer-account 111111111111 \
  --region us-east-1

# Output:
# Creating execution role in account 222222222222...
# Created role: AWSCloudFormationStackSetExecutionRole
# Attaching AdministratorAccess policy...
# ✓ Setup complete for account 222222222222
#   Role ARN: arn:aws:iam::222222222222:role/AWSCloudFormationStackSetExecutionRole
#   Trusted by: Deployer account 111111111111
```

Repeat for all target accounts:

```bash
# Target account 2
export AWS_PROFILE=target-account-2
./aws-deployer setup-aws setup-target --deployer-account 111111111111

# Target account 3
export AWS_PROFILE=target-account-3
./aws-deployer setup-aws setup-target --deployer-account 111111111111
```

### Verify Setup

Switch back to deployer account credentials and verify cross-account access:

```bash
export AWS_PROFILE=deployer-account

./aws-deployer setup-aws verify \
  --deployer-account 111111111111 \
  --target-account 222222222222

# Output:
# Verifying role setup...
#   Deployer account: 111111111111
#   Target account: 222222222222
#   Role: AWSCloudFormationStackSetExecutionRole
# ✓ Verification successful
#   Deployer account 111111111111 can assume role in target account 222222222222
```

---

## Step 3: Create OAuth Secrets in AWS Secrets Manager

AWS Deployer requires OAuth credentials for the web console authentication.

### For Google CIAM

```bash
aws secretsmanager create-secret \
  --name aws-deployer/dev/secrets \
  --description "OAuth configuration for AWS Deployer" \
  --secret-string '{
    "provider": "google-ciam",
    "client_id": "your-client-id.apps.googleusercontent.com",
    "client_secret": "GOCSPX-xxxxxxxxxxxxx",
    "project_id": "your-gcp-project-id"
  }'
```

### For Auth0

```bash
aws secretsmanager create-secret \
  --name aws-deployer/dev/secrets \
  --description "OAuth configuration for AWS Deployer" \
  --secret-string '{
    "provider": "auth0",
    "client_id": "your-auth0-client-id",
    "client_secret": "your-auth0-client-secret",
    "domain": "your-tenant.us.auth0.com"
  }'
```

**Note**: Replace `dev` with your environment name if different.

---

## Step 4: Build and Upload Lambda Functions

Build all Lambda functions and upload to S3:

```bash
# From project root
make build

# Upload to S3 (uses parallel sync)
S3_BUCKET=your-artifacts-bucket make upload-to-s3

# Output:
# Uploading Lambda packages to S3...
# Version: 20250113-143022
# upload: build/s3-trigger.zip to s3://your-artifacts-bucket/aws-deployer/20250113-143022/s3-trigger.zip
# ... (15 files uploaded in parallel)
# Upload completed!
```

Save the VERSION for the next step:

```bash
# Show the version
make show-version

# Output: Current version: 20250113-143022
```

---

## Step 5: Deploy CloudFormation Stack in Multi-Account Mode

Deploy the aws-deployer infrastructure with `DEPLOYMENT_MODE=multi`:

```bash
make deploy-infrastructure \
  ENVIRONMENT=dev \
  S3_BUCKET=your-artifacts-bucket \
  VERSION=20250113-143022 \
  DEPLOYMENT_MODE=multi \
  AWS_REGION=us-east-1
```

### Optional Parameters

```bash
# With custom domain (requires ACM certificate)
make deploy-infrastructure \
  ENVIRONMENT=dev \
  S3_BUCKET=your-artifacts-bucket \
  VERSION=20250113-143022 \
  DEPLOYMENT_MODE=multi \
  ZONE_ID=Z1234567890ABC \
  DOMAIN_NAME=deployer.example.com \
  CERTIFICATE_ARN=arn:aws:acm:us-east-1:111111111111:certificate/xxxxx \
  AWS_REGION=us-east-1

# With email authorization (restrict to specific email)
make deploy-infrastructure \
  ENVIRONMENT=dev \
  S3_BUCKET=your-artifacts-bucket \
  VERSION=20250113-143022 \
  DEPLOYMENT_MODE=multi \
  ALLOWED_EMAIL=admin@example.com \
  AWS_REGION=us-east-1

# With custom session token rotation (days)
make deploy-infrastructure \
  ENVIRONMENT=dev \
  S3_BUCKET=your-artifacts-bucket \
  VERSION=20250113-143022 \
  DEPLOYMENT_MODE=multi \
  ROTATION_SCHEDULE_DAYS=7 \
  AWS_REGION=us-east-1
```

### Deployment creates:

1. **DynamoDB Tables**:
   - `{env}-aws-deployer-builds` - Build tracking
   - `{env}-aws-deployer--targets` - Multi-account target configuration
   - `{env}-aws-deployer--deployments` - Deployment state per account
   - `{env}-aws-deployer--locks` - Deployment locks

2. **Step Functions**:
   - `{env}-aws-deployer-deployment` - Single-account state machine (unused in multi mode)
   - `{env}-aws-deployer-multi-account-deployment` - Multi-account orchestration

3. **Lambda Functions** (15 total):
   - Trigger functions, multi-account orchestrators, web server, token rotator

4. **API Gateway**:
   - HTTP API for web console

5. **IAM Roles**:
   - `AWSCloudFormationStackSetAdministrationRole` - Manages StackSets

6. **Secrets Manager**:
   - `aws-deployer/{env}/session-token` - Auto-rotating session keys

---

## Step 6: Configure Deployment Targets

Deployment targets define which accounts/regions to deploy to for each repo/environment.

### Target Record Structure

```json
{
  "pk": "repo-name/env-name",
  "targets": [
    {
      "account_id": "222222222222",
      "regions": ["us-east-1", "us-west-2"]
    },
    {
      "account_id": "333333333333",
      "regions": ["eu-west-1"]
    }
  ]
}
```

### Add Targets via AWS CLI

```bash
# Add targets for myapp/dev
aws dynamodb put-item \
  --table-name dev-aws-deployer--targets \
  --item '{
    "pk": {"S": "myapp/dev"},
    "targets": {"L": [
      {"M": {
        "account_id": {"S": "222222222222"},
        "regions": {"L": [{"S": "us-east-1"}]}
      }}
    ]}
  }'

# Add targets for myapp/prod (multi-region, multi-account)
aws dynamodb put-item \
  --table-name dev-aws-deployer--targets \
  --item '{
    "pk": {"S": "myapp/prod"},
    "targets": {"L": [
      {"M": {
        "account_id": {"S": "222222222222"},
        "regions": {"L": [{"S": "us-east-1"}, {"S": "us-west-2"}]}
      }},
      {"M": {
        "account_id": {"S": "333333333333"},
        "regions": {"L": [{"S": "eu-west-1"}]}
      }}
    ]}
  }'
```

### Add Targets via Web Console (Future)

The web console will provide a UI for managing targets (currently requires DynamoDB direct access).

---

## Step 7: Configure S3 Notification

Configure the S3 bucket to trigger deployments when new builds are uploaded:

```bash
make configure-s3-notification \
  ENVIRONMENT=dev \
  S3_BUCKET=your-artifacts-bucket \
  AWS_REGION=us-east-1
```

This creates an S3 event notification that triggers the `s3-trigger` Lambda when new objects are uploaded.

---

## Step 8: Test the Setup

### 1. Access the Web Console

Get the API Gateway URL:

```bash
aws cloudformation describe-stacks \
  --stack-name dev-aws-deployer \
  --query 'Stacks[0].Outputs[?OutputKey==`ServerAPIURL`].OutputValue' \
  --output text
```

Open the URL in your browser. You should see the OAuth login flow.

### 2. Trigger a Test Deployment

Upload a test CloudFormation template and trigger a deployment:

```bash
# Create test files
mkdir -p /tmp/test-build
cat > /tmp/test-build/cloudformation.template <<'EOF'
AWSTemplateFormatVersion: '2010-09-09'
Description: Test deployment
Resources:
  TestBucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Sub 'test-${AWS::AccountId}-${AWS::Region}'
EOF

# Upload to S3 (this triggers deployment)
aws s3 cp /tmp/test-build/cloudformation.template \
  s3://your-artifacts-bucket/test-app/main/1.0.0/cloudformation.template
```

### 3. Monitor Deployment

Watch the Step Functions execution:

```bash
# List executions
aws stepfunctions list-executions \
  --state-machine-arn $(aws cloudformation describe-stacks \
    --stack-name dev-aws-deployer \
    --query 'Stacks[0].Outputs[?OutputKey==`MultiAccountStateMachineArn`].OutputValue' \
    --output text)

# Get execution details
aws stepfunctions describe-execution \
  --execution-arn <execution-arn-from-above>
```

View Lambda logs:

```bash
# Trigger build Lambda logs
aws logs tail /aws/lambda/dev-aws-deployer-trigger-build --follow

# Multi-account state machine Lambda logs
aws logs tail /aws/lambda/dev-aws-deployer-fetch-targets --follow
```

Check DynamoDB for build status:

```bash
aws dynamodb scan \
  --table-name dev-aws-deployer-builds \
  --filter-expression "repo = :repo" \
  --expression-attribute-values '{":repo":{"S":"test-app"}}'
```

---

## Post-Installation Configuration

### Update Lambda Code (After Changes)

```bash
# Rebuild and upload
make build
S3_BUCKET=your-artifacts-bucket VERSION=$(cat .version) make upload-to-s3

# Update all Lambda functions
make update-lambda-code \
  ENVIRONMENT=dev \
  S3_BUCKET=your-artifacts-bucket \
  VERSION=$(cat .version)
```

### Add More Target Accounts

```bash
# Setup IAM role in new account
export AWS_PROFILE=new-target-account
cd cmd/aws-deployer-setup
./aws-deployer setup-aws setup-target --deployer-account 111111111111

# Verify from deployer account
export AWS_PROFILE=deployer-account
./aws-deployer setup-aws verify \
  --deployer-account 111111111111 \
  --target-account 444444444444

# Add to targets table
aws dynamodb update-item \
  --table-name dev-aws-deployer--targets \
  --key '{"pk":{"S":"myapp/prod"}}' \
  --update-expression "SET targets = list_append(targets, :new_target)" \
  --expression-attribute-values '{
    ":new_target": {"L": [
      {"M": {
        "account_id": {"S": "444444444444"},
        "regions": {"L": [{"S": "us-east-1"}]}
      }}
    ]}
  }'
```

### Update OAuth Configuration

```bash
aws secretsmanager update-secret \
  --secret-id aws-deployer/dev/secrets \
  --secret-string '{
    "provider": "google-ciam",
    "client_id": "new-client-id.apps.googleusercontent.com",
    "client_secret": "new-secret",
    "project_id": "your-gcp-project-id"
  }'
```

### Rotate Session Tokens Manually

Session tokens rotate automatically based on `ROTATION_SCHEDULE_DAYS`. To trigger manually:

```bash
aws secretsmanager rotate-secret \
  --secret-id aws-deployer/dev/session-token
```

---

## Troubleshooting

### Deployment fails with "Access Denied" in target account

**Cause**: StackSet execution role not configured or trust policy incorrect.

**Solution**:
1. Verify role exists:
   ```bash
   aws iam get-role --role-name AWSCloudFormationStackSetExecutionRole --profile target-account
   ```
2. Re-run setup:
   ```bash
   ./aws-deployer setup-aws setup-target --deployer-account 111111111111
   ```
3. Verify from deployer account:
   ```bash
   ./aws-deployer setup-aws verify --deployer-account 111111111111 --target-account 222222222222
   ```

### Template download fails: "Access Denied" on S3

**Cause**: S3 bucket policy doesn't allow organization-wide read access.

**Solution**:
1. Check bucket policy includes organization condition:
   ```bash
   aws s3api get-bucket-policy --bucket your-artifacts-bucket
   ```
2. Verify organization ID is correct
3. Re-apply bucket policy from Step 1

### OAuth login fails / redirect loop

**Cause**: Session token secret not initialized or OAuth config incorrect.

**Solution**:
1. Check secret exists:
   ```bash
   aws secretsmanager describe-secret --secret-id aws-deployer/dev/secrets
   aws secretsmanager describe-secret --secret-id aws-deployer/dev/session-token
   ```
2. Verify OAuth callback URL matches configured redirect URI in provider
3. Trigger rotation:
   ```bash
   aws secretsmanager rotate-secret --secret-id aws-deployer/dev/session-token
   ```
4. Check Lambda logs:
   ```bash
   aws logs tail /aws/lambda/dev-aws-deployer-server --follow
   ```

### Single-account state machine triggered in multi mode

**Cause**: This was a bug, fixed in the latest version.

**Solution**: Redeploy with latest code. Verify `DEPLOYMENT_MODE=multi` is set:
```bash
aws lambda get-function-configuration \
  --function-name dev-aws-deployer-trigger-build \
  --query 'Environment.Variables.DEPLOYMENT_MODE'
```

### No targets found, deployment fails

**Cause**: Targets not configured in DynamoDB for this repo/env combination.

**Solution**: Add targets to the `--targets` table (see Step 6).

### StackSet operation stuck "IN_PROGRESS"

**Cause**: CloudFormation stack in target account is in a failed state.

**Solution**:
1. Check StackSet operation status:
   ```bash
   aws cloudformation describe-stack-set-operation \
     --stack-set-name <stack-set-name> \
     --operation-id <operation-id>
   ```
2. View failed stack instances:
   ```bash
   aws cloudformation list-stack-instances \
     --stack-set-name <stack-set-name> \
     --filters "[{\"Name\":\"Status\",\"Values\":\"FAILED\"}]"
   ```
3. Manually delete failed stack in target account if needed
4. Retry deployment

---

## Security Best Practices

### 1. Scope Down IAM Permissions

The default execution role uses `AdministratorAccess`. For production:

```bash
# Create custom policy with minimal permissions
aws iam create-policy \
  --policy-name CloudFormationDeploymentPolicy \
  --policy-document file://minimal-cfn-policy.json

# Update execution role
aws iam detach-role-policy \
  --role-name AWSCloudFormationStackSetExecutionRole \
  --policy-arn arn:aws:iam::aws:policy/AdministratorAccess

aws iam attach-role-policy \
  --role-name AWSCloudFormationStackSetExecutionRole \
  --policy-arn arn:aws:iam::<account-id>:policy/CloudFormationDeploymentPolicy
```

### 2. Enable CloudTrail in All Accounts

Monitor cross-account activity:

```bash
aws cloudtrail create-trail \
  --name aws-deployer-audit \
  --s3-bucket-name audit-logs-bucket \
  --is-multi-region-trail \
  --is-organization-trail
```

### 3. Use Service Control Policies (SCPs)

Add guardrails at the organization level to prevent unauthorized resource creation.

### 4. Restrict OAuth Access

Use `ALLOWED_EMAIL` parameter to restrict console access:

```bash
make deploy-infrastructure \
  ALLOWED_EMAIL=admin@yourcompany.com \
  ...
```

### 5. Encrypt Secrets with KMS

Use custom KMS keys for Secrets Manager:

```bash
aws secretsmanager create-secret \
  --name aws-deployer/dev/secrets \
  --kms-key-id arn:aws:kms:us-east-1:111111111111:key/xxxxx \
  --secret-string '{...}'
```

---

## Clean Up

### Remove aws-deployer from deployer account

```bash
# Delete CloudFormation stack
aws cloudformation delete-stack --stack-name dev-aws-deployer

# Wait for deletion
aws cloudformation wait stack-delete-complete --stack-name dev-aws-deployer

# Clean up S3 artifacts manually if needed
aws s3 rm s3://your-artifacts-bucket/aws-deployer/ --recursive
```

### Remove execution roles from target accounts

```bash
# For each target account
export AWS_PROFILE=target-account-1
cd cmd/aws-deployer-setup
./aws-deployer setup-aws teardown

# Repeat for all target accounts
```

---

## Next Steps

- [Application Integration Guide](INSTALL.md) - How to integrate your app with aws-deployer
- [API Documentation](README.md) - Web console API and GraphQL schema
- [Development Guide](CONTRIBUTING.md) - Contributing to aws-deployer

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/yourorg/aws-deployer/issues
- Documentation: https://github.com/yourorg/aws-deployer/wiki
