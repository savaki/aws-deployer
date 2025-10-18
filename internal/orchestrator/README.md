# Orchestrator

Step Functions orchestration layer for AWS Deployer.

## Overview

The `orchestrator` package encapsulates the logic for starting and managing AWS Step Functions executions for
CloudFormation deployments. It provides a clean interface for triggering deployments and tracking their execution state.

## Key Responsibilities

1. **Step Function Execution**: Start Step Functions executions with properly formatted input
2. **Status Management**: Atomically update build status to IN_PROGRESS when execution starts
3. **Execution Tracking**: Record Step Functions execution ARN in DynamoDB for traceability

## Architecture

The orchestrator acts as a bridge between DynamoDB stream triggers and Step Functions:

```
DynamoDB Stream → trigger-build → Orchestrator → Step Functions
                                       ↓
                               Update build status (IN_PROGRESS)
                               Record execution ARN
```

## Types

### StepFunctionInput

Represents the input payload for Step Functions executions:

```go
type StepFunctionInput struct {
    Repo       string `json:"repo"`        // Repository name
    Env        string `json:"env"`         // Environment (dev, stg, prd)
    Branch     string `json:"branch"`      // Git branch
    Version    string `json:"version"`     // Version string
    SK         string `json:"sk"`          // KSUID - build identifier
    CommitHash string `json:"commit_hash"` // Git commit hash
    S3Bucket   string `json:"s3_bucket"`   // S3 bucket containing artifacts
    S3Key      string `json:"s3_key"`      // S3 key prefix for artifacts
}
```

### Orchestrator

Main struct for orchestrating Step Functions executions:

```go
type Orchestrator struct {
    sfnClient       *sfn.Client
    stateMachineArn string
    dao             *builddao.DAO
}
```

## Usage

### Creating an Orchestrator

```go
import (
    "github.com/aws/aws-sdk-go-v2/service/sfn"
    "github.com/savaki/aws-deployer/internal/dao/builddao"
    "github.com/savaki/aws-deployer/internal/orchestrator"
)

// Initialize dependencies
cfg, _ := config.LoadDefaultConfig(ctx)
sfnClient := sfn.NewFromConfig(cfg)
dao := builddao.New(dynamoClient, tableName)

// Create orchestrator
orch := orchestrator.New(
    sfnClient,
    "arn:aws:states:us-east-1:123456789:stateMachine:deployment",
    dao,
)
```

### Starting an Execution

```go
input := orchestrator.StepFunctionInput{
    Repo:       "myapp",
    Env:        "prd",
    Branch:     "main",
    Version:    "1.2.3",
    SK:         "2HFj3kLmNoPqRsTuVwXy",
    CommitHash: "abc123",
    S3Bucket:   "my-artifacts-bucket",
    S3Key:      "myapp/main/1.2.3",
}

executionArn, err := orch.StartExecution(ctx, input)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Started execution: %s\n", executionArn)
```

## Execution Flow

When `StartExecution` is called:

1. **Marshal Input**: Converts `StepFunctionInput` to JSON
2. **Generate Name**: Creates execution name: `{repo}-{env}-{sk}`
3. **Start Execution**: Calls AWS Step Functions `StartExecution` API
4. **Update Status**: Atomically updates build record:
    - Sets `Status` to `IN_PROGRESS`
    - Records `ExecutionArn`
    - Updates `UpdatedAt` timestamp
5. **Return ARN**: Returns the execution ARN for tracking

## Error Handling

If the Step Functions execution fails to start, the build status is **not** automatically marked as FAILED. The
trigger-build Lambda is responsible for catching errors and updating the build status accordingly.

## Status Progression

The orchestrator is responsible for the PENDING → IN_PROGRESS transition:

- **PENDING**: Build record created (s3-trigger)
- **IN_PROGRESS**: Step Function execution started (orchestrator)
- **SUCCESS/FAILED**: Step Function completion (handled by Step Functions state machine)

## Integration Points

### Used By

- `internal/lambda/trigger-build`: DynamoDB stream trigger Lambda
- Future: Direct API endpoints for manual deployments

### Dependencies

- `internal/dao/builddao`: For atomically updating build status
- AWS SDK Step Functions client: For starting executions

## Testing

To test the orchestrator, you'll need:

1. AWS credentials with Step Functions permissions
2. A deployed Step Functions state machine
3. DynamoDB Local for DAO operations

Example test:

```go
func TestOrchestrator_StartExecution(t *testing.T) {
    // Setup mocks/local resources
    sfnClient := mockSFNClient()
    dao := builddao.New(localDynamoClient, "test-table")
    orch := orchestrator.New(sfnClient, "test-arn", dao)

    input := orchestrator.StepFunctionInput{
        Repo: "test",
        Env:  "dev",
        SK:   ksuid.New().String(),
        // ... other fields
    }

    executionArn, err := orch.StartExecution(ctx, input)
    assert.NoError(t, err)
    assert.NotEmpty(t, executionArn)
}
```

## Design Decisions

### Why Atomic Status Update?

The `StartExecution` method atomically updates the build status and execution ARN in a single DynamoDB operation. This
ensures:

- Build status always reflects whether an execution has started
- No race conditions between execution start and status update
- Execution ARN is immediately available for tracking

### Why Not Return the Full Build Record?

`StartExecution` returns only the execution ARN, not the updated build record. This keeps the interface simple and
focused. Callers can query the build record separately if needed.

### Execution Name Format

Execution names follow the pattern: `{repo}-{env}-{sk}`

- **Unique**: KSUID ensures uniqueness
- **Descriptive**: Easy to identify in AWS Console
- **Traceable**: Maps directly to build record
