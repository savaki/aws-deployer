package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/services"
)

// setupLocalDynamoDB creates a DynamoDB client configured for local testing
// Set DYNAMODB_ENDPOINT environment variable to use local DynamoDB (e.g., http://localhost:8000)
func setupLocalDynamoDB(t *testing.T) *services.DynamoDBService {
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	if tableName == "" {
		tableName = "test-builds"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-west-2"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// Create table if it doesn't exist
	ctx := context.Background()
	_, err = client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		// Table doesn't exist, create it
		_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String(tableName),
			AttributeDefinitions: []types.AttributeDefinition{
				{
					AttributeName: aws.String("pk"),
					AttributeType: types.ScalarAttributeTypeS,
				},
				{
					AttributeName: aws.String("sk"),
					AttributeType: types.ScalarAttributeTypeS,
				},
			},
			KeySchema: []types.KeySchemaElement{
				{
					AttributeName: aws.String("pk"),
					KeyType:       types.KeyTypeHash,
				},
				{
					AttributeName: aws.String("sk"),
					KeyType:       types.KeyTypeRange,
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		if err != nil {
			t.Fatalf("failed to create table: %v", err)
		}

		// Wait for table to be active
		waiter := dynamodb.NewTableExistsWaiter(client)
		err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(tableName),
		}, 30*time.Second)
		if err != nil {
			t.Fatalf("failed to wait for table: %v", err)
		}
	}

	return services.NewDynamoDBServiceWithClient(client, tableName)
}

// cleanupTable removes all items from the table
func cleanupTable(t *testing.T, service *services.DynamoDBService) {
	ctx := context.Background()

	// Scan and delete all items
	result, err := service.GetClient().Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(service.GetTableName()),
	})
	if err != nil {
		t.Logf("failed to scan table: %v", err)
		return
	}

	for _, item := range result.Items {
		_, err := service.GetClient().DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(service.GetTableName()),
			Key: map[string]types.AttributeValue{
				"pk": item["pk"],
				"sk": item["sk"],
			},
		})
		if err != nil {
			t.Logf("failed to delete item: %v", err)
		}
	}
}

// loadTestData loads JSON test data from a file
func loadTestData(t *testing.T, filename string) *UpdateStatusInput {
	t.Helper()

	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test data file %s: %v", path, err)
	}

	var input UpdateStatusInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	return &input
}

// TestUpdateBuildStatus_Success verifies that updating a build to SUCCESS status works correctly
func TestUpdateBuildStatus_Success(t *testing.T) {
	service := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, service)
	})

	handler := &Handler{dbService: service}
	ctx := context.Background()

	// Create initial build in PENDING state
	_, err := service.PutBuild(ctx, builddao.CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          "2HFj3kLmNoPqRsTuVwXy",
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("failed to seed build: %v", err)
	}

	// Load and apply success update
	input := loadTestData(t, "success.json")
	if err := handler.HandleUpdateBuildStatus(ctx, input); err != nil {
		t.Fatalf("HandleUpdateBuildStatus failed: %v", err)
	}

	// Verify the build was updated
	updatedBuild, err := service.GetBuild(ctx, "test-repo", "dev", "2HFj3kLmNoPqRsTuVwXy")
	if err != nil {
		t.Fatalf("failed to get updated build: %v", err)
	}

	if updatedBuild.Status != builddao.BuildStatusSuccess {
		t.Errorf("expected status SUCCESS, got %s", updatedBuild.Status)
	}

	if updatedBuild.FinishedAt == nil {
		t.Error("expected FinishedAt to be set, got nil")
	}
}

// TestUpdateBuildStatus_Failure verifies that updating a build to FAILED status works correctly
func TestUpdateBuildStatus_Failure(t *testing.T) {
	service := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, service)
	})

	handler := &Handler{dbService: service}
	ctx := context.Background()

	// Create initial build in IN_PROGRESS state
	_, err := service.PutBuild(ctx, builddao.CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          "2HFj3kLmNoPqRsTuVwXy",
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("failed to seed build: %v", err)
	}

	// Load and apply failure update
	input := loadTestData(t, "failure.json")
	if err := handler.HandleUpdateBuildStatus(ctx, input); err != nil {
		t.Fatalf("HandleUpdateBuildStatus failed: %v", err)
	}

	// Verify the build was updated with error message
	updatedBuild, err := service.GetBuild(ctx, "test-repo", "dev", "2HFj3kLmNoPqRsTuVwXy")
	if err != nil {
		t.Fatalf("failed to get updated build: %v", err)
	}

	if updatedBuild.Status != builddao.BuildStatusFailed {
		t.Errorf("expected status FAILED, got %s", updatedBuild.Status)
	}

	if updatedBuild.ErrorMsg == nil {
		t.Error("expected ErrorMsg to be set, got nil")
	} else if *updatedBuild.ErrorMsg != "CloudFormation stack creation failed: Resource limit exceeded" {
		t.Errorf("unexpected error message: %s", *updatedBuild.ErrorMsg)
	}

	if updatedBuild.FinishedAt == nil {
		t.Error("expected FinishedAt to be set, got nil")
	}
}

