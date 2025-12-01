package builddao

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/savaki/ddb/v2"
	"github.com/savaki/ddb/v2/ddbtest"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/assert"
)

// Unit tests for key types

func TestNewPK(t *testing.T) {
	tests := []struct {
		name string
		repo string
		env  string
		want PK
	}{
		{
			name: "valid repo and env",
			repo: "test-repo",
			env:  "dev",
			want: PK("test-repo/dev"),
		},
		{
			name: "prod environment",
			repo: "my-service",
			env:  "prod",
			want: PK("my-service/prod"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPK(tt.repo, tt.env)
			if got != tt.want {
				t.Errorf("NewPK() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParsePK(t *testing.T) {
	tests := []struct {
		name     string
		pk       PK
		wantRepo string
		wantEnv  string
		wantErr  bool
	}{
		{
			name:     "valid PK",
			pk:       PK("test-repo/dev"),
			wantRepo: "test-repo",
			wantEnv:  "dev",
			wantErr:  false,
		},
		{
			name:     "invalid PK - no slash",
			pk:       PK("test-repo"),
			wantRepo: "",
			wantEnv:  "",
			wantErr:  true,
		},
		{
			name:     "invalid PK - too many slashes",
			pk:       PK("test/repo/dev"),
			wantRepo: "",
			wantEnv:  "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, env, err := ParsePK(tt.pk)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePK() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if repo != tt.wantRepo {
				t.Errorf("ParsePK() repo = %v, want %v", repo, tt.wantRepo)
			}
			if env != tt.wantEnv {
				t.Errorf("ParsePK() env = %v, want %v", env, tt.wantEnv)
			}
		})
	}
}

func TestPK_String(t *testing.T) {
	pk := NewPK("test-repo", "dev")
	expected := "test-repo/dev"

	result := pk.String()
	if result != expected {
		t.Errorf("PK.String() = %v, want %v", result, expected)
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		id      ID
		wantPK  PK
		wantSK  string
		wantErr bool
	}{
		{
			name:    "valid ID",
			id:      "test-repo/dev:2HFj3kLmNoPqRsTuVwXy",
			wantPK:  PK("test-repo/dev"),
			wantSK:  "2HFj3kLmNoPqRsTuVwXy",
			wantErr: false,
		},
		{
			name:    "invalid ID - no colon",
			id:      "test-repo/dev",
			wantPK:  "",
			wantSK:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pk, sk, err := ParseID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if pk != tt.wantPK {
				t.Errorf("ParseID() pk = %v, want %v", pk, tt.wantPK)
			}
			if sk != tt.wantSK {
				t.Errorf("ParseID() sk = %v, want %v", sk, tt.wantSK)
			}
		})
	}
}

func TestNewID(t *testing.T) {
	pk := NewPK("test-repo", "dev")
	sk := "2HFj3kLmNoPqRsTuVwXy"
	expected := ID("test-repo/dev:2HFj3kLmNoPqRsTuVwXy")

	result := NewID(pk, sk)
	if result != expected {
		t.Errorf("NewID() = %v, want %v", result, expected)
	}
}

func TestRecord_ID(t *testing.T) {
	record := &Record{
		PK: NewPK("test-repo", "dev"),
		SK: "2HFj3kLmNoPqRsTuVwXy",
	}

	expected := ID("test-repo/dev:2HFj3kLmNoPqRsTuVwXy")
	result := record.GetID()

	if result != expected {
		t.Errorf("Record.ID() = %v, want %v", result, expected)
	}
}

// Integration test helpers

type testSetup struct {
	dao       *DAO
	client    *dynamodb.Client
	tableName string
}

// setupLocalDynamoDB creates a DynamoDB client configured for local testing
// Set DYNAMODB_ENDPOINT environment variable to use local DynamoDB (e.g., http://localhost:8000)
// Run: docker-compose up -d dynamodb-local
func setupLocalDynamoDB(t *testing.T) *testSetup {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	tableName := "test-builds-" + ksuid.New().String()

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

	// Create table
	ctx := context.Background()
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

	return &testSetup{
		dao:       New(client, tableName),
		client:    client,
		tableName: tableName,
	}
}

// cleanupTable deletes the test table
func cleanupTable(t *testing.T, setup *testSetup) {
	ctx := context.Background()
	_, err := setup.client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(setup.tableName),
	})
	if err != nil {
		t.Logf("failed to delete table: %v", err)
	}
}

