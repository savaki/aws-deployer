package builddao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/savaki/ddb/v2/ddbtest"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/assert"
)

func TestDAOComprehensive(t *testing.T) {
	ddbtest.WithTable[Data](t, setup, func(t *testing.T, ctx context.Context, data Data) {
		dao := data.DAO

		// Test 1: Create
		t.Run("Create", func(t *testing.T) {
			sk := ksuid.New().String()
			input := CreateInput{
				Repo:        "test-repo",
				Env:         "dev",
				SK:          sk,
				BuildNumber: "100",
				Branch:      "main",
				Version:     "100.abc123",
				CommitHash:  "abc123",
				StackName:   "dev-test-repo",
			}

			record, err := dao.Create(ctx, input)
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, input.Repo, record.Repo)
			assert.Equal(t, input.Env, record.Env)
			assert.Equal(t, input.SK, record.SK)
			assert.Equal(t, input.BuildNumber, record.BuildNumber)
			assert.Equal(t, BuildStatusPending, record.Status)
			assert.NotZero(t, record.CreatedAt)
			assert.NotZero(t, record.UpdatedAt)
			assert.Equal(t, "test-repo/dev", record.PK.String())
		})

		// Test 2: Find
		t.Run("Find", func(t *testing.T) {
			// Create a record first
			sk := ksuid.New().String()
			input := CreateInput{
				Repo:        "find-repo",
				Env:         "dev",
				SK:          sk,
				BuildNumber: "101",
				Branch:      "feature",
				Version:     "101.def456",
				CommitHash:  "def456",
				StackName:   "dev-find-repo",
			}

			created, err := dao.Create(ctx, input)
			assert.NoError(t, err)

			// Find it
			id := created.GetID()
			found, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, created.Repo, found.Repo)
			assert.Equal(t, created.BuildNumber, found.BuildNumber)
			assert.Equal(t, created.Status, found.Status)
		})

		// Test 3: Find non-existent record
		t.Run("Find_NotFound", func(t *testing.T) {
			pk := NewPK("non-existent", "dev")
			id := NewID(pk, "non-existent-ksuid")

			_, err := dao.Find(ctx, id)
			assert.Error(t, err, "should return error for non-existent record")
		})

		// Test 4: Delete
		t.Run("Delete", func(t *testing.T) {
			// Create a record
			sk := ksuid.New().String()
			input := CreateInput{
				Repo:        "delete-repo",
				Env:         "dev",
				SK:          sk,
				BuildNumber: "102",
				Branch:      "main",
				Version:     "102.ghi789",
				CommitHash:  "ghi789",
				StackName:   "dev-delete-repo",
			}

			created, err := dao.Create(ctx, input)
			assert.NoError(t, err)

			// Delete it
			err = dao.Delete(ctx, created.GetID())
			assert.NoError(t, err)

			// Verify it's gone
			_, err = dao.Find(ctx, created.GetID())
			assert.Error(t, err, "should return error after delete")
		})

		// Test 5: UpdateStatus - Success
		t.Run("UpdateStatus_Success", func(t *testing.T) {
			// Create a record
			sk := ksuid.New().String()
			input := CreateInput{
				Repo:        "update-repo",
				Env:         "dev",
				SK:          sk,
				BuildNumber: "103",
				Branch:      "main",
				Version:     "103.jkl012",
				CommitHash:  "jkl012",
				StackName:   "dev-update-repo",
			}

			created, err := dao.Create(ctx, input)
			assert.NoError(t, err)

			// Small delay to ensure different timestamp
			time.Sleep(10 * time.Millisecond)

			// Update to SUCCESS
			status := BuildStatusSuccess
			err = dao.UpdateStatus(ctx, UpdateInput{
				PK:     created.PK,
				SK:     created.SK,
				Status: &status,
			})
			assert.NoError(t, err)

			// Verify update
			found, err := dao.Find(ctx, created.GetID())
			assert.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, BuildStatusSuccess, found.Status)
			assert.NotNil(t, found.FinishedAt)
			assert.GreaterOrEqual(t, found.UpdatedAt, created.UpdatedAt)
		})

		// Test 6: UpdateStatus - Failed with error message
		t.Run("UpdateStatus_Failed", func(t *testing.T) {
			// Create a record
			sk := ksuid.New().String()
			input := CreateInput{
				Repo:        "fail-repo",
				Env:         "dev",
				SK:          sk,
				BuildNumber: "104",
				Branch:      "main",
				Version:     "104.mno345",
				CommitHash:  "mno345",
				StackName:   "dev-fail-repo",
			}

			created, err := dao.Create(ctx, input)
			assert.NoError(t, err)

			// Update to FAILED with error
			status := BuildStatusFailed
			errorMsg := "Deployment failed: timeout"
			err = dao.UpdateStatus(ctx, UpdateInput{
				PK:       created.PK,
				SK:       created.SK,
				Status:   &status,
				ErrorMsg: &errorMsg,
			})
			assert.NoError(t, err)

			// Verify update
			found, err := dao.Find(ctx, created.GetID())
			assert.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, BuildStatusFailed, found.Status)
			assert.NotNil(t, found.ErrorMsg)
			assert.Equal(t, errorMsg, *found.ErrorMsg)
			assert.NotNil(t, found.FinishedAt)
		})

		// Test 7: UpdateStatus - InProgress (no finishedAt)
		t.Run("UpdateStatus_InProgress", func(t *testing.T) {
			// Create a record
			sk := ksuid.New().String()
			input := CreateInput{
				Repo:        "progress-repo",
				Env:         "dev",
				SK:          sk,
				BuildNumber: "105",
				Branch:      "main",
				Version:     "105.pqr678",
				CommitHash:  "pqr678",
				StackName:   "dev-progress-repo",
			}

			created, err := dao.Create(ctx, input)
			assert.NoError(t, err)

			// Update to IN_PROGRESS
			status := BuildStatusInProgress
			err = dao.UpdateStatus(ctx, UpdateInput{
				PK:     created.PK,
				SK:     created.SK,
				Status: &status,
			})
			assert.NoError(t, err)

			// Verify update
			found, err := dao.Find(ctx, created.GetID())
			assert.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, BuildStatusInProgress, found.Status)
			assert.Nil(t, found.FinishedAt) // Should NOT be set for in-progress
		})

		// Test 8: Query by PK
		t.Run("Query", func(t *testing.T) {
			// Create multiple builds for same repo/env
			repo := "query-repo-" + ksuid.New().String()[:6]
			for i := 0; i < 3; i++ {
				input := CreateInput{
					Repo:        repo,
					Env:         "dev",
					SK:          ksuid.New().String(),
					BuildNumber: fmt.Sprintf("%d", 200+i),
					Branch:      "main",
					Version:     fmt.Sprintf("%d.abc", 200+i),
					CommitHash:  fmt.Sprintf("abc%d", i),
					StackName:   fmt.Sprintf("dev-%s", repo),
				}

				_, err := dao.Create(ctx, input)
				assert.NoError(t, err)
			}

			// Query all builds
			pk := NewPK(repo, "dev")
			records, err := dao.Query(ctx, pk)
			assert.NoError(t, err)
			assert.Len(t, records, 3)
		})

		// Test 9: QueryByRepoEnv
		t.Run("QueryByRepoEnv", func(t *testing.T) {
			// Create builds in multiple environments
			repo := "multi-env-repo-" + ksuid.New().String()[:6]
			environments := []string{"dev", "staging", "prod"}

			for _, env := range environments {
				input := CreateInput{
					Repo:        repo,
					Env:         env,
					SK:          ksuid.New().String(),
					BuildNumber: "300",
					Branch:      "main",
					Version:     "300.xyz",
					CommitHash:  "xyz123",
					StackName:   fmt.Sprintf("%s-%s", env, repo),
				}

				_, err := dao.Create(ctx, input)
				assert.NoError(t, err)
			}

			// Query only dev builds
			records, err := dao.QueryByRepoEnv(ctx, repo, "dev")
			assert.NoError(t, err)
			assert.Len(t, records, 1)
			assert.Equal(t, "dev", records[0].Env)

			// Query staging builds
			records, err = dao.QueryByRepoEnv(ctx, repo, "staging")
			assert.NoError(t, err)
			assert.Len(t, records, 1)
			assert.Equal(t, "staging", records[0].Env)
		})

		// Test 10: QueryLatestBuilds
		t.Run("QueryLatestBuilds", func(t *testing.T) {
			// Create builds for multiple repos in same environment
			env := "test-env-" + ksuid.New().String()[:6]
			repos := []string{"repo-a", "repo-b", "repo-c"}

			for _, repo := range repos {
				sk := ksuid.New().String()

				// Create build
				input := CreateInput{
					Repo:        repo,
					Env:         env,
					SK:          sk,
					BuildNumber: "400",
					Branch:      "main",
					Version:     "400.abc",
					CommitHash:  "abc",
					StackName:   fmt.Sprintf("%s-%s", env, repo),
				}

				_, err := dao.Create(ctx, input)
				assert.NoError(t, err)

				// Update status to trigger latest record creation
				pk := NewPK(repo, env)
				status := BuildStatusSuccess
				err = dao.UpdateStatus(ctx, UpdateInput{
					PK:     pk,
					SK:     sk,
					Status: &status,
				})
				assert.NoError(t, err)

				// Small delay to ensure different UpdatedAt
				time.Sleep(10 * time.Millisecond)
			}

			// Query latest builds
			latestBuilds, err := dao.QueryLatestBuilds(ctx, env)
			assert.NoError(t, err)
			assert.Len(t, latestBuilds, 3)

			// Verify sorted by UpdatedAt descending
			for i := 0; i < len(latestBuilds)-1; i++ {
				assert.GreaterOrEqual(t, latestBuilds[i].UpdatedAt, latestBuilds[i+1].UpdatedAt)
			}

			// Verify all repos are represented
			foundRepos := make(map[string]bool)
			for _, build := range latestBuilds {
				foundRepos[build.Repo] = true
			}
			for _, repo := range repos {
				assert.True(t, foundRepos[repo], "Expected repo %s in latest builds", repo)
			}
		})

		// Test 11: QueryLatestBuilds with multiple updates (latest should be most recent)
		t.Run("QueryLatestBuilds_MultipleUpdates", func(t *testing.T) {
			env := "multi-update-env-" + ksuid.New().String()[:6]
			repo := "multi-update-repo"

			// Create and update first build
			sk1 := ksuid.New().String()
			input1 := CreateInput{
				Repo:        repo,
				Env:         env,
				SK:          sk1,
				BuildNumber: "500",
				Branch:      "main",
				Version:     "500.abc",
				CommitHash:  "abc",
				StackName:   fmt.Sprintf("%s-%s", env, repo),
			}

			_, err := dao.Create(ctx, input1)
			assert.NoError(t, err)

			pk := NewPK(repo, env)
			status1 := BuildStatusSuccess
			err = dao.UpdateStatus(ctx, UpdateInput{
				PK:     pk,
				SK:     sk1,
				Status: &status1,
			})
			assert.NoError(t, err)

			time.Sleep(100 * time.Millisecond)

			// Create and update second build (should be latest)
			sk2 := ksuid.New().String()
			input2 := CreateInput{
				Repo:        repo,
				Env:         env,
				SK:          sk2,
				BuildNumber: "501",
				Branch:      "main",
				Version:     "501.def",
				CommitHash:  "def",
				StackName:   fmt.Sprintf("%s-%s", env, repo),
			}

			_, err = dao.Create(ctx, input2)
			assert.NoError(t, err)

			status2 := BuildStatusSuccess
			err = dao.UpdateStatus(ctx, UpdateInput{
				PK:     pk,
				SK:     sk2,
				Status: &status2,
			})
			assert.NoError(t, err)

			// Query latest - should only return one record per repo
			latestBuilds, err := dao.QueryLatestBuilds(ctx, env)
			assert.NoError(t, err)
			assert.Len(t, latestBuilds, 1)
			assert.Equal(t, repo, latestBuilds[0].Repo)
			assert.Equal(t, BuildStatusSuccess, latestBuilds[0].Status)
		})

		// Test 12: ParsePK and ParseID edge cases
		t.Run("ParsePK_ParseID", func(t *testing.T) {
			// Valid PK
			repo, env, err := ParsePK(NewPK("myrepo", "dev"))
			assert.NoError(t, err)
			assert.Equal(t, "myrepo", repo)
			assert.Equal(t, "dev", env)

			// Invalid PK
			_, _, err = ParsePK(PK("invalid"))
			assert.Error(t, err)

			// Valid ID
			testPK := NewPK("myrepo", "dev")
			testSK := "2HFj3kLmNoPqRsTuVwXy"
			testID := NewID(testPK, testSK)

			parsedPK, parsedSK, err := ParseID(testID)
			assert.NoError(t, err)
			assert.Equal(t, testPK, parsedPK)
			assert.Equal(t, testSK, parsedSK)

			// Invalid ID
			_, _, err = ParseID(ID("invalid"))
			assert.Error(t, err)
		})
	})
}
