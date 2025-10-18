package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
)

// SetupGitHubCommand returns the github command for creating GitHub OIDC roles
func SetupGitHubCommand(logger *zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:    "github",
		Usage:   "Create an IAM role for GitHub Actions OIDC authentication",
		Description: `Configure GitHub repository with AWS OIDC authentication.

This command creates an IAM role for GitHub Actions OIDC authentication and configures
the necessary GitHub secrets for your repository to deploy to AWS without long-lived credentials.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "role-name",
				Aliases:  []string{"n"},
				Usage:    "IAM role name to create (defaults to 'github-{repo}' if not provided)",
				Required: false,
				EnvVars:  []string{"GITHUB_ROLE_NAME"},
			},
			&cli.StringFlag{
				Name:     "repo",
				Aliases:  []string{"r"},
				Usage:    "Repository in format 'owner/repo'",
				Required: true,
				EnvVars:  []string{"GITHUB_REPO"},
			},
			&cli.StringFlag{
				Name:     "bucket",
				Aliases:  []string{"b"},
				Usage:    "S3 artifact bucket name",
				Required: true,
				EnvVars:  []string{"S3_ARTIFACT_BUCKET"},
			},
			&cli.StringFlag{
				Name:     "github-token-secret",
				Aliases:  []string{"t"},
				Usage:    "Path to GitHub PAT token in AWS Secrets Manager",
				Required: true,
				EnvVars:  []string{"GITHUB_TOKEN_SECRET"},
			},
		},
		Action: setupGitHubAction,
	}
}

// setupGitHubAction creates an IAM role for GitHub Actions OIDC authentication
func setupGitHubAction(c *cli.Context) error {
	logger := zerolog.Ctx(c.Context)

	roleName := c.String("role-name")
	repoFullPath := c.String("repo")
	bucket := c.String("bucket")
	githubTokenSecret := c.String("github-token-secret")

	if repoFullPath == "" {
		return fmt.Errorf("repo is required")
	}
	if bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if githubTokenSecret == "" {
		return fmt.Errorf("github-token-secret is required")
	}

	// Parse owner/repo format
	parts := strings.SplitN(repoFullPath, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("repo must be in format 'owner/repo', got: %s", repoFullPath)
	}
	owner := parts[0]
	repo := parts[1]

	// Default role name if not provided
	if roleName == "" {
		roleName = fmt.Sprintf("github-%s", repo)
		logger.Info().
			Str("role_name", roleName).
			Msg("No role name provided, using default")
	}

	logger.Info().
		Str("role_name", roleName).
		Str("owner", owner).
		Str("repo", repo).
		Str("bucket", bucket).
		Msg("Creating GitHub OIDC role")

	// Create IAM service
	iamService, err := services.NewIAMService()
	if err != nil {
		return fmt.Errorf("failed to create IAM service: %w", err)
	}

	// Create or update the OIDC role and get ARN
	roleARN, err := iamService.CreateGitHubOIDCRole(context.Background(), roleName, owner, repo, bucket)
	if err != nil {
		return fmt.Errorf("failed to create/update GitHub OIDC role: %w", err)
	}

	logger.Info().
		Str("role_name", roleName).
		Str("role_arn", roleARN).
		Msg("Successfully created/updated GitHub OIDC role")

	// Get GitHub token from Secrets Manager
	secretsService, err := services.NewSecretsManagerService()
	if err != nil {
		return fmt.Errorf("failed to create Secrets Manager service: %w", err)
	}

	githubToken, err := secretsService.GetGitHubPAT(context.Background(), githubTokenSecret)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token from Secrets Manager: %w", err)
	}

	// Create GitHub service
	githubService := services.NewGitHubService(githubToken)

	// Create or update AWS_ROLE_ARN secret
	logger.Info().
		Str("owner", owner).
		Str("repo", repo).
		Msg("Creating/updating AWS_ROLE_ARN secret in GitHub")
	if err := githubService.CreateOrUpdateSecret(context.Background(), owner, repo, "AWS_ROLE_ARN", roleARN); err != nil {
		return fmt.Errorf("failed to create/update AWS_ROLE_ARN secret: %w", err)
	}

	// Create or update S3_ARTIFACT_BUCKET secret
	logger.Info().
		Str("owner", owner).
		Str("repo", repo).
		Str("bucket", bucket).
		Msg("Creating/updating S3_ARTIFACT_BUCKET secret in GitHub")
	if err := githubService.CreateOrUpdateSecret(context.Background(), owner, repo, "S3_ARTIFACT_BUCKET", bucket); err != nil {
		return fmt.Errorf("failed to create/update S3_ARTIFACT_BUCKET secret: %w", err)
	}

	fmt.Printf("✓ IAM role %s created/updated successfully\n", roleName)
	fmt.Printf("✓ Role ARN: %s\n", roleARN)
	fmt.Printf("✓ IAM policy grants S3 access to: %s/%s/*\n", bucket, repo)
	fmt.Printf("✓ Trust policy allows GitHub Actions from: %s/%s\n", owner, repo)
	fmt.Printf("✓ GitHub secrets created/updated in: %s/%s\n", owner, repo)
	fmt.Printf("  - AWS_ROLE_ARN\n")
	fmt.Printf("  - S3_ARTIFACT_BUCKET\n")
	fmt.Printf("\n")
	fmt.Printf("🔐 Using OIDC authentication (no long-lived credentials needed)\n")
	fmt.Printf("ℹ️  This tool is idempotent - safe to run multiple times\n")

	return nil
}
