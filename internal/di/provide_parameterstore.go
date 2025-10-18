package di

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/services"
)

// ProvideSSMClient provides an SSM client for Parameter Store access
// Returns nil if SSM is disabled (for local development)
func ProvideSSMClient(awsConfig aws.Config) *ssm.Client {
	// Check if SSM should be disabled (local development)
	if os.Getenv("DISABLE_SSM") == "true" {
		return nil
	}

	return ssm.NewFromConfig(awsConfig)
}

// ProvideParameterStore provides a ParameterStore implementation
// Uses SSM Parameter Store in AWS, falls back to environment variables when disabled
func ProvideParameterStore(ctx context.Context, ssmClient *ssm.Client, env string) services.ParameterStore {
	logger := zerolog.Ctx(ctx)

	if ssmClient == nil {
		logger.Info().Msg("Using environment variables for configuration (SSM disabled)")
		return services.NewEnvParameterStore(env)
	}

	logger.Info().Msg("Using AWS Systems Manager Parameter Store for configuration")
	return services.NewSSMParameterStore(ssmClient, env)
}

// ProvideAppConfig loads application configuration from Parameter Store or environment variables
func ProvideAppConfig(ctx context.Context, store services.ParameterStore) (*services.Config, error) {
	logger := zerolog.Ctx(ctx)

	config, err := store.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	logger.Info().
		Str("deployment_mode", config.DeploymentMode).
		Str("s3_bucket", config.S3Bucket).
		Bool("has_allowed_email", config.AllowedEmail != "").
		Bool("has_custom_domain", config.CustomDomain != "").
		Msg("Configuration loaded successfully")

	return config, nil
}
