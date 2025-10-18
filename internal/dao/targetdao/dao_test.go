package targetdao

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
		tableName = fmt.Sprintf("targets-test-%v", ksuid.New().String())
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

		// Test 1: Create and Find default targets
		t.Run("Create_Find_Default", func(t *testing.T) {
			targets := []Target{
				{
					AccountIDs: []string{"111111111111", "222222222222"},
					Regions:    []string{"us-east-1", "us-west-2"},
				},
			}

			created, err := dao.Create(ctx, CreateInput{
				Repo:    "$",
				Env:     "dev",
				Targets: targets,
			})
			assert.NoError(t, err)
			assert.NotNil(t, created)

			// Find default targets
			id := NewID("$", "dev")
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, "$", record.PK.String())
			assert.Equal(t, "dev", record.SK)
			assert.Equal(t, "$:dev", record.GetID().String())
			assert.Len(t, record.Targets, 1)
			assert.Len(t, record.Targets[0].AccountIDs, 2)
			assert.Len(t, record.Targets[0].Regions, 2)
		})

		// Test 2: Create and Find repo-specific targets
		t.Run("Create_Find_RepoSpecific", func(t *testing.T) {
			targets := []Target{
				{
					AccountIDs: []string{"333333333333"},
					Regions:    []string{"eu-west-1"},
				},
			}

			created, err := dao.Create(ctx, CreateInput{
				Repo:    "my-repo",
				Env:     "prod",
				Targets: targets,
			})
			assert.NoError(t, err)
			assert.NotNil(t, created)

			id := NewID("my-repo", "prod")
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, "my-repo", record.PK.String())
			assert.Equal(t, "prod", record.SK)
			assert.Equal(t, "my-repo:prod", record.GetID().String())
			assert.Len(t, record.Targets, 1)
			assert.Equal(t, "333333333333", record.Targets[0].AccountIDs[0])
		})

		// Test 3: Find non-existent targets
		t.Run("Find_NotFound", func(t *testing.T) {
			id := NewID("non-existent", "dev")
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Nil(t, record)
		})

		// Test 4: GetWithDefault fallback
		t.Run("GetWithDefault_FallbackToDefault", func(t *testing.T) {
			// Put only default targets
			defaultTargets := []Target{
				{
					AccountIDs: []string{"999999999999"},
					Regions:    []string{"ap-south-1"},
				},
			}
			err := dao.Put(ctx, "$", "staging", defaultTargets)
			assert.NoError(t, err)

			// Get for non-existent repo should fall back to default
			record, err := dao.GetWithDefault(ctx, "some-repo", "staging")
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, "$", record.PK.String())
			assert.Equal(t, "999999999999", record.Targets[0].AccountIDs[0])
		})

		// Test 5: GetWithDefault uses repo-specific when available
		t.Run("GetWithDefault_UsesRepoSpecific", func(t *testing.T) {
			// Put default targets
			defaultTargets := []Target{
				{
					AccountIDs: []string{"111111111111"},
					Regions:    []string{"us-east-1"},
				},
			}
			err := dao.Put(ctx, "$", "test-env", defaultTargets)
			assert.NoError(t, err)

			// Put repo-specific targets
			repoTargets := []Target{
				{
					AccountIDs: []string{"222222222222"},
					Regions:    []string{"us-west-2"},
				},
			}
			err = dao.Put(ctx, "specific-repo", "test-env", repoTargets)
			assert.NoError(t, err)

			// GetWithDefault should return repo-specific, not default
			record, err := dao.GetWithDefault(ctx, "specific-repo", "test-env")
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, "specific-repo", record.PK.String())
			assert.Equal(t, "222222222222", record.Targets[0].AccountIDs[0])
		})

		// Test 6: Delete
		t.Run("Delete", func(t *testing.T) {
			targets := []Target{
				{
					AccountIDs: []string{"444444444444"},
					Regions:    []string{"ca-central-1"},
				},
			}

			created, err := dao.Create(ctx, CreateInput{
				Repo:    "delete-repo",
				Env:     "dev",
				Targets: targets,
			})
			assert.NoError(t, err)

			id := created.GetID()

			// Verify it exists
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, record)

			// Delete it
			err = dao.Delete(ctx, id)
			assert.NoError(t, err)

			// Verify it's gone
			record, err = dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.Nil(t, record)
		})

		// Test 7: Update targets
		t.Run("Update", func(t *testing.T) {
			// Initial targets
			initialTargets := []Target{
				{
					AccountIDs: []string{"111111111111"},
					Regions:    []string{"us-east-1"},
				},
			}
			created, err := dao.Create(ctx, CreateInput{
				Repo:    "update-repo",
				Env:     "dev",
				Targets: initialTargets,
			})
			assert.NoError(t, err)

			id := created.GetID()

			// Update with new targets
			newTargets := []Target{
				{
					AccountIDs: []string{"222222222222", "333333333333"},
					Regions:    []string{"us-west-1", "us-west-2"},
				},
			}
			updated, err := dao.Update(ctx, UpdateInput{
				ID:      id,
				Targets: newTargets,
			})
			assert.NoError(t, err)
			assert.NotNil(t, updated)

			// Verify update
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Len(t, record.Targets, 1)
			assert.Len(t, record.Targets[0].AccountIDs, 2)
			assert.Len(t, record.Targets[0].Regions, 2)
		})

		// Test 8: ExpandTargets
		t.Run("ExpandTargets", func(t *testing.T) {
			targets := []Target{
				{
					AccountIDs: []string{"111111111111", "222222222222"},
					Regions:    []string{"us-east-1", "us-west-2"},
				},
			}

			expanded := ExpandTargets(targets)
			assert.Len(t, expanded, 4) // 2 accounts × 2 regions = 4

			// Verify all combinations exist
			expected := map[string]bool{
				"111111111111-us-east-1": false,
				"111111111111-us-west-2": false,
				"222222222222-us-east-1": false,
				"222222222222-us-west-2": false,
			}

			for _, item := range expanded {
				key := fmt.Sprintf("%s-%s", item.AccountID, item.Region)
				expected[key] = true
			}

			for key, found := range expected {
				assert.True(t, found, "Missing combination: %s", key)
			}
		})

		// Test 9: ExpandTargets with multiple target groups
		t.Run("ExpandTargets_MultipleGroups", func(t *testing.T) {
			targets := []Target{
				{
					AccountIDs: []string{"111111111111"},
					Regions:    []string{"us-east-1"},
				},
				{
					AccountIDs: []string{"222222222222"},
					Regions:    []string{"eu-west-1", "ap-south-1"},
				},
			}

			expanded := ExpandTargets(targets)
			// Group 1: 1 account × 1 region = 1
			// Group 2: 1 account × 2 regions = 2
			// Total: 3
			assert.Len(t, expanded, 3)
		})

		// Test 10: Multiple environments
		t.Run("MultipleEnvironments", func(t *testing.T) {
			repo := "multi-env-repo"
			envs := []string{"dev", "staging", "prod"}

			// Create targets for each environment
			for i, env := range envs {
				targets := []Target{
					{
						AccountIDs: []string{fmt.Sprintf("%d%d%d%d%d%d%d%d%d%d%d%d", i, i, i, i, i, i, i, i, i, i, i, i)},
						Regions:    []string{fmt.Sprintf("region-%d", i)},
					},
				}
				err := dao.Put(ctx, repo, env, targets)
				assert.NoError(t, err)
			}

			// Verify each environment has correct targets
			for _, env := range envs {
				record, err := dao.Get(ctx, repo, env)
				assert.NoError(t, err)
				assert.NotNil(t, record)
				assert.Equal(t, env, record.SK)
			}
		})

		// Test 11: SetConfig and GetConfig for default
		t.Run("Config_Default", func(t *testing.T) {
			// Set default initial env
			record, err := dao.SetConfig(ctx, "$", "stg")
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, "$", record.PK.String())
			assert.Equal(t, "$", record.SK)
			assert.Equal(t, "stg", record.InitialEnv)

			// Get config
			config, err := dao.GetConfig(ctx, "$")
			assert.NoError(t, err)
			assert.NotNil(t, config)
			assert.Equal(t, "stg", config.InitialEnv)
		})

		// Test 12: SetConfig and GetConfig for repo
		t.Run("Config_Repo", func(t *testing.T) {
			// Set repo-specific initial env
			record, err := dao.SetConfig(ctx, "my-app", "prd")
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, "my-app", record.PK.String())
			assert.Equal(t, "$", record.SK)
			assert.Equal(t, "prd", record.InitialEnv)

			// Get config
			config, err := dao.GetConfig(ctx, "my-app")
			assert.NoError(t, err)
			assert.NotNil(t, config)
			assert.Equal(t, "prd", config.InitialEnv)
		})

		// Test 13: GetInitialEnv with repo-specific config
		t.Run("GetInitialEnv_RepoSpecific", func(t *testing.T) {
			// Set repo-specific initial env
			_, err := dao.SetConfig(ctx, "test-repo", "stg")
			assert.NoError(t, err)

			// Get initial env
			initialEnv, err := dao.GetInitialEnv(ctx, "test-repo")
			assert.NoError(t, err)
			assert.Equal(t, "stg", initialEnv)
		})

		// Test 14: GetInitialEnv fallback to default
		t.Run("GetInitialEnv_FallbackToDefault", func(t *testing.T) {
			// Set only default initial env
			_, err := dao.SetConfig(ctx, "$", "prd")
			assert.NoError(t, err)

			// Get initial env for repo without specific config
			initialEnv, err := dao.GetInitialEnv(ctx, "no-config-repo")
			assert.NoError(t, err)
			assert.Equal(t, "prd", initialEnv)
		})

		// Test 15: GetInitialEnv ultimate fallback
		t.Run("GetInitialEnv_UltimateFallback", func(t *testing.T) {
			// Delete default config if it exists (from previous tests)
			defaultConfigID := NewID(DefaultRepo, ConfigEnv)
			_ = dao.Delete(ctx, defaultConfigID)

			// Get initial env when nothing is configured
			initialEnv, err := dao.GetInitialEnv(ctx, "unconfigured-repo")
			assert.NoError(t, err)
			assert.Equal(t, "dev", initialEnv) // Should default to "dev"
		})

		// Test 16: DownstreamEnv
		t.Run("DownstreamEnv", func(t *testing.T) {
			targets := []Target{
				{
					AccountIDs: []string{"123456789012"},
					Regions:    []string{"us-east-1"},
				},
			}

			// Create with downstream env
			created, err := dao.Create(ctx, CreateInput{
				Repo:          "promo-repo",
				Env:           "dev",
				Targets:       targets,
				DownstreamEnv: []string{"stg"},
			})
			assert.NoError(t, err)
			assert.NotNil(t, created)
			assert.Equal(t, []string{"stg"}, created.DownstreamEnv)

			// Find and verify
			id := NewID("promo-repo", "dev")
			record, err := dao.Find(ctx, id)
			assert.NoError(t, err)
			assert.NotNil(t, record)
			assert.Equal(t, []string{"stg"}, record.DownstreamEnv)

			// Update downstream env
			updated, err := dao.Update(ctx, UpdateInput{
				ID:            id,
				Targets:       targets,
				DownstreamEnv: []string{"stg", "prd"},
			})
			assert.NoError(t, err)
			assert.Equal(t, []string{"stg", "prd"}, updated.DownstreamEnv)
		})
	})
}
