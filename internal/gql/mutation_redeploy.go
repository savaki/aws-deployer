package gql

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/orchestrator"
	"github.com/segmentio/ksuid"
)

// Redeploy resolves the redeploy mutation - triggers a redeploy of a specific build
// Returns the Query type to allow chaining queries after the mutation
func (r *Resolver) Redeploy(ctx context.Context, args struct{ BuildId string }) (*Resolver, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().Str("buildId", args.BuildId).Msg("Redeploy mutation called")

	// Parse buildId to get the build
	id := builddao.ID(args.BuildId)
	build, err := r.build.Find(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
	}

	// Generate new KSUID for the redeployment to avoid execution name conflicts
	sk := ksuid.New().String()

	logger.Info().
		Str("repo", build.Repo).
		Str("env", build.Env).
		Str("version", build.Version).
		Str("original_sk", build.SK).
		Str("new_sk", sk).
		Msg("Triggering redeploy for build")

	// Create a new build record for this redeploy with all fields from the original build
	pk := builddao.NewPK(build.Repo, build.Env)
	_, err = r.build.Create(ctx, builddao.CreateInput{
		Repo:        build.Repo,
		Env:         build.Env,
		SK:          sk,
		BuildNumber: build.BuildNumber,
		Branch:      build.Branch,
		Version:     build.Version,
		CommitHash:  build.CommitHash,
		StackName:   build.StackName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create build record for redeploy: %w", err)
	}

	// Construct Step Function input from build record
	input := orchestrator.StepFunctionInput{
		Repo:       build.Repo,
		Env:        build.Env,
		Branch:     build.Branch,
		Version:    build.Version,
		SK:         sk,
		CommitHash: build.CommitHash,
		S3Bucket:   r.appConfig.S3Bucket,
		S3Key:      fmt.Sprintf("%s/%s/%s", build.Repo, build.Branch, build.Version),
	}

	// Start Step Functions execution
	executionArn, err := r.orchestrator.StartExecution(ctx, input)
	if err != nil {
		// Update build status to FAILED using the new SK
		status := builddao.BuildStatusFailed
		errorMsg := fmt.Sprintf("Failed to start step function: %v", err)
		if updateErr := r.build.UpdateStatus(ctx, builddao.UpdateInput{
			PK:       pk,
			SK:       sk,
			Status:   &status,
			ErrorMsg: &errorMsg,
		}); updateErr != nil {
			logger.Error().Err(updateErr).Msg("Failed to update build status")
		}
		return nil, fmt.Errorf("failed to start execution: %w", err)
	}

	logger.Info().
		Str("execution_arn", executionArn).
		Str("repo", build.Repo).
		Str("env", build.Env).
		Str("sk", sk).
		Msg("Successfully started redeploy execution")

	// Return the root resolver to allow query chaining
	return r, nil
}
