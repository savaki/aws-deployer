package commands

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
)

func SetupSigningCommand(logger *zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "setup-signing",
		Usage: "Configure code signing verification settings",
		Description: `Configure signature verification for Lambda functions and container images.

This command sets up SSM parameters that control:
  - Whether signature verification is enabled
  - Enforcement mode (warn vs enforce)
  - Signing profile names for verification

Verification is performed by the verify-signatures Lambda function during deployments.

Enforcement modes:
  - warn: Log warnings for unsigned code but allow deployment (recommended for migration)
  - enforce: Block deployment if code is not properly signed

Examples:
  # Enable signature verification in dev environment (warn mode)
  aws-deployer setup-signing --env dev --enforcement-mode warn

  # Enable signature verification in production (enforce mode)
  aws-deployer setup-signing --env prod --enforcement-mode enforce

  # Disable signature verification
  aws-deployer setup-signing --env dev --lambda-verification disabled`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "env",
				Aliases:  []string{"e"},
				Usage:    "Environment name (dev, staging, prod)",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "region",
				Usage:   "AWS region",
				Value:   "us-east-1",
				EnvVars: []string{"AWS_REGION"},
			},
			&cli.StringFlag{
				Name:  "lambda-verification",
				Usage: "Lambda signature verification: enabled or disabled",
				Value: "enabled",
			},
			&cli.StringFlag{
				Name:  "container-verification",
				Usage: "Container signature verification: enabled or disabled",
				Value: "enabled",
			},
			&cli.StringFlag{
				Name:  "enforcement-mode",
				Usage: "Enforcement mode: warn or enforce",
				Value: "warn",
			},
			&cli.StringFlag{
				Name:  "lambda-profile-name",
				Usage: "AWS Signer profile name for Lambda signature verification (optional)",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be configured without making changes",
			},
		},
		Action: func(c *cli.Context) error {
			return setupSigningAction(c, logger)
		},
	}
}

func setupSigningAction(c *cli.Context, logger *zerolog.Logger) error {
	ctx := context.Background()

	env := c.String("env")
	region := c.String("region")
	lambdaVerification := c.String("lambda-verification")
	containerVerification := c.String("container-verification")
	enforcementMode := c.String("enforcement-mode")
	lambdaProfileName := c.String("lambda-profile-name")
	dryRun := c.Bool("dry-run")

	// Validate inputs
	if lambdaVerification != "enabled" && lambdaVerification != "disabled" {
		return fmt.Errorf("lambda-verification must be 'enabled' or 'disabled'")
	}
	if containerVerification != "enabled" && containerVerification != "disabled" {
		return fmt.Errorf("container-verification must be 'enabled' or 'disabled'")
	}
	if enforcementMode != "warn" && enforcementMode != "enforce" {
		return fmt.Errorf("enforcement-mode must be 'warn' or 'enforce'")
	}

	// Show configuration
	logger.Info().Msg("Signature Verification Configuration")
	logger.Info().Msgf("Environment:             %s", env)
	logger.Info().Msgf("Lambda Verification:     %s", lambdaVerification)
	logger.Info().Msgf("Container Verification:  %s", containerVerification)
	logger.Info().Msgf("Enforcement Mode:        %s", enforcementMode)
	if lambdaProfileName != "" {
		logger.Info().Msgf("Lambda Profile Name:     %s", lambdaProfileName)
	}
	logger.Info().Msg("")

	if dryRun {
		logger.Info().Msg("DRY RUN: Would configure the following SSM parameters:")
		logger.Info().Msgf("  /%s/aws-deployer/signing/lambda-verification = %s", env, lambdaVerification)
		logger.Info().Msgf("  /%s/aws-deployer/signing/container-verification = %s", env, containerVerification)
		logger.Info().Msgf("  /%s/aws-deployer/signing/enforcement-mode = %s", env, enforcementMode)
		if lambdaProfileName != "" {
			logger.Info().Msgf("  /%s/aws-deployer/signing/lambda-profile-name = %s", env, lambdaProfileName)
		}
		return nil
	}

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	ssmClient := ssm.NewFromConfig(cfg)

	// Store parameters in SSM
	parameters := map[string]string{
		fmt.Sprintf("/%s/aws-deployer/signing/lambda-verification", env):    lambdaVerification,
		fmt.Sprintf("/%s/aws-deployer/signing/container-verification", env): containerVerification,
		fmt.Sprintf("/%s/aws-deployer/signing/enforcement-mode", env):       enforcementMode,
	}

	if lambdaProfileName != "" {
		parameters[fmt.Sprintf("/%s/aws-deployer/signing/lambda-profile-name", env)] = lambdaProfileName
	}

	logger.Info().Msg("Storing configuration in SSM Parameter Store...")

	for path, value := range parameters {
		logger.Info().Msgf("  Setting %s = %s", path, value)

		_, err := ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
			Name:      aws.String(path),
			Value:     aws.String(value),
			Type:      types.ParameterTypeString,
			Overwrite: aws.Bool(true),
			Description: aws.String(fmt.Sprintf("Code signing configuration for %s environment", env)),
		})
		if err != nil {
			return fmt.Errorf("failed to store parameter %s: %w", path, err)
		}
	}

	logger.Info().Msg("")
	logger.Info().Msg("========================================")
	logger.Info().Msg("Signing Configuration Complete!")
	logger.Info().Msg("========================================")
	logger.Info().Msgf("Environment: %s", env)
	logger.Info().Msg("")
	logger.Info().Msg("Configuration:")
	logger.Info().Msgf("  Lambda Verification:     %s", lambdaVerification)
	logger.Info().Msgf("  Container Verification:  %s", containerVerification)
	logger.Info().Msgf("  Enforcement Mode:        %s", enforcementMode)
	logger.Info().Msg("")

	if enforcementMode == "warn" {
		logger.Info().Msg("⚠️  Enforcement mode is 'warn' - unsigned code will be logged but allowed.")
		logger.Info().Msg("   Use --enforcement-mode enforce to block unsigned deployments.")
	} else {
		logger.Info().Msg("✓ Enforcement mode is 'enforce' - unsigned code will block deployment.")
	}

	logger.Info().Msg("")
	logger.Info().Msg("Next steps:")
	logger.Info().Msg("  1. Update your GitHub Actions workflows to sign artifacts")
	logger.Info().Msg("  2. See docs/GITHUB_ACTIONS_SIGNING.md for examples")
	logger.Info().Msg("  3. Deploy aws-deployer with updated Lambda functions")
	logger.Info().Msg("  4. Test with a signed deployment")

	return nil
}
