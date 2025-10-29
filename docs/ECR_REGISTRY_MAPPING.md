# ECR Registry to GitHub Repository Mapping

This document explains how AWS Deployer maps ECR container registries to GitHub repositories using SSM Parameter Store.

## Overview

AWS Deployer stores an allowlist of permitted ECR registries for each GitHub repository in SSM Parameter Store. During deployment, the `verify-signatures` Lambda validates that container images come from approved registries.

## SSM Parameter Structure

### Path Pattern
```
/{env}/aws-deployer/ecr-registries/{owner}/{repo}
```

### Components
- `{env}` - Environment name (e.g., `dev`, `staging`, `prod`)
- `{owner}/{repo}` - GitHub repository in format `owner/repo`

### Value Format
Comma-separated string of ECR registry names

### Examples

**SSM Paths:**
```
/dev/aws-deployer/ecr-registries/acme/webapp
/staging/aws-deployer/ecr-registries/savaki/myapp
/prod/aws-deployer/ecr-registries/foo/bar
```

**Values:**
```
"acme/webapp/api,acme/webapp/worker,acme/webapp/frontend"
```

## Naming Conventions

### GitHub Repository Format
```
{owner}/{repo}
```

**Examples:**
- `acme/webapp` → owner: `acme`, repo: `webapp`
- `savaki/myapp` → owner: `savaki`, repo: `myapp`

### ECR Registry Format
Typically: `{owner}/{repo}/{service}`

**Examples for `acme/webapp`:**
- `acme/webapp` (monolith/base)
- `acme/webapp/api` (API service)
- `acme/webapp/worker` (worker service)
- `acme/webapp/frontend` (frontend service)

## How It Works

### 1. Setup Phase (setup-ecr command)

**Command:**
```bash
aws-deployer setup-ecr \
  --repo acme/webapp \
  --ecr-registry acme/webapp/api \
  --ecr-registry acme/webapp/worker \
  --ecr-registry acme/webapp/frontend \
  --env dev
```

**Actions:**
1. Creates ECR repositories in AWS:
   - `acme/webapp/api`
   - `acme/webapp/worker`
   - `acme/webapp/frontend`

2. Stores allowlist in SSM:
   - **Path:** `/dev/aws-deployer/ecr-registries/acme/webapp`
   - **Value:** `"acme/webapp/api,acme/webapp/worker,acme/webapp/frontend"`

3. Configures ECR with:
   - Scan on push enabled
   - Tag immutability enabled
   - Org-wide read permissions (if in AWS Organization)

**Code Location:** `cmd/aws-deployer/commands/setup_ecr.go` (function `storeRegistryAllowlistInSSM`)

### 2. Deployment Phase (verify-signatures Lambda)

**Step Function Input:**
```json
{
  "Repo": "acme/webapp",
  "Env": "dev",
  "S3Bucket": "artifacts-bucket",
  "S3Key": "acme/webapp/main/123/"
}
```

**Process:**

1. **Lambda downloads:** `s3://artifacts-bucket/acme/webapp/main/123/container-images.json`

   ```json
   {
     "images": [
       {
         "name": "api",
         "registry": "acme/webapp/api",
         "tag": "v1.0.0",
         "signed": true,
         "parameterName": "ApiImageUri"
       },
       {
         "name": "worker",
         "registry": "acme/webapp/worker",
         "tag": "v1.0.0",
         "signed": true,
         "parameterName": "WorkerImageUri"
       }
     ]
   }
   ```

2. **Reads SSM allowlist:**
   - **Query:** `/dev/aws-deployer/ecr-registries/acme/webapp`
   - **Gets:** `["acme/webapp/api", "acme/webapp/worker", "acme/webapp/frontend"]`

3. **Validates registries:**
   - Checks: `"acme/webapp/api"` ∈ allowlist? ✓
   - Checks: `"acme/webapp/worker"` ∈ allowlist? ✓

