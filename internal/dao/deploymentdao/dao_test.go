package deploymentdao

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/ddb/v2"
	"github.com/savaki/ddb/v2/ddbtest"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/assert"
)

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
		tableName = fmt.Sprintf("deployments-test-%v", ksuid.New().String())
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
		dao := data.DAO

		// Test 1: Create deployment record
		t.Run("Create", func(t *testing.T) {
			buildID := ksuid.New().String()

			created, err := dao.Create(ctx, CreateInput{
				Env:     "dev",
				Repo:    "test-repo",
				Account: "111111111111",
				Region:  "us-east-1",
				BuildID: buildID,
			})
			assert.NoError(t, err)
			assert.NotNil(t, created)

			id := created.GetID()

			// Verify it was created
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, "dev/test-repo", record.PK.String())
			assert.Equal(t, "111111111111/us-east-1", record.SK.String())
			assert.Equal(t, "dev/test-repo:111111111111/us-east-1", record.GetID().String())
			assert.Equal(t, buildID, record.BuildID)
			assert.Equal(t, StatusPending, record.Status)
			assert.NotZero(t, record.CreatedAt)
			assert.NotZero(t, record.UpdatedAt)
		})

		// Test 2: Find non-existent deployment
		t.Run("Find_NotFound", func(t *testing.T) {
			id := NewID("dev", "non-existent", "111111111111", "us-east-1")
			_, err := dao.Find(ctx, id)
			assert.Error(t, err, "should return error for non-existent record")
		})

		// Test 3: UpdateStatus to IN_PROGRESS
		t.Run("UpdateStatus_InProgress", func(t *testing.T) {
			buildID := ksuid.New().String()
			env := "dev"
			repo := "progress-repo"
			account := "222222222222"
			region := "us-west-2"

			created, err := dao.Create(ctx, CreateInput{
				Env:     env,
				Repo:    repo,
				Account: account,
				Region:  region,
				BuildID: buildID,
			})
			assert.NoError(t, err)

			id := created.GetID()

			// Update to IN_PROGRESS
			err = dao.UpdateStatus(ctx, UpdateInput{
				Env:         env,
				Repo:        repo,
				Account:     account,
				Region:      region,
				Status:      StatusInProgress,
				OperationID: "op-12345",
			})
			assert.NoError(t, err)

			// Verify update
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, StatusInProgress, record.Status)
			assert.Equal(t, "op-12345", record.OperationID)
			assert.Zero(t, record.FinishedAt) // Should NOT be set
		})

		// Test 4: UpdateStatus to SUCCESS
		t.Run("UpdateStatus_Success", func(t *testing.T) {
			buildID := ksuid.New().String()
			env := "dev"
			repo := "success-repo"
			account := "333333333333"
			region := "eu-west-1"

			created, err := dao.Create(ctx, CreateInput{
				Env:     env,
				Repo:    repo,
				Account: account,
				Region:  region,
				BuildID: buildID,
			})
			assert.NoError(t, err)

			id := created.GetID()

			// Update to SUCCESS
			err = dao.UpdateStatus(ctx, UpdateInput{
				Env:     env,
				Repo:    repo,
				Account: account,
				Region:  region,
				Status:  StatusSuccess,
				StackID: "arn:aws:cloudformation:eu-west-1:333333333333:stack/test/id",
			})
			assert.NoError(t, err)

			// Verify update
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, StatusSuccess, record.Status)
			assert.NotEmpty(t, record.StackID)
			assert.NotZero(t, record.FinishedAt) // Should be set for terminal status
		})

		// Test 5: UpdateStatus to FAILED with error details
		t.Run("UpdateStatus_Failed", func(t *testing.T) {
			buildID := ksuid.New().String()
			env := "dev"
			repo := "failed-repo"
			account := "444444444444"
			region := "ap-south-1"

			created, err := dao.Create(ctx, CreateInput{
				Env:     env,
				Repo:    repo,
				Account: account,
				Region:  region,
				BuildID: buildID,
			})
			assert.NoError(t, err)

			id := created.GetID()

			// Update to FAILED with error details
			stackEvents := []string{
				"Resource1: CREATE_FAILED - Timeout",
				"Resource2: CREATE_FAILED - Limit exceeded",
			}
			err = dao.UpdateStatus(ctx, UpdateInput{
				Env:          env,
				Repo:         repo,
				Account:      account,
				Region:       region,
				Status:       StatusFailed,
				StatusReason: "Stack creation failed",
				ErrorMsg:     "CREATE_FAILED: Timeout waiting for resources",
				StackEvents:  stackEvents,
			})
			assert.NoError(t, err)

			// Verify update
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, StatusFailed, record.Status)
			assert.Equal(t, "Stack creation failed", record.StatusReason)
			assert.Equal(t, "CREATE_FAILED: Timeout waiting for resources", record.ErrorMsg)
			assert.Len(t, record.StackEvents, 2)
			assert.NotZero(t, record.FinishedAt) // Should be set for terminal status
		})

		// Test 6: QueryByBuild - get all deployments for a build
		t.Run("QueryByBuild", func(t *testing.T) {
			buildID := ksuid.New().String()
			env := "query-env"
			repo := "query-repo"

			// Create multiple deployments for same build (different accounts/regions)
			deployments := []struct {
				account string
				region  string
			}{
				{"111111111111", "us-east-1"},
				{"111111111111", "us-west-2"},
				{"222222222222", "us-east-1"},
				{"222222222222", "us-west-2"},
			}

			for _, d := range deployments {
				_, err := dao.Create(ctx, CreateInput{
					Env:     env,
					Repo:    repo,
					Account: d.account,
					Region:  d.region,
					BuildID: buildID,
				})
				assert.NoError(t, err)
			}

			// Query all deployments for this build
			records, err := dao.QueryByBuild(ctx, env, repo, buildID)
			assert.NoError(t, err)
			assert.Len(t, records, 4)

			// Verify all deployments have same build ID
			for _, record := range records {
				assert.Equal(t, buildID, record.BuildID)
			}
		})

		// Test 7: Mixed status deployments (partial success scenario)
		t.Run("PartialSuccess", func(t *testing.T) {
			buildID := ksuid.New().String()
			env := "partial-env"
			repo := "partial-repo"

			// Create 4 deployments
			deployments := []struct {
				account string
				region  string
				status  DeploymentStatus
			}{
				{"111111111111", "us-east-1", StatusSuccess},
				{"111111111111", "us-west-2", StatusSuccess},
				{"222222222222", "us-east-1", StatusFailed},
				{"222222222222", "us-west-2", StatusInProgress},
			}

			for _, d := range deployments {
				_, err := dao.Create(ctx, CreateInput{
					Env:     env,
					Repo:    repo,
					Account: d.account,
					Region:  d.region,
					BuildID: buildID,
				})
				assert.NoError(t, err)

				// Update to target status
				err = dao.UpdateStatus(ctx, UpdateInput{
					Env:     env,
					Repo:    repo,
					Account: d.account,
					Region:  d.region,
					Status:  d.status,
				})
				assert.NoError(t, err)
			}

			// Query and count statuses
			records, err := dao.QueryByBuild(ctx, env, repo, buildID)
			assert.NoError(t, err)
			assert.Len(t, records, 4)

			succeeded := 0
			failed := 0
			inProgress := 0

			for _, record := range records {
				switch record.Status {
				case StatusSuccess:
					succeeded++
				case StatusFailed:
					failed++
				case StatusInProgress:
					inProgress++
				}
			}

			assert.Equal(t, 2, succeeded)
			assert.Equal(t, 1, failed)
			assert.Equal(t, 1, inProgress)
		})

		// Test 8: Delete deployment
		t.Run("Delete", func(t *testing.T) {
			buildID := ksuid.New().String()
			env := "delete-env"
			repo := "delete-repo"
			account := "555555555555"
			region := "ca-central-1"

			created, err := dao.Create(ctx, CreateInput{
				Env:     env,
				Repo:    repo,
				Account: account,
				Region:  region,
				BuildID: buildID,
			})
			assert.NoError(t, err)

			id := created.GetID()

			// Verify it exists
			_, err = dao.Find(ctx, id)
			assert.NoError(t, err)

			// Delete it
			err = dao.Delete(ctx, id)
			assert.NoError(t, err)

			// Verify it's gone
			_, err = dao.Find(ctx, id)
			assert.Error(t, err, "should return error after delete")
		})
	})
}
