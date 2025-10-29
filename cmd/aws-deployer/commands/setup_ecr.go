package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
)

func SetupECRCommand(logger *zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "setup-ecr",
		Usage: "Create ECR registries for repository",
		Description: `Create ECR registries with scan-on-push, tag immutability, and org-wide permissions.

Registry names can be any valid ECR repository name. Common patterns:
  - owner/repo (e.g., foo/bar)
  - owner/repo/component (e.g., foo/bar/backend)
  - Simple names (e.g., backend, frontend, api)

If the AWS account belongs to an organization, org-wide read permissions will be configured automatically.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "repo",
				Aliases:  []string{"r"},
				Usage:    "Repository in format 'owner/repo'",
				Required: true,
				EnvVars:  []string{"GITHUB_REPO"},
			},
			&cli.StringSliceFlag{
				Name:     "ecr-registry",
				Aliases:  []string{"e"},
				Usage:    "ECR registry name(s) to create (can be specified multiple times)",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "region",
				Usage:   "AWS region for ECR registries",
				Value:   "us-east-1",
				EnvVars: []string{"AWS_REGION"},
			},
			&cli.StringFlag{
				Name:    "role-name",
				Aliases: []string{"n"},
				Usage:   "IAM role name to grant ECR push permissions (typically the GitHub OIDC role)",
			},
			&cli.StringFlag{
				Name:    "env",
				Aliases: []string{"environment"},
				Usage:   "Environment name (dev, staging, prod) - stores registry allowlist in SSM for signature verification",
				Value:   "dev",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be created without creating resources",
			},
		},
		Action: func(c *cli.Context) error {
			return setupECRAction(c, logger)
		},
	}
}

func setupECRAction(c *cli.Context, logger *zerolog.Logger) error {
	ctx := context.Background()

	repo := c.String("repo")
	registries := c.StringSlice("ecr-registry")
	region := c.String("region")
	roleName := c.String("role-name")
	env := c.String("env")
	dryRun := c.Bool("dry-run")

	// Validate repository format
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: %q (expected 'owner/repo')", repo)
	}

	// Log registry names
	logger.Info().Msgf("Will create %d registry/registries...", len(registries))
	for _, registryName := range registries {
		logger.Info().Msgf("  - %s", registryName)
	}

	if dryRun {
		logger.Info().Msg("DRY RUN: Would create the following ECR registries:")
		for _, registryName := range registries {
			logger.Info().Msgf("  - %s (region: %s)", registryName, region)
		}
		logger.Info().Msg("DRY RUN: Would enable:")
		logger.Info().Msg("  - Scan on push")
		logger.Info().Msg("  - Tag immutability")
		logger.Info().Msg("DRY RUN: Would check for AWS Organization and set org-wide read permissions if applicable")
		if roleName != "" {
			logger.Info().Msgf("DRY RUN: Would add ECR push permissions to IAM role: %s", roleName)
		}
		ssmPath := fmt.Sprintf("/%s/aws-deployer/ecr-registries/%s", env, repo)
		logger.Info().Msgf("DRY RUN: Would store registry allowlist in SSM: %s", ssmPath)
		logger.Info().Msgf("DRY RUN: Registry list: %s", strings.Join(registries, ","))
		return nil
	}

	// Create ECR repositories
	logger.Info().Msgf("Initializing ECR service in region %s...", region)
	result, err := createECRRepositories(ctx, logger, region, registries)
	if err != nil {
		return err
	}

	// Get account ID for display
	ecrService, err := services.NewECRService(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to create ECR service: %w", err)
	}
	accountID, err := ecrService.GetAccountID(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get account ID")
		accountID = "unknown"
	}

	// Add ECR push permissions to IAM role if specified
	if err := addECRPermissionsToRole(ctx, logger, roleName, result.Repositories); err != nil {
		return err
	}

	// Store registry allowlist in SSM for signature verification
	if err := storeRegistryAllowlistInSSM(ctx, logger, env, repo, region, registries); err != nil {
		logger.Warn().Err(err).Msg("Failed to store registry allowlist in SSM - signature verification will not work")
		// Don't fail the command, just warn
	}

	// Summary
	logger.Info().Msg("")
	logger.Info().Msg("========================================")
	logger.Info().Msg("ECR Setup Complete!")
	logger.Info().Msg("========================================")
	logger.Info().Msgf("Region:      %s", region)
	logger.Info().Msgf("Account:     %s", accountID)
	logger.Info().Msgf("Registries:  %d created", len(result.Repositories))
	logger.Info().Msg("")
	logger.Info().Msg("Features enabled:")
	logger.Info().Msg("  ✓ Scan on push")
	logger.Info().Msg("  ✓ Tag immutability")
	if result.OrganizationID != "" {
		logger.Info().Msg("  ✓ Org-wide read permissions")
	}
	if roleName != "" {
		logger.Info().Msgf("  ✓ IAM role %s has ECR push permissions", roleName)
	}
	logger.Info().Msg("")
	logger.Info().Msg("Registry URIs:")
	for _, repo := range result.Repositories {
		logger.Info().Msgf("  %s", repo.URI)
	}
	logger.Info().Msg("")
	logger.Info().Msg("To push images:")
	logger.Info().Msgf("  1. Authenticate: aws ecr get-login-password --region %s | docker login --username AWS --password-stdin %s.dkr.ecr.%s.amazonaws.com", region, accountID, region)
	logger.Info().Msg("  2. Tag image: docker tag myimage:latest <registry-uri>:latest")
	logger.Info().Msg("  3. Push image: docker push <registry-uri>:latest")
	logger.Info().Msg("")
	logger.Info().Msgf("Registry allowlist stored in SSM: /%s/aws-deployer/ecr-registries/%s", env, repo)
	logger.Info().Msg("To enable signature verification, run: aws-deployer setup-signing")

	return nil
}

// storeRegistryAllowlistInSSM stores the list of allowed ECR registries in SSM Parameter Store
// This is used by the signature verification Lambda to validate container images
func storeRegistryAllowlistInSSM(ctx context.Context, logger *zerolog.Logger, env, repo, region string, registries []string) error {
	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	ssmClient := ssm.NewFromConfig(cfg)

	// Build SSM parameter path
	ssmPath := fmt.Sprintf("/%s/aws-deployer/ecr-registries/%s", env, repo)

	// Join registries as comma-separated list
	registryList := strings.Join(registries, ",")

	logger.Info().
		Str("ssm_path", ssmPath).
		Str("registries", registryList).
		Msg("storing registry allowlist in SSM")

	// Store in SSM
	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(ssmPath),
		Value:     aws.String(registryList),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true),
		Description: aws.String(fmt.Sprintf("Allowed ECR registries for %s in %s environment (for signature verification)", repo, env)),
	})
	if err != nil {
		return fmt.Errorf("failed to store registry allowlist in SSM: %w", err)
	}

	logger.Info().Msg("registry allowlist stored successfully")
	return nil
}
