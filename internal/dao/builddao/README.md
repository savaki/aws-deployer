# BuildDAO

Data access layer for deployment build records using DynamoDB.

## Overview

The `builddao` package provides a clean abstraction for managing build records in DynamoDB. It uses
the [savaki/ddb/v2](https://github.com/savaki/ddb) library for simplified DynamoDB operations.

## Key Design Principles

1. **Type-Safe Keys**: Uses custom `PK` type instead of raw strings for partition keys
2. **ID-Based API**: Primary methods use composite `ID` type (format: `{repo}/{env}:{ksuid}`)
3. **Unix Timestamps**: All timestamps stored as `int64` Unix epochs for efficiency
4. **Single Source of Truth**: `builddao.Record` is the canonical build representation

## Table Structure

### Primary Records

Build records use a composite key structure:

- **Partition Key (PK)**: `PK` type representing `{repo}/{env}` (e.g., `myrepo/dev`)
- **Sort Key (SK)**: KSUID timestamp identifier (string)

Example: A build for repository `myapp` in environment `prd` might have:

- PK: `PK("myapp/prd")`  // Created via `NewPK("myapp", "prd")`
- SK: `"2HFj3kLmNoPqRsTuVwXy"`

### Latest Magic Records

To efficiently query the latest build for each repository in an environment, the system maintains special "latest"
records:

- **Partition Key (PK)**: `latest/{env}` (e.g., `latest/dev`)
- **Sort Key (SK)**: `{repo}/{env}` (e.g., `myrepo/dev`)
- **UpdatedAt**: Timestamp of last update

These magic records are automatically created/updated whenever a build status is updated via `UpdateStatus()`.

#### How It Works

1. When you call `UpdateStatus()`, the method performs two writes:
    - Updates the original build record
    - Creates/updates a latest record with `pk=latest/{env}` and `sk={repo}/{env}`

2. The latest record contains a complete copy of the build data, allowing for efficient queries without requiring a GSI
   or scan operations.

3. Query all latest builds for an environment using `QueryLatestBuilds(env)`:
   ```go
   records, err := dao.QueryLatestBuilds(ctx, "dev")
   // Returns one record per repo in the dev environment
   ```

## Type Definitions

### PK Type

```go
type PK string

// NewPK creates a new partition key from repo and env
func NewPK(repo, env string) PK

// ParsePK parses a partition key into its repo and env components
func ParsePK(pk PK) (repo, env string, err error)

// String returns the string representation
func (pk PK) String() string
```

### ID Type

```go
type ID string

// BuildID constructs an ID from partition key and sort key
func BuildID(pk PK, sk string) ID

// ParseID parses a build ID into its partition key and sort key components
func ParseID(id ID) (pk PK, sk string, err error)
```

## Record Fields

```go
type Record struct {
    PK           PK          // {repo}/{env} - DynamoDB partition key (type-safe)
    SK           string      // KSUID - DynamoDB sort key
    Repo         string      // Repository name only
    Env          string      // Environment name (dev, stg, prd)
    BuildNumber  string      // Build number from version
    Branch       string      // Git branch
    Version      string      // Version string
    CommitHash   string      // Git commit hash
    Status       BuildStatus // Current build status
    StackName    string      // CloudFormation stack name
    ExecutionArn *string     // Step Functions execution ARN (optional)
    ErrorMsg     *string     // Error message (optional)
    CreatedAt    int64       // Unix epoch timestamp of creation
    FinishedAt   *int64      // Unix epoch timestamp of completion (optional)
    UpdatedAt    int64       // Unix epoch timestamp of last update
}
```

## Usage Examples

### Creating a Build

```go
dao := builddao.New(dynamoClient, "builds-table")

// Status defaults to PENDING automatically
record, err := dao.Create(ctx, builddao.CreateInput{
    Repo:        "myapp",
    Env:         "prd",
    SK:          ksuid.New().String(),
    BuildNumber: "123",
    Branch:      "main",
    Version:     "v1.2.3",
    CommitHash:  "abc123",
    StackName:   "myapp-prd",
})
```

### Updating Build Status

```go
status := builddao.BuildStatusSuccess
_, err := dao.UpdateStatus(ctx, builddao.UpdateInput{
    PK:     builddao.NewPK("myapp", "prd"),
    SK:     "2HFj3kLmNoPqRsTuVwXy",
    Status: &status,
})
// This automatically updates both the primary record and the latest record
```

### Starting Step Functions Execution

```go
// Atomically sets status to IN_PROGRESS and records execution ARN
err := dao.StartExecution(ctx,
    builddao.NewPK("myapp", "prd"),
    "2HFj3kLmNoPqRsTuVwXy",
    "arn:aws:states:us-east-1:123456789:execution:deployment:abc-123")
```

### Querying Latest Builds

```go
// Get the latest build for each repo in the dev environment
records, err := dao.QueryLatestBuilds(ctx, "dev")
for _, record := range records {
    fmt.Printf("Repo: %s, Status: %s, Updated: %s\n",
        record.Repo, record.Status, record.UpdatedAt)
}
```

### Querying All Builds for a Repo/Env

```go
// Get all build history for a specific repo/env
records, err := dao.QueryByRepoEnv(ctx, "myapp", "prd")
```

### Finding a Specific Build

```go
// By ID (recommended - default method)
pk := builddao.NewPK("myapp", "prd")
id := builddao.BuildID(pk, "2HFj3kLmNoPqRsTuVwXy")
record, err := dao.Find(ctx, id)

// Or construct ID from string if you have the full format
id := builddao.ID("myapp/prd:2HFj3kLmNoPqRsTuVwXy")
record, err := dao.Find(ctx, id)
```

### Deleting a Build

```go
// Delete by ID
id := builddao.ID("myapp/prd:2HFj3kLmNoPqRsTuVwXy")
err := dao.Delete(ctx, id)
```

## Build Status Values

```go
const (
    BuildStatusPending    BuildStatus = "PENDING"
    BuildStatusInProgress BuildStatus = "IN_PROGRESS"
    BuildStatusSuccess    BuildStatus = "SUCCESS"
    BuildStatusFailed     BuildStatus = "FAILED"
)
```

## API Design Philosophy

### ID-Based Methods Are Default

Unlike traditional DAO patterns that require separate partition and sort keys, builddao uses composite IDs as the
primary API:

```go
// ❌ Old pattern - passing keys separately
record, err := dao.Find(ctx, pk, sk)

// ✅ New pattern - using ID
id := builddao.BuildID(pk, sk)
record, err := dao.Find(ctx, id)
```

**Rationale**:

- Reduces parameter passing complexity
- Type-safe ID construction via `BuildID()`
- Natural mapping to REST/GraphQL ID fields
- Prevents accidental key swapping

### Type-Safe PK Construction

Always use `NewPK()` instead of string formatting:

```go
// ❌ Error-prone
pk := fmt.Sprintf("%s/%s", repo, env)

// ✅ Type-safe
pk := builddao.NewPK(repo, env)
```

### Unix Epoch Timestamps

Timestamps are `int64` Unix epochs, not `time.Time`:

```go
record.CreatedAt  // int64, e.g., 1704067200
record.UpdatedAt  // int64
record.EndTime    // *int64 (optional)

// Convert to time.Time when needed
t := time.Unix(record.CreatedAt, 0)
```

**Benefits**:

- More efficient DynamoDB storage
- Simpler JSON serialization
- No timezone ambiguity
- Direct numeric comparison

## Benefits of Latest Magic Records

1. **No GSI Required**: Query latest builds without a Global Secondary Index
2. **Efficient Queries**: Single query operation returns all latest builds for an environment
3. **Automatic Maintenance**: Latest records are automatically updated on status changes
4. **Complete Data**: Latest records contain full build information, no need for additional lookups
5. **Time Ordering**: Records include `UpdatedAt` (int64) for sorting by most recent updates

## Design Considerations

- The latest record approach trades write amplification (2 writes per update) for read efficiency
- This pattern is ideal when you frequently need to display the latest state across multiple repos
- Storage overhead is minimal since latest records are just duplicates of the most recent builds
- Updates are not atomic across both records, but eventual consistency is acceptable for this use case

## Testing

The package includes comprehensive unit and integration tests.

### Running Unit Tests Only

Unit tests test the key types (PK, ID) and don't require external dependencies:

```bash
go test -short ./internal/dao/builddao/... -v
```

### Running Integration Tests

Integration tests require a local DynamoDB instance. Start DynamoDB Local using Docker:

```bash
# Start DynamoDB Local
docker-compose up -d dynamodb-local

# Run all tests (including integration tests)
go test ./internal/dao/builddao/... -v

# Run with explicit DynamoDB endpoint
DYNAMODB_ENDPOINT=http://localhost:8000 go test ./internal/dao/builddao/... -v
```

### Test Coverage

The integration tests cover:

- **Create and Find**: Creating build records and retrieving them by ID
- **Find Not Found**: Handling non-existent records gracefully
- **Delete**: Removing build records
- **UpdateStatus**: Updating build status with various states (PENDING, IN_PROGRESS, SUCCESS, FAILED)
- **Latest Records**: Verifying that latest magic records are created and updated correctly
- **Query**: Querying all builds for a repo/env
- **QueryByRepoEnv**: Querying builds by repository and environment
- **QueryLatestBuilds**: Querying the latest build for each repository in an environment
- **Multiple Updates**: Ensuring latest records reflect the most recent update

### Writing New Tests

When adding new functionality, follow the existing test pattern:

```go
func TestDAO_YourNewFeature(t *testing.T) {
    setup := setupLocalDynamoDB(t)
    t.Cleanup(func() {
        cleanupTable(t, setup)
    })

    ctx := context.Background()

    // Your test code here
}
```

Each test creates its own unique table (using a KSUID in the name) and cleans it up automatically after the test
completes.
