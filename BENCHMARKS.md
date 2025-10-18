# Benchmark Tests

This project includes benchmark tests for DynamoDB operations using a local DynamoDB instance.

## Prerequisites

- Docker and Docker Compose
- Go 1.24 or later

## Running Benchmarks

### Quick Start

Use the provided script to start local DynamoDB and run all benchmarks:

```bash
./run-benchmarks.sh
```

This script will:

1. Start a local DynamoDB instance using Docker Compose
2. Wait for DynamoDB to be ready
3. Run all benchmark tests
4. Display results

### Manual Execution

#### 1. Start Local DynamoDB

```bash
docker-compose up -d dynamodb-local
```

#### 2. Run Benchmarks

```bash
export DYNAMODB_ENDPOINT=http://localhost:8000
export DYNAMODB_TABLE_NAME=benchmark-test-builds
go test -bench=. -benchmem -benchtime=5s ./internal/services/
```

#### 3. Stop Local DynamoDB

```bash
docker-compose down
```

## Available Benchmarks

### Single Operation Benchmarks

- **BenchmarkPutBuild** - Measures performance of inserting build records
- **BenchmarkGetBuild** - Measures performance of retrieving a single build record
- **BenchmarkUpdateBuildStatus** - Measures performance of updating build status
- **BenchmarkQueryBuildsByRepo** - Measures performance of querying builds by repository (10 items)
- **BenchmarkQueryBuildsByRepo_100Items** - Measures query performance with 100 items

### Concurrent Operation Benchmarks

- **BenchmarkConcurrentPutBuild** - Measures performance of concurrent write operations
- **BenchmarkConcurrentGetBuild** - Measures performance of concurrent read operations

## Benchmark Options

### Run Specific Benchmark

```bash
go test -bench=BenchmarkPutBuild -benchmem ./internal/services/
```

### Adjust Benchmark Duration

```bash
go test -bench=. -benchmem -benchtime=10s ./internal/services/
```

### Run with CPU Profiling

```bash
go test -bench=. -benchmem -cpuprofile=cpu.prof ./internal/services/
go tool pprof cpu.prof
```

### Run with Memory Profiling

```bash
go test -bench=. -benchmem -memprofile=mem.prof ./internal/services/
go tool pprof mem.prof
```

## Configuration

### Environment Variables

- `DYNAMODB_ENDPOINT` - DynamoDB endpoint URL (default: `http://localhost:8000`)
- `DYNAMODB_TABLE_NAME` - Table name for benchmarks (default: `benchmark-test-builds`)

### Custom Endpoint

To benchmark against a different DynamoDB endpoint:

```bash
export DYNAMODB_ENDPOINT=http://your-endpoint:8000
./run-benchmarks.sh
```

## Interpreting Results

Benchmark output includes:

- **ns/op** - Nanoseconds per operation (lower is better)
- **B/op** - Bytes allocated per operation (lower is better)
- **allocs/op** - Number of allocations per operation (lower is better)

Example output:

```
BenchmarkPutBuild-8                     1000      5234567 ns/op      2048 B/op      45 allocs/op
BenchmarkGetBuild-8                     2000      2123456 ns/op      1024 B/op      23 allocs/op
```

## Troubleshooting

### Port Already in Use

If port 8000 is already in use, modify `docker-compose.yml`:

```yaml
ports:
  - "8001:8000"  # Use port 8001 on host
```

Then set the endpoint:

```bash
export DYNAMODB_ENDPOINT=http://localhost:8001
```

### Table Already Exists

The benchmarks automatically create and clean up tables. If you encounter issues, manually delete the table:

```bash
aws dynamodb delete-table --table-name benchmark-test-builds --endpoint-url http://localhost:8000
```

## CI/CD Integration

To integrate benchmarks into CI/CD pipelines, use the provided script:

```bash
# In your CI configuration
- name: Run Benchmarks
  run: |
    ./run-benchmarks.sh
    docker-compose down
```

## Best Practices

1. **Run benchmarks multiple times** to account for variance
2. **Use consistent hardware** for comparing benchmark results
3. **Close other applications** to minimize interference
4. **Warm up the system** by running benchmarks twice (first run discarded)
5. **Monitor system resources** during benchmark execution

## Additional Resources

- [Go Benchmarking Guide](https://golang.org/pkg/testing/#hdr-Benchmarks)
- [DynamoDB Local Documentation](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html)
- [AWS SDK Go v2 Documentation](https://aws.github.io/aws-sdk-go-v2/)
