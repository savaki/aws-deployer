# Trigger Build Lambda

DynamoDB Stream trigger for starting CloudFormation deployment executions.

## Overview

The `trigger-build` Lambda function processes DynamoDB Stream events from the build tracking table. When a new build
record is created (INSERT event), it automatically starts a Step Functions execution to deploy the CloudFormation stack.

## Architecture

```
S3 Upload → s3-trigger → DynamoDB Write (PENDING)
                              ↓
                        DynamoDB Stream
                              ↓
                        trigger-build Lambda
                              ↓
                        orchestrator.StartExecution()
                              ↓
                        Step Functions (IN_PROGRESS)
```

## Responsibilities

1. **Stream Processing**: Listen to DynamoDB Stream events
2. **Event Filtering**: Process only INSERT events (ignore MODIFY, REMOVE)
3. **Execution Triggering**: Start Step Functions executions via orchestrator
4. **Error Handling**: Update build status to FAILED if execution fails to start

## Environment Variables

| Variable              | Required | Description                                          |
|-----------------------|----------|------------------------------------------------------|
| `DYNAMODB_TABLE_NAME` | Yes      | Name of the build tracking DynamoDB table            |
| `STATE_MACHINE_ARN`   | Yes      | ARN of the deployment Step Functions state machine   |
| `S3_BUCKET_NAME`      | Yes      | S3 bucket containing build artifacts                 |
| `ENV`                 | No       | Environment name (dev, stg, prd) - defaults to "dev" |
| `VERSION`             | No       | Deployment version for tracking                      |

## Event Processing

### INSERT Events

When a new build record is created:

1. Unmarshal DynamoDB `NewImage` to `builddao.Record`
2. Extract build metadata (repo, env, branch, version, sk, commitHash)
3. Construct `orchestrator.StepFunctionInput`
4. Call `orchestrator.StartExecution()` to:
    - Start Step Functions execution
    - Update status to IN_PROGRESS
    - Record execution ARN
5. Log success with execution ARN

### MODIFY/REMOVE Events

These events are ignored with an info log message.

## Error Handling

If `orchestrator.StartExecution()` fails:

1. Log error details
2. Update build status to FAILED
3. Record error message in build record
4. Return error to Lambda (triggers retry if configured)

## Deployment

### CloudFormation

The function is deployed via `cloudformation.template`:

```yaml
TriggerBuildFunction:
  Type: AWS::Lambda::Function
  Properties:
    FunctionName: !Sub '${Environment}-aws-deployer-trigger-build'
    Runtime: provided.al2
    Handler: bootstrap
    Code:
      S3Bucket: !Ref S3BucketName
      S3Key: !Sub 'aws-deployer/${Version}/trigger-build.zip'
    Role: !GetAtt TriggerBuildLambdaRole.Arn
    Environment:
      Variables:
        DYNAMODB_TABLE_NAME: !Ref BuildsTable
        STATE_MACHINE_ARN: !Ref DeploymentStateMachine
        S3_BUCKET_NAME: !Ref S3BucketName

TriggerBuildEventSourceMapping:
  Type: AWS::Lambda::EventSourceMapping
  Properties:
    EventSourceArn: !GetAtt BuildsTable.StreamArn
    FunctionName: !Ref TriggerBuildFunction
    StartingPosition: LATEST
    BatchSize: 10
```

### IAM Permissions

The function requires:

- **DynamoDB**: Read stream, update items
- **Step Functions**: StartExecution permission
- **CloudWatch Logs**: Write logs (via AWSLambdaBasicExecutionRole)

## Building

```bash
# Build trigger-build Lambda
make build

# Or build specifically
cd internal/lambda/trigger-build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o ../../build/trigger-build/bootstrap .
cd ../../build/trigger-build && zip -r ../trigger-build.zip .
```

## Testing Locally

The function supports CLI mode for local testing:

```bash
# Run with environment variables
export DYNAMODB_TABLE_NAME=dev-aws-deployer--builds
export STATE_MACHINE_ARN=arn:aws:states:us-east-1:123:stateMachine:dev-deployer
export S3_BUCKET_NAME=my-artifacts-bucket

# Initialize handler (validates configuration)
go run internal/lambda/trigger-build/main.go \
    --state-machine-arn $STATE_MACHINE_ARN \
    --table-name $DYNAMODB_TABLE_NAME
```

## Monitoring

### CloudWatch Logs

Logs are written to `/aws/lambda/${Environment}-aws-deployer-trigger-build`

Key log messages:

- `Processing new build record`: Build being processed
- `Skipping non-INSERT event`: Filtered event type
- `Started Step Functions execution`: Success
- `Error processing DynamoDB record`: Failure

### Metrics

Monitor these CloudWatch metrics:

- `Invocations`: Number of times function is invoked
- `Errors`: Failed invocations
- `Duration`: Execution time
- `IteratorAge`: Stream processing lag

### Tail Logs

```bash
aws logs tail /aws/lambda/dev-aws-deployer-trigger-build --follow
```

## Stream Configuration

The DynamoDB Stream is configured with:

- **StreamViewType**: `NEW_AND_OLD_IMAGES`
- **BatchSize**: 10 records per invocation
- **StartingPosition**: `LATEST` (process only new records)
- **MaximumRetryAttempts**: 3
- **BisectBatchOnFunctionError**: true (isolate failing records)

## Status Flow

This function is responsible for the **PENDING → IN_PROGRESS** transition:

1. **PENDING**: Build record created by s3-trigger
2. **IN_PROGRESS**: Step Functions execution started by trigger-build
3. **SUCCESS/FAILED**: Completion handled by Step Functions state machine

## Integration Points

### Upstream

- **s3-trigger**: Creates build records with PENDING status
- **DynamoDB Streams**: Delivers INSERT events to this function

### Downstream

- **orchestrator**: Starts Step Functions executions
- **Step Functions**: Executes deployment workflow
- **DynamoDB**: Updates build status and execution ARN

## Design Decisions

### Why DynamoDB Streams?

Using DynamoDB Streams instead of direct Step Functions invocation from s3-trigger provides:

1. **Separation of Concerns**: S3 trigger only writes to DynamoDB
2. **Retry Isolation**: Stream retries don't re-create build records
3. **Audit Trail**: All build records are persisted before execution
4. **Future Extensibility**: Easy to add additional stream consumers

### Why Process Only INSERT?

- **MODIFY**: Status changes don't trigger new executions
- **REMOVE**: Build deletions don't need execution handling

### Batch Processing

The function processes batches of up to 10 records. Each record is processed independently. If one fails, only that
record's batch fails (with `BisectBatchOnFunctionError: true`).

## Troubleshooting

### "DYNAMODB_TABLE_NAME environment variable is required"

Set the required environment variables in the Lambda configuration.

### "Failed to start step function"

Check:

1. State machine ARN is correct
2. IAM role has `states:StartExecution` permission
3. State machine exists and is active
4. Input JSON is valid

### Stream Processing Lag

If `IteratorAge` metric is high:

1. Increase Lambda reserved concurrency
2. Reduce batch size
3. Check for errors causing retries
4. Verify Step Functions capacity