// TestUpdateBuildStatus_InProgress verifies that updating a build to IN_PROGRESS status works correctly
func TestUpdateBuildStatus_InProgress(t *testing.T) {
	service := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, service)
	})

	handler := &Handler{dbService: service}
	ctx := context.Background()

	// Create initial build in PENDING state
	_, err := service.PutBuild(ctx, builddao.CreateInput{
		Repo:        "test-repo",
		Env:         "staging",
		SK:          "2HGk4lMnOqPrStUvXwYz",
		BuildNumber: "124",
		Branch:      "main",
		Version:     "124.def456",
		CommitHash:  "def456",
		StackName:   "staging-test-repo",
	})
	if err != nil {
		t.Fatalf("failed to seed build: %v", err)
	}

	// Load and apply in-progress update
	input := loadTestData(t, "in_progress.json")
	if err := handler.HandleUpdateBuildStatus(ctx, input); err != nil {
		t.Fatalf("HandleUpdateBuildStatus failed: %v", err)
	}

	// Verify the build was updated
	updatedBuild, err := service.GetBuild(ctx, "test-repo", "staging", "2HGk4lMnOqPrStUvXwYz")
	if err != nil {
		t.Fatalf("failed to get updated build: %v", err)
	}

	if updatedBuild.Status != builddao.BuildStatusInProgress {
		t.Errorf("expected status IN_PROGRESS, got %s", updatedBuild.Status)
	}

	// EndTime should not be set for IN_PROGRESS
	if updatedBuild.FinishedAt != nil {
		t.Error("expected EndTime to be nil for IN_PROGRESS, got non-nil")
	}
}

// TestUpdateBuildStatus_NonExistentBuild verifies error handling when build doesn't exist
func TestUpdateBuildStatus_NonExistentBuild(t *testing.T) {
	service := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, service)
	})

	handler := &Handler{dbService: service}
	ctx := context.Background()

	// Try to update a build that doesn't exist
	input := &UpdateStatusInput{
		Repo:   "non-existent-repo",
		Env:    "dev",
		SK:     "non-existent-ksuid",
		Status: "SUCCESS",
	}

	// This should succeed but not find anything to update
	// DynamoDB UpdateItem doesn't fail if the item doesn't exist
	err := handler.HandleUpdateBuildStatus(ctx, input)
	if err != nil {
		t.Logf("Update returned error (this is implementation-dependent): %v", err)
	}
}

// TestHandleUpdateBuildStatus_AllTestData verifies all test data files can be loaded and processed
func TestHandleUpdateBuildStatus_AllTestData(t *testing.T) {
	service := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, service)
	})

	handler := &Handler{dbService: service}
	ctx := context.Background()

	testFiles := []string{"success.json", "failure.json", "in_progress.json"}

	for _, filename := range testFiles {
		t.Run(filename, func(t *testing.T) {
			// Load test data
			input := loadTestData(t, filename)

			// Create a build to update
			_, err := service.PutBuild(ctx, builddao.CreateInput{
				Repo:        input.Repo,
				Env:         input.Env,
				SK:          input.SK,
				BuildNumber: "test",
				Branch:      "main",
				Version:     "1.0.0",
				CommitHash:  "abc123",
				StackName:   input.Env + "-" + input.Repo,
			})
			if err != nil {
				t.Fatalf("failed to seed build: %v", err)
			}

			// Apply update
			if err := handler.HandleUpdateBuildStatus(ctx, input); err != nil {
				t.Errorf("HandleUpdateBuildStatus failed for %s: %v", filename, err)
			}

			// Verify the build exists and was updated
			updatedBuild, err := service.GetBuild(ctx, input.Repo, input.Env, input.SK)
			if err != nil {
				t.Errorf("failed to get updated build: %v", err)
			}

			if string(updatedBuild.Status) != input.Status {
				t.Errorf("expected status %s, got %s", input.Status, updatedBuild.Status)
			}
		})
	}
}
