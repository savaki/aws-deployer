package lockdao

import (
	"context"
	"fmt"
	"testing"
	"time"

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
		tableName = fmt.Sprintf("locks-test-%v", ksuid.New().String())
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

		// Test 1: Acquire lock when none exists
		t.Run("Acquire_Success", func(t *testing.T) {
			env := "acquire-env"
			repo := "acquire-repo"
			buildID := ksuid.New().String()
			executionArn := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID)

			record, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)
			assert.NotNil(t, record)

			id := NewID(env, repo)

			// Verify lock was created
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, lock)
			assert.Equal(t, buildID, lock.BuildID)
			assert.Equal(t, executionArn, lock.ExecutionArn)
			assert.Equal(t, fmt.Sprintf("%s/%s:LOCK", env, repo), lock.GetID().String())
			assert.NotZero(t, lock.AcquiredAt)
			assert.NotZero(t, lock.TTL)
			assert.Greater(t, lock.TTL, lock.AcquiredAt) // TTL should be in future
		})

		// Test 2: Try to acquire when lock already held by another build
		t.Run("Acquire_Conflict", func(t *testing.T) {
			env := "conflict-env"
			repo := "conflict-repo"
			buildID1 := ksuid.New().String()
			buildID2 := ksuid.New().String()
			executionArn1 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID1)
			executionArn2 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID2)

			// Build 1 acquires lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID1,
				ExecutionArn: executionArn1,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Build 2 tries to acquire (should fail)
			_, acquired, err = dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID2,
				ExecutionArn: executionArn2,
			})
			assert.NoError(t, err)
			assert.False(t, acquired)

			// Verify lock still held by build 1
			id := NewID(env, repo)
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, lock)
			assert.Equal(t, buildID1, lock.BuildID)
		})

		// Test 3: Idempotent acquisition (same build acquires again)
		t.Run("Acquire_Idempotent", func(t *testing.T) {
			env := "idempotent-env"
			repo := "idempotent-repo"
			buildID := ksuid.New().String()
			executionArn := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID)

			input := AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			}

			// First acquisition
			_, acquired, err := dao.Acquire(ctx, input)
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Same build tries to acquire again (retry scenario)
			_, acquired, err = dao.Acquire(ctx, input)
			assert.NoError(t, err)
			assert.True(t, acquired) // Should succeed (idempotent)
		})

		// Test 4: Find lock info
		t.Run("Find", func(t *testing.T) {
			env := "find-env"
			repo := "find-repo"
			buildID := ksuid.New().String()
			executionArn := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID)

			// Acquire lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Find lock info
			id := NewID(env, repo)
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, lock)
			assert.Equal(t, env+"/"+repo, lock.PK.String())
			assert.Equal(t, "LOCK", lock.SK)
			assert.Equal(t, buildID, lock.BuildID)
			assert.Equal(t, executionArn, lock.ExecutionArn)
		})

		// Test 5: Find when no lock exists
		t.Run("Find_NoLock", func(t *testing.T) {
			id := NewID("no-lock-env", "no-lock-repo")
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Nil(t, lock)
		})

		// Test 6: Release lock
		t.Run("Release_Success", func(t *testing.T) {
			env := "release-env"
			repo := "release-repo"
			buildID := ksuid.New().String()
			executionArn := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID)

			// Acquire lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			id := NewID(env, repo)

			// Release lock
			err = dao.Release(ctx, ReleaseInput{
				ID:      id,
				BuildID: buildID,
			})
			assert.NoError(t, err)

			// Verify lock is gone
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Nil(t, lock)
		})

		// Test 7: Release when not lock holder
		t.Run("Release_NotHolder", func(t *testing.T) {
			env := "wrong-release-env"
			repo := "wrong-release-repo"
			buildID1 := ksuid.New().String()
			buildID2 := ksuid.New().String()
			executionArn1 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID1)

			// Build 1 acquires lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID1,
				ExecutionArn: executionArn1,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			id := NewID(env, repo)

			// Build 2 tries to release (should fail)
			err = dao.Release(ctx, ReleaseInput{
				ID:      id,
				BuildID: buildID2,
			})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "lock not held by build")

			// Verify lock still held by build 1
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, lock)
			assert.Equal(t, buildID1, lock.BuildID)
		})

		// Test 8: Release when no lock exists (idempotent)
		t.Run("Release_NoLock", func(t *testing.T) {
			id := NewID("no-lock", "no-lock")
			err := dao.Release(ctx, ReleaseInput{
				ID:      id,
				BuildID: ksuid.New().String(),
			})
			assert.NoError(t, err) // Should be idempotent (no error)
		})

		// Test 9: ForceRelease via Delete regardless of holder
		t.Run("ForceDelete", func(t *testing.T) {
			env := "force-env"
			repo := "force-repo"
			buildID := ksuid.New().String()
			executionArn := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID)

			id := NewID(env, repo)

			// Acquire lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Force delete (emergency cleanup - bypasses build ID check)
			err = dao.Delete(ctx, id)
			assert.NoError(t, err)

			// Verify lock is gone
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Nil(t, lock)
		})

		// Test 10: Lock lifecycle (acquire → release → re-acquire)
		t.Run("Lifecycle", func(t *testing.T) {
			env := "lifecycle-env"
			repo := "lifecycle-repo"
			buildID1 := ksuid.New().String()
			buildID2 := ksuid.New().String()
			executionArn1 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID1)
			executionArn2 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID2)

			id := NewID(env, repo)

			// Build 1 acquires lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID1,
				ExecutionArn: executionArn1,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Build 2 cannot acquire
			_, acquired, err = dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID2,
				ExecutionArn: executionArn2,
			})
			assert.NoError(t, err)
			assert.False(t, acquired)

			// Build 1 releases lock
			err = dao.Release(ctx, ReleaseInput{
				ID:      id,
				BuildID: buildID1,
			})
			assert.NoError(t, err)

			// Now build 2 can acquire
			_, acquired, err = dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID2,
				ExecutionArn: executionArn2,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Verify build 2 holds lock
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, lock)
			assert.Equal(t, buildID2, lock.BuildID)
		})

		// Test 11: Multiple repos/envs with locks
		t.Run("MultipleLocksIsolation", func(t *testing.T) {
			// Different repos should have independent locks
			buildID1 := ksuid.New().String()
			buildID2 := ksuid.New().String()
			executionArn1 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID1)
			executionArn2 := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID2)

			// Acquire lock for repo-a/dev
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          "dev",
				Repo:         "repo-a",
				BuildID:      buildID1,
				ExecutionArn: executionArn1,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Acquire lock for repo-b/dev (different repo, should succeed)
			_, acquired, err = dao.Acquire(ctx, AcquireInput{
				Env:          "dev",
				Repo:         "repo-b",
				BuildID:      buildID2,
				ExecutionArn: executionArn2,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Verify both locks exist independently
			idA := NewID("dev", "repo-a")
			lockA, err := dao.Find(ctx, idA)
			assert.NoError(t, err)
			assert.NotNil(t, lockA)
			assert.Equal(t, buildID1, lockA.BuildID)

			idB := NewID("dev", "repo-b")
			lockB, err := dao.Find(ctx, idB)
			assert.NoError(t, err)
			assert.NotNil(t, lockB)
			assert.Equal(t, buildID2, lockB.BuildID)
		})

		// Test 12: TTL field is set correctly
		t.Run("TTL_FieldSet", func(t *testing.T) {
			env := "ttl-env"
			repo := "ttl-repo"
			buildID := ksuid.New().String()
			executionArn := fmt.Sprintf("arn:aws:states:us-east-1:123456789012:execution:test:%s", buildID)

			beforeAcquire := time.Now().Unix()

			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			id := NewID(env, repo)
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, lock)

			// TTL should be 4 hours in future
			expectedTTL := beforeAcquire + (4 * 3600)
			assert.GreaterOrEqual(t, lock.TTL, expectedTTL-5) // Allow 5 second tolerance
			assert.LessOrEqual(t, lock.TTL, expectedTTL+5)

			// AcquiredAt should be recent
			assert.GreaterOrEqual(t, lock.AcquiredAt, beforeAcquire)
			assert.LessOrEqual(t, lock.AcquiredAt, time.Now().Unix()+1)
		})

		// Test 13: ID and PK format
		t.Run("ID_PK_Format", func(t *testing.T) {
			pk := NewPK("my-env", "my-repo")
			assert.Equal(t, "my-env/my-repo", pk.String())

			id := NewID("my-env", "my-repo")
			assert.Equal(t, "my-env/my-repo:LOCK", id.String())

			// Acquire lock and verify formats in record
			buildID := ksuid.New().String()
			executionArn := "arn:test"

			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          "my-env",
				Repo:         "my-repo",
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, "my-env/my-repo", lock.PK.String())
			assert.Equal(t, "LOCK", lock.SK)
			assert.Equal(t, "my-env/my-repo:LOCK", lock.GetID().String())
		})

		// Test 14: Concurrent acquisition attempts (race condition simulation)
		t.Run("ConcurrentAcquisition", func(t *testing.T) {
			env := "concurrent-env"
			repo := "concurrent-repo"
			buildID1 := ksuid.New().String()
			buildID2 := ksuid.New().String()
			executionArn1 := "arn:test1"
			executionArn2 := "arn:test2"

			// Simulate concurrent attempts
			// Note: Without true concurrency, we'll test sequential behavior
			_, acquired1, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID1,
				ExecutionArn: executionArn1,
			})
			assert.NoError(t, err)
			assert.True(t, acquired1)

			// Immediate second attempt (simulating race)
			_, acquired2, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID2,
				ExecutionArn: executionArn2,
			})
			assert.NoError(t, err)
			assert.False(t, acquired2)

			// Only build 1 should hold lock
			id := NewID(env, repo)
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, buildID1, lock.BuildID)
		})

		// Test 15: Release and re-acquire workflow
		t.Run("ReleaseAndReacquire", func(t *testing.T) {
			env := "reacquire-env"
			repo := "reacquire-repo"
			buildID1 := ksuid.New().String()
			buildID2 := ksuid.New().String()
			executionArn1 := "arn:test1"
			executionArn2 := "arn:test2"

			id := NewID(env, repo)

			// Build 1 acquires
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID1,
				ExecutionArn: executionArn1,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Build 1 completes and releases
			err = dao.Release(ctx, ReleaseInput{
				ID:      id,
				BuildID: buildID1,
			})
			assert.NoError(t, err)

			// Build 2 can now acquire
			_, acquired, err = dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID2,
				ExecutionArn: executionArn2,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Verify build 2 holds lock
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Equal(t, buildID2, lock.BuildID)
		})

		// Test 16: Delete (force release) emergency cleanup
		t.Run("Delete_Emergency", func(t *testing.T) {
			env := "emergency-env"
			repo := "emergency-repo"
			buildID := ksuid.New().String()
			executionArn := "arn:test"

			id := NewID(env, repo)

			// Acquire lock
			_, acquired, err := dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      buildID,
				ExecutionArn: executionArn,
			})
			assert.NoError(t, err)
			assert.True(t, acquired)

			// Force delete (emergency cleanup)
			err = dao.Delete(ctx, id)
			assert.NoError(t, err)

			// Verify lock is gone
			lock, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Nil(t, lock)

			// Should be able to acquire now
			newBuildID := ksuid.New().String()
			_, acquired, err = dao.Acquire(ctx, AcquireInput{
				Env:          env,
				Repo:         repo,
				BuildID:      newBuildID,
				ExecutionArn: "arn:new",
			})
			assert.NoError(t, err)
			assert.True(t, acquired)
		})

		// Test 17: Multiple environments, same repo
		t.Run("MultipleEnvironments", func(t *testing.T) {
			repo := "multi-env-lock-repo"
			envs := []string{"dev", "staging", "prod"}

			// Each environment should have independent locks
			for _, env := range envs {
				buildID := ksuid.New().String()
				executionArn := "arn:test:" + env

				_, acquired, err := dao.Acquire(ctx, AcquireInput{
					Env:          env,
					Repo:         repo,
					BuildID:      buildID,
					ExecutionArn: executionArn,
				})
				assert.NoError(t, err)
				assert.True(t, acquired)
			}

			// Verify all locks exist independently
			for _, env := range envs {
				id := NewID(env, repo)
				lock, err := dao.Find(ctx, id)
				assert.NoError(t, err)
				assert.NotNil(t, lock)
				assert.Equal(t, fmt.Sprintf("%s/%s", env, repo), lock.PK.String())
				assert.Equal(t, fmt.Sprintf("%s/%s:LOCK", env, repo), lock.GetID().String())
			}
		})
	})
}