// Integration Tests

func TestDAO_CreateAndFind(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	sk := ksuid.New().String()

	// Create a build record
	input := CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	}

	created, err := setup.dao.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify created record
	if created.Repo != input.Repo {
		t.Errorf("created.Repo = %v, want %v", created.Repo, input.Repo)
	}
	if created.Status != BuildStatusPending {
		t.Errorf("created.Status = %v, want %v", created.Status, BuildStatusPending)
	}
	if created.CreatedAt == 0 {
		t.Error("created.CreatedAt should be set")
	}
	if created.UpdatedAt == 0 {
		t.Error("created.UpdatedAt should be set")
	}

	// Find the record
	pk := NewPK("test-repo", "dev")
	id := NewID(pk, sk)
	found, err := setup.dao.Find(ctx, id)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if found.Repo != input.Repo {
		t.Errorf("found.Repo = %v, want %v", found.Repo, input.Repo)
	}
	if found.BuildNumber != input.BuildNumber {
		t.Errorf("found.BuildNumber = %v, want %v", found.BuildNumber, input.BuildNumber)
	}
}

func TestDAO_Find_NotFound(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	pk := NewPK("non-existent", "dev")
	id := NewID(pk, "non-existent-ksuid")

	_, err := setup.dao.Find(ctx, id)
	if err == nil {
		t.Fatal("Find should return error for non-existent record")
	}
}

func TestDAO_Delete(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	sk := ksuid.New().String()

	// Create a build record
	input := CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	}

	_, err := setup.dao.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete the record
	pk := NewPK("test-repo", "dev")
	id := NewID(pk, sk)
	err = setup.dao.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, err = setup.dao.Find(ctx, id)
	if err == nil {
		t.Fatal("Find should return error after delete")
	}
}