4. **Builds full ECR URIs:**
   ```
   123456789012.dkr.ecr.us-east-1.amazonaws.com/acme/webapp/api@sha256:abc123...
   123456789012.dkr.ecr.us-east-1.amazonaws.com/acme/webapp/worker@sha256:def456...
   ```

5. **Verifies signatures** (if code signing enabled)

6. **Passes URIs to CloudFormation** as parameters

**Code Locations:**
- `internal/lambda/step-functions/verify-signatures/main.go` (function `getAllowedRegistries`)
- `internal/services/container_metadata.go` (function `ValidateRegistries`)

## Validation Logic

### Exact String Matching

The validation uses **exact string matching** - no wildcards or prefix matching.

**Examples:**
- ✓ `"acme/webapp/api"` matches `"acme/webapp/api"`
- ✗ `"acme/webapp/api"` does NOT match `"acme/webapp"`
- ✗ `"acme/webapp/other"` fails if not in allowlist

### Code
```go
// Build map for O(1) lookup
allowed := make(map[string]bool)
for _, registry := range allowedRegistries {
    allowed[registry] = true
}

// Check each image's registry
for _, image := range metadata.Images {
    if !allowed[image.Registry] {
        return fmt.Errorf("invalid registries found: %v (allowed: %v)",
            invalidRegistries, allowedRegistries)
    }
}
```

## Environment Isolation

**Key Feature:** Each environment has independent configuration

```
/dev/aws-deployer/ecr-registries/acme/webapp     → "acme/webapp/api,acme/webapp/worker"
/staging/aws-deployer/ecr-registries/acme/webapp → "acme/webapp/api,acme/webapp/worker,acme/webapp/frontend"
/prod/aws-deployer/ecr-registries/acme/webapp    → "acme/webapp/api,acme/webapp/worker"
```

Each environment can have:
- Different allowed registries
- Different enforcement policies
- Independent signature verification settings

## Enforcement Modes

Configured via: `/{env}/aws-deployer/signing/enforcement-mode`

### Warn Mode
- Logs warnings for unregistered or unsigned images
- Allows deployment to proceed
- Good for migration/testing

### Enforce Mode
- Blocks deployment if registry not in allowlist
- Blocks deployment if signature verification fails
- Production-ready security

## Error Scenarios

### 1. Missing SSM Parameter

**Situation:** SSM parameter doesn't exist

**Impact:** Deployment fails with error:
```
Failed to get allowed registries from SSM: ParameterNotFound
```

**Solution:** Run `setup-ecr` for the repository and environment

### 2. Registry Not in Allowlist

**Situation:** Container uses registry not in SSM allowlist

**Impact:**
- **Warn mode:** Logs warning, allows deployment
- **Enforce mode:** Blocks deployment with error:
  ```
  invalid registries found: [acme/webapp/unauthorized] (allowed: [acme/webapp/api, acme/webapp/worker])
  ```

**Solution:** Either:
- Add registry via `setup-ecr --ecr-registry acme/webapp/unauthorized`
- Or use an allowed registry

### 3. Cross-Environment Access

**Situation:** Trying to use dev registries in prod

**Impact:** Each environment has separate allowlists - dev registries won't be in prod allowlist

**Solution:** Run `setup-ecr --env prod` to configure prod environment

### 4. Registry Name Mismatch

**Situation:** ECR registry name doesn't exactly match allowlist entry

**Example:**
- Allowlist: `["acme/webapp"]`
- Container uses: `"acme/webapp/backend"`
- Result: **VALIDATION FAILS** ✗

**Solution:** Must explicitly list all registry variants:
```bash
aws-deployer setup-ecr \
  --repo acme/webapp \
  --ecr-registry acme/webapp \
  --ecr-registry acme/webapp/backend
```

## Complete Example

### Setup

```bash
# Create ECR registries and store allowlist
aws-deployer setup-ecr \
  --repo acme/webapp \
  --ecr-registry acme/webapp/frontend \
  --ecr-registry acme/webapp/backend \
  --env dev \
  --region us-east-1
```

