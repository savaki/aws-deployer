package gql

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/segmentio/ksuid"
)

// Promote resolves the promote mutation - promotes a build to downstream environments
// Returns the Query type to allow chaining queries after the mutation
func (r *Resolver) Promote(ctx context.Context, args struct{ BuildId string }) (*Resolver, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().Str("buildId", args.BuildId).Msg("Promote mutation called")

	// Parse buildId to get the build
	id := builddao.ID(args.BuildId)
	build, err := r.build.Find(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
	}

	// Validate build status - only allow promotion of successful builds
	if build.Status != builddao.BuildStatusSuccess {
		return nil, fmt.Errorf("cannot promote build with status %s - only SUCCESS builds can be promoted", build.Status)
	}

	// Get downstream environments from targetdao
	targets, err := r.targetDAO.GetWithDefault(ctx, build.Repo, build.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to get targets: %w", err)
	}

	// Check if there are downstream environments configured
	if targets == nil || len(targets.DownstreamEnv) == 0 {
		return nil, fmt.Errorf("no downstream environments configured for %s/%s", build.Repo, build.Env)
	}

	logger.Info().
		Str("repo", build.Repo).
		Str("env", build.Env).
		Strs("downstreamEnvs", targets.DownstreamEnv).
		Str("version", build.Version).
		Msg("Promoting build to downstream environments")

	// Create a new build for each downstream environment
	for _, downstreamEnv := range targets.DownstreamEnv {
		// Generate new KSUID for the promoted build
		sk := ksuid.New().String()

		// Create stack name for downstream environment
		stackName := fmt.Sprintf("%s-%s", downstreamEnv, build.Repo)

		// Create new build record in downstream environment
		_, err := r.build.Create(ctx, builddao.CreateInput{
			Repo:        build.Repo,
			Env:         downstreamEnv,
			SK:          sk,
			BuildNumber: build.BuildNumber,
			Branch:      build.Branch,
			Version:     build.Version,
			CommitHash:  build.CommitHash,
			StackName:   stackName,
		})
		if err != nil {
			logger.Error().
				Err(err).
				Str("repo", build.Repo).
				Str("downstreamEnv", downstreamEnv).
				Msg("Failed to create promoted build")
			return nil, fmt.Errorf("failed to create promoted build for %s: %w", downstreamEnv, err)
		}

		logger.Info().
			Str("repo", build.Repo).
			Str("env", downstreamEnv).
			Str("version", build.Version).
			Str("sk", sk).
			Msg("Successfully created promoted build")
	}

	// Return the root resolver to allow query chaining
	return r, nil
}
