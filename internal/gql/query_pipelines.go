package gql

import (
	"context"
	"sort"

	"github.com/savaki/aws-deployer/internal/dao/targetdao"
)

// Pipelines resolves the pipelines query - returns all pipeline configurations (default + per-repo)
func (r *Resolver) Pipelines(ctx context.Context) ([]*PipelineConfigResolver, error) {
	// Scan all records from the targets table
	records, err := r.targetDAO.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	// Group records by repo
	repoMap := make(map[string]struct {
		config       *targetdao.Record
		environments []*targetdao.Record
	})

	for _, record := range records {
		repo := record.PK.String()
		entry := repoMap[repo]

		if record.SK == targetdao.ConfigEnv {
			// This is a config record
			entry.config = record
		} else {
			// This is an environment record
			entry.environments = append(entry.environments, record)
		}

		repoMap[repo] = entry
	}

	// Build pipeline config resolvers
	var resolvers []*PipelineConfigResolver

	// Collect all repo names
	var repos []string
	for repo := range repoMap {
		repos = append(repos, repo)
	}

	// Sort: default first, then alphabetical
	sort.Slice(repos, func(i, j int) bool {
		if repos[i] == targetdao.DefaultRepo {
			return true
		}
		if repos[j] == targetdao.DefaultRepo {
			return false
		}
		return repos[i] < repos[j]
	})

	// Create resolvers in sorted order
	for _, repo := range repos {
		entry := repoMap[repo]

		// Determine initial env (fallback to "dev" if not set)
		initialEnv := "dev"
		if entry.config != nil && entry.config.InitialEnv != "" {
			initialEnv = entry.config.InitialEnv
		}

		// Sort environments by a standard order (dev, stg, prd, then alphabetical)
		sortEnvironments(entry.environments)

		resolvers = append(resolvers, newPipelineConfigResolver(repo, initialEnv, entry.environments))
	}

	return resolvers, nil
}

// sortEnvironments sorts environment records by standard order
func sortEnvironments(envs []*targetdao.Record) {
	envOrder := map[string]int{
		"dev": 0,
		"stg": 1,
		"prd": 2,
	}

	sort.Slice(envs, func(i, j int) bool {
		envI := envs[i].SK
		envJ := envs[j].SK

		orderI, okI := envOrder[envI]
		orderJ, okJ := envOrder[envJ]

		// If both are standard envs, sort by order
		if okI && okJ {
			return orderI < orderJ
		}

		// If only i is standard, it comes first
		if okI {
			return true
		}

		// If only j is standard, it comes first
		if okJ {
			return false
		}

		// Both are custom envs, sort alphabetically
		return envI < envJ
	})
}