func TestDAO_UpdateStatus_Success(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	sk := ksuid.New().String()

	// Create initial build
	_, err := setup.dao.Create(ctx, CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update to SUCCESS
	pk := NewPK("test-repo", "dev")
	status := BuildStatusSuccess
	err = setup.dao.UpdateStatus(ctx, UpdateInput{
		PK:     pk,
		SK:     sk,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify the update
	id := NewID(pk, sk)
	updated, err := setup.dao.Find(ctx, id)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if updated.Status != BuildStatusSuccess {
		t.Errorf("updated.Status = %v, want %v", updated.Status, BuildStatusSuccess)
	}
	if updated.FinishedAt == nil {
		t.Error("updated.FinishedAt should be set for SUCCESS status")
	}
	if updated.UpdatedAt == 0 {
		t.Error("updated.UpdatedAt should be set")
	}
}

func TestDAO_UpdateStatus_Failure(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	sk := ksuid.New().String()

	// Create initial build
	_, err := setup.dao.Create(ctx, CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update to FAILED with error message
	pk := NewPK("test-repo", "dev")
	status := BuildStatusFailed
	errorMsg := "Stack creation failed: Resource limit exceeded"
	err = setup.dao.UpdateStatus(ctx, UpdateInput{
		PK:       pk,
		SK:       sk,
		Status:   &status,
		ErrorMsg: &errorMsg,
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify the update
	id := NewID(pk, sk)
	updated, err := setup.dao.Find(ctx, id)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if updated.Status != BuildStatusFailed {
		t.Errorf("updated.Status = %v, want %v", updated.Status, BuildStatusFailed)
	}
	if updated.ErrorMsg == nil {
		t.Fatal("updated.ErrorMsg should be set for FAILED status")
	}
	if *updated.ErrorMsg != errorMsg {
		t.Errorf("updated.ErrorMsg = %v, want %v", *updated.ErrorMsg, errorMsg)
	}
	if updated.FinishedAt == nil {
		t.Error("updated.FinishedAt should be set for FAILED status")
	}
}

func TestDAO_UpdateStatus_InProgress(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	sk := ksuid.New().String()

	// Create initial build
	_, err := setup.dao.Create(ctx, CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update to IN_PROGRESS
	pk := NewPK("test-repo", "dev")
	status := BuildStatusInProgress
	err = setup.dao.UpdateStatus(ctx, UpdateInput{
		PK:     pk,
		SK:     sk,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify the update
	id := NewID(pk, sk)
	updated, err := setup.dao.Find(ctx, id)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if updated.Status != BuildStatusInProgress {
		t.Errorf("updated.Status = %v, want %v", updated.Status, BuildStatusInProgress)
	}
	if updated.FinishedAt != nil {
		t.Error("updated.FinishedAt should be nil for IN_PROGRESS status")
	}
}

func TestDAO_UpdateStatus_CreatesLatestRecord(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()
	sk := ksuid.New().String()

	// Create initial build
	_, err := setup.dao.Create(ctx, CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update status
	pk := NewPK("test-repo", "dev")
	status := BuildStatusSuccess
	err = setup.dao.UpdateStatus(ctx, UpdateInput{
		PK:     pk,
		SK:     sk,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify latest record was created
	latestPK := NewPK(latest, "dev")
	latestID := NewID(latestPK, pk.String())
	latestRecord, err := setup.dao.Find(ctx, latestID)
	if err != nil {
		t.Fatalf("Find latest record failed: %v", err)
	}

	if latestRecord.Repo != "test-repo" {
		t.Errorf("latestRecord.Repo = %v, want test-repo", latestRecord.Repo)
	}
	if latestRecord.Env != "dev" {
		t.Errorf("latestRecord.Env = %v, want dev", latestRecord.Env)
	}
	if latestRecord.Status != BuildStatusSuccess {
		t.Errorf("latestRecord.Status = %v, want %v", latestRecord.Status, BuildStatusSuccess)
	}
}

func TestDAO_Query(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()

	// Create multiple builds for same repo/env
	for i := 0; i < 3; i++ {
		_, err := setup.dao.Create(ctx, CreateInput{
			Repo:        "test-repo",
			Env:         "dev",
			SK:          ksuid.New().String(),
			BuildNumber: "123",
			Branch:      "main",
			Version:     "123.abc123",
			CommitHash:  "abc123",
			StackName:   "dev-test-repo",
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Query all builds
	pk := NewPK("test-repo", "dev")
	records, err := setup.dao.Query(ctx, pk)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(records) != 3 {
		t.Errorf("Query returned %d records, want 3", len(records))
	}
}

func TestDAO_QueryByRepoEnv(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()

	// Create builds in different environments
	for _, env := range []string{"dev", "staging", "prod"} {
		_, err := setup.dao.Create(ctx, CreateInput{
			Repo:        "test-repo",
			Env:         env,
			SK:          ksuid.New().String(),
			BuildNumber: "123",
			Branch:      "main",
			Version:     "123.abc123",
			CommitHash:  "abc123",
			StackName:   env + "-test-repo",
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Query only dev builds
	records, err := setup.dao.QueryByRepoEnv(ctx, "test-repo", "dev")
	if err != nil {
		t.Fatalf("QueryByRepoEnv failed: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("QueryByRepoEnv returned %d records, want 1", len(records))
	}

	if records[0].Env != "dev" {
		t.Errorf("records[0].Env = %v, want dev", records[0].Env)
	}
}

func TestDAO_QueryLatestBuilds(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()

	// Create builds for different repos in same environment
	repos := []string{"repo-a", "repo-b", "repo-c"}
	for _, repo := range repos {
		sk := ksuid.New().String()

		// Create initial build
		_, err := setup.dao.Create(ctx, CreateInput{
			Repo:        repo,
			Env:         "dev",
			SK:          sk,
			BuildNumber: "123",
			Branch:      "main",
			Version:     "123.abc123",
			CommitHash:  "abc123",
			StackName:   "dev-" + repo,
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Update status to trigger latest record creation
		pk := NewPK(repo, "dev")
		status := BuildStatusSuccess
		err = setup.dao.UpdateStatus(ctx, UpdateInput{
			PK:     pk,
			SK:     sk,
			Status: &status,
		})
		if err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}

		// Small delay to ensure different UpdatedAt timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Query latest builds
	latestBuilds, err := setup.dao.QueryLatestBuilds(ctx, "dev")
	if err != nil {
		t.Fatalf("QueryLatestBuilds failed: %v", err)
	}

	if len(latestBuilds) != 3 {
		t.Errorf("QueryLatestBuilds returned %d records, want 3", len(latestBuilds))
	}

	// Verify they are sorted by UpdatedAt descending (most recent first)
	for i := 0; i < len(latestBuilds)-1; i++ {
		if latestBuilds[i].UpdatedAt < latestBuilds[i+1].UpdatedAt {
			t.Errorf("Latest builds not sorted by UpdatedAt descending: %d < %d",
				latestBuilds[i].UpdatedAt, latestBuilds[i+1].UpdatedAt)
		}
	}

	// Verify all repos are represented
	foundRepos := make(map[string]bool)
	for _, build := range latestBuilds {
		foundRepos[build.Repo] = true
	}

	for _, repo := range repos {
		if !foundRepos[repo] {
			t.Errorf("Latest builds missing repo: %s", repo)
		}
	}
}

func TestDAO_QueryLatestBuilds_MultipleUpdates(t *testing.T) {
	setup := setupLocalDynamoDB(t)
	t.Cleanup(func() {
		cleanupTable(t, setup)
	})

	ctx := context.Background()

	// Create multiple builds for same repo, update them at different times
	sk1 := ksuid.New().String()
	sk2 := ksuid.New().String()

	// Create first build
	_, err := setup.dao.Create(ctx, CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk1,
		BuildNumber: "123",
		Branch:      "main",
		Version:     "123.abc123",
		CommitHash:  "abc123",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update first build
	pk := NewPK("test-repo", "dev")
	status1 := BuildStatusSuccess
	err = setup.dao.UpdateStatus(ctx, UpdateInput{
		PK:     pk,
		SK:     sk1,
		Status: &status1,
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create second build
	_, err = setup.dao.Create(ctx, CreateInput{
		Repo:        "test-repo",
		Env:         "dev",
		SK:          sk2,
		BuildNumber: "124",
		Branch:      "main",
		Version:     "124.def456",
		CommitHash:  "def456",
		StackName:   "dev-test-repo",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update second build (this should be the latest)
	status2 := BuildStatusSuccess
	err = setup.dao.UpdateStatus(ctx, UpdateInput{
		PK:     pk,
		SK:     sk2,
		Status: &status2,
	})
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Query latest builds - should only return one for this repo
	latestBuilds, err := setup.dao.QueryLatestBuilds(ctx, "dev")
	if err != nil {
		t.Fatalf("QueryLatestBuilds failed: %v", err)
	}

	if len(latestBuilds) != 1 {
		t.Fatalf("QueryLatestBuilds returned %d records, want 1", len(latestBuilds))
	}

	// Verify it's pointing to the latest build
	// Note: The latest record's SK is the original PK (test-repo/dev), not the build SK
	if latestBuilds[0].Repo != "test-repo" {
		t.Errorf("Latest build repo = %v, want test-repo", latestBuilds[0].Repo)
	}
}

type Data struct {
	DAO *DAO
}

func setup(t *testing.T) (ctx context.Context, data Data, cleanup func()) {
	ctx = context.Background()

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion("us-west-2"),
		config.WithBaseEndpoint("http://localhost:8000"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("blah", "blah", ""),
		),
	)
	assert.NoError(t, err)

	var (
		client    = dynamodb.NewFromConfig(cfg)
		db        = ddb.New(client)
		tableName = fmt.Sprintf("table-%v", ksuid.New().String())
		table     = db.MustTable(tableName, Record{})
		dao       = New(client, tableName)
	)

	err = table.CreateTableIfNotExists(ctx)
	assert.NoError(t, err)

	return ctx, Data{DAO: dao}, func() {
		_ = table.DeleteTableIfExists(ctx)
	}
}

func TestDAO(t *testing.T) {
	ddbtest.WithTable[Data](t, setup, func(t *testing.T, ctx context.Context, data Data) {

	})
}
