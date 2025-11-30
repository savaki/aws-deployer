# Docker Image Promotion Plan

## Overview

Add Docker image promotion capability to aws-deployer as part of multi-stage deployments. Images are promoted from a source ECR registry to target ECR registries before CloudFormation runs.

## User Requirements (from discussion)

1. **Timing**: Promotion happens BEFORE CloudFormation runs
2. **SDK**: Use AWS SDK for Go (not CLI)
3. **Cross-account**: Promote to each target account (not central registry)
4. **Metadata**: Use separate manifest file (recommended for future extensibility)

## Manifest File Design

**File**: `deploy-manifest.json` (in same S3 directory as cloudformation-params.json)

```json
{
  "images": [
    {
      "repository": "myapp/api",
      "tag": "1.0.0-abc123"
    },
    {
      "repository": "myapp/worker",
      "tag": "1.0.0-abc123"
    }
  ]
}
```

- Optional file - if not present, skip image promotion
- Supports multiple images per deployment
- Tag typically matches version/commit from build

## Architecture

### Single-Account Mode

```
S3 Upload → S3 Lambda → DynamoDB → trigger-build → Step Function
                                                        ↓
                                               [promote-images] ← NEW
                                                        ↓
                                               deploy-cloudformation
                                                        ↓
                                               check-stack-status
```

### Multi-Account Mode

```
S3 Upload → S3 Lambda → DynamoDB → trigger-build → Step Function
                                                        ↓
                                                  acquire-lock
                                                        ↓
                                                  fetch-targets
                                                        ↓
                                             [promote-images] ← NEW (per target)
                                                        ↓
                                               create-stackset
                                                        ↓
                                            deploy-stack-instances
                                                        ...
```

## Implementation Steps

### Phase 1: Core Promotion Lambda

1. **Create `internal/lambda/step-functions/promote-images/main.go`**
   - Input: StepFunctionInput + target account/region (for multi-account)
   - Read `deploy-manifest.json` from S3
   - If manifest doesn't exist, return success (no-op)
   - For each image in manifest:
     - Pull image manifest from source ECR
     - Push image manifest to target ECR
   - Use ECR batch operations where possible

2. **ECR Image Promotion Logic**
   ```go
   type ImagePromoter struct {
       sourceECR *ecr.Client
       targetECR *ecr.Client  // May be cross-account
   }

   func (p *ImagePromoter) PromoteImage(ctx context.Context, repo, tag string) error {
       // 1. Get image manifest from source
       // 2. Get authorization token for target
       // 3. Put image to target ECR
       // Uses ECR's BatchGetImage and PutImage APIs
   }
   ```

3. **Cross-Account ECR Access**
   - Source: Use Lambda's execution role
   - Target: Assume role in target account (reuse existing `EXECUTION_ROLE_NAME` pattern from StackSets)
   - Target ECR must have repository policy allowing cross-account push

### Phase 2: Step Function Integration

1. **Update `step-function-definition.json`**
   - Add `PromoteImages` state before `DeployCloudFormation`
   - Handle promotion failures (fail build)

   ```json
   {
     "PromoteImages": {
       "Type": "Task",
       "Resource": "${PromoteImagesLambdaArn}",
       "Next": "DeployCloudFormation",
       "Catch": [{
         "ErrorEquals": ["States.ALL"],
         "Next": "UpdateBuildStatusFailed"
       }]
     }
   }
   ```

2. **Update `multi-account-step-function-definition.json`**
   - Add `PromoteImages` state after `FetchTargets`
   - Use Map state to promote to all targets in parallel
   - Or: Promote during `DeployStackInstances` per-target

### Phase 3: Infrastructure Updates

1. **Update `cloudformation.template`**
   - Add `promote-images` Lambda function
   - Add IAM permissions:
     - `ecr:GetAuthorizationToken`
     - `ecr:BatchGetImage`
     - `ecr:GetDownloadUrlForLayer`
     - `ecr:PutImage`
     - `ecr:InitiateLayerUpload`
     - `ecr:UploadLayerPart`
     - `ecr:CompleteLayerUpload`
     - `ecr:BatchCheckLayerAvailability`
   - Add cross-account assume role permissions