**Result in SSM:**
```
Path:  /dev/aws-deployer/ecr-registries/acme/webapp
Value: "acme/webapp/frontend,acme/webapp/backend"
Description: "Allowed ECR registries for acme/webapp in dev environment (for signature verification)"
```

### Deployment

**GitHub Actions uploads to S3:**
```
s3://artifacts-bucket/acme/webapp/main/42/container-images.json
s3://artifacts-bucket/acme/webapp/main/42/cloudformation.template
s3://artifacts-bucket/acme/webapp/main/42/cloudformation-params.json  # Triggers deployment
```

**container-images.json:**
```json
{
  "images": [
    {
      "name": "frontend",
      "registry": "acme/webapp/frontend",
      "tag": "v1.0.0",
      "digest": "sha256:abc123...",
      "signed": true,
      "parameterName": "FrontendImageUri"
    },
    {
      "name": "backend",
      "registry": "acme/webapp/backend",
      "tag": "v1.0.0",
      "digest": "sha256:def456...",
      "signed": true,
      "parameterName": "BackendImageUri"
    }
  ]
}
```

**Verification Flow:**
1. S3 trigger → DynamoDB → Step Function starts
2. Step Function calls `verify-signatures` Lambda
3. Lambda reads SSM: `/dev/aws-deployer/ecr-registries/acme/webapp`
4. Gets allowlist: `["acme/webapp/frontend", "acme/webapp/backend"]`
5. Validates: `"acme/webapp/frontend"` ✓, `"acme/webapp/backend"` ✓
6. Builds full URIs: `123456.dkr.ecr.us-east-1.amazonaws.com/acme/webapp/frontend:v1.0.0`
7. Verifies signatures (if enabled)
8. Passes parameters to CloudFormation:
   - `FrontendImageUri=123456.dkr.ecr.us-east-1.amazonaws.com/acme/webapp/frontend@sha256:abc123...`
   - `BackendImageUri=123456.dkr.ecr.us-east-1.amazonaws.com/acme/webapp/backend@sha256:def456...`
9. CloudFormation deploys with validated, signed images

## Verification Commands

### Check SSM Parameter

```bash
aws ssm get-parameter \
  --name "/dev/aws-deployer/ecr-registries/acme/webapp" \
  --query 'Parameter.Value' \
  --output text
```

### List All Registry Allowlists for Environment

```bash
aws ssm get-parameters-by-path \
  --path "/dev/aws-deployer/ecr-registries/" \
  --recursive
```

### Check ECR Repository Exists

```bash
aws ecr describe-repositories \
  --repository-names acme/webapp/api
```

## Best Practices

1. **Run setup-ecr for each environment:**
   ```bash
   aws-deployer setup-ecr --repo acme/webapp --ecr-registry acme/webapp/api --env dev
   aws-deployer setup-ecr --repo acme/webapp --ecr-registry acme/webapp/api --env staging
   aws-deployer setup-ecr --repo acme/webapp --ecr-registry acme/webapp/api --env prod
   ```

2. **Use consistent naming conventions:**
   - Follow `{owner}/{repo}/{service}` pattern
   - Keep service names descriptive (api, worker, frontend, etc.)

3. **List all registry variants explicitly:**
   - Don't assume wildcards - list every registry name
   - Include base registry if used: `--ecr-registry acme/webapp`

4. **Start with warn mode:**
   ```bash
   aws-deployer setup-signing --env dev --enforcement-mode warn
   ```

5. **Monitor CloudWatch logs:**
   - Check for validation warnings
   - Verify signatures are being checked
   - Look for unauthorized registry attempts

6. **Update allowlists when adding services:**
   ```bash
   # Adding a new service
   aws-deployer setup-ecr \
     --repo acme/webapp \
     --ecr-registry acme/webapp/new-service \
     --env dev
   ```

## Related Documentation

- [Code Signing Setup](CODE_SIGNING.md)
- [GitHub Actions Integration](GITHUB_ACTIONS_SIGNING.md)
- [Multi-Account Deployments](MULTI.md)
- [Installation Guide](../INSTALL.md)