2. **Update Lambda build**
   - Add to Makefile build targets
   - Add to `update-lambda-code` target

### Phase 4: Testing

1. **Unit tests for promote-images Lambda**
   - Mock ECR clients
   - Test manifest parsing
   - Test missing manifest (no-op)
   - Test cross-account role assumption
   - Test error handling

2. **Integration tests**
   - Requires actual ECR repositories
   - Test single image promotion
   - Test multi-image promotion
   - Test cross-account promotion

## Input/Output Contracts

### Promote Images Lambda Input

```go
type Input struct {
    Env          string `json:"env"`
    Repo         string `json:"repo"`
    SK           string `json:"sk"`
    S3Bucket     string `json:"s3_bucket"`
    S3Key        string `json:"s3_key"`
    TemplateName string `json:"template_name,omitempty"`
    BaseRepo     string `json:"base_repo,omitempty"`

    // For multi-account mode
    TargetAccount string `json:"target_account,omitempty"`
    TargetRegion  string `json:"target_region,omitempty"`
}
```

### Promote Images Lambda Output

```go
type Output struct {
    ImagesPromoted int      `json:"images_promoted"`
    Images         []string `json:"images"`  // List of promoted image URIs
    Skipped        bool     `json:"skipped"` // True if no manifest found
}
```

### Deploy Manifest Schema

```go
type DeployManifest struct {
    Images []ImageSpec `json:"images"`
}

type ImageSpec struct {
    Repository string `json:"repository"`  // ECR repo name (e.g., "myapp/api")
    Tag        string `json:"tag"`         // Image tag
    // Future: Digest string for immutable references
}
```

## Configuration

### New SSM Parameters

- `/{env}/aws-deployer/source-ecr-account` - Source ECR account ID (optional, defaults to current account)
- `/{env}/aws-deployer/source-ecr-region` - Source ECR region (optional, defaults to current region)

### Environment Variables for Lambda

- `SOURCE_ECR_ACCOUNT` - Source account for ECR images
- `SOURCE_ECR_REGION` - Source region for ECR images

## Error Handling

1. **Missing manifest**: Success (no-op, log info)
2. **Missing source image**: Fail build with clear error
3. **Target ECR permission denied**: Fail build, suggest repository policy fix
4. **Cross-account role assumption failed**: Fail build, suggest IAM fix
5. **Partial promotion failure**: Fail build (no partial success)

## Rollback Considerations

- Image promotion is idempotent (re-running promotes same images)
- No automatic rollback of promoted images on CloudFormation failure
- Images remain in target ECR (manual cleanup if needed)

## Future Enhancements (not in initial scope)

- [ ] Image signing verification before promotion
- [ ] Digest-based promotion (immutable)
- [ ] Parallel promotion across targets
- [ ] Image vulnerability scanning gate
- [ ] Cleanup of old images in target ECR

## Files to Create/Modify

### New Files
- `internal/lambda/step-functions/promote-images/main.go`
- `internal/lambda/step-functions/promote-images/main_test.go`

### Modified Files
- `cloudformation.template` - Add Lambda, IAM permissions
- `step-function-definition.json` - Add PromoteImages state
- `multi-account-step-function-definition.json` - Add PromoteImages state
- `Makefile` - Add build target
- `README.md` - Document image promotion
- `CLAUDE.md` - Document image promotion

## Open Questions

1. Should promotion happen once (to source region) or per-target in multi-account?
   - **Decision**: Per-target account to support isolated accounts

2. What happens if target ECR repo doesn't exist?
   - **Option A**: Fail with clear error (recommended - explicit setup required)
   - **Option B**: Auto-create repository

3. Should we support promoting from non-ECR sources (Docker Hub, etc.)?
   - **Decision**: ECR-only for initial implementation
