package commands

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/services"
)

// ECRCreationResult contains information about created ECR repositories
type ECRCreationResult struct {
	Repositories   []*services.RepositoryInfo
	OrganizationID string
}

// createECRRepositories creates ECR repositories with org-wide permissions
func createECRRepositories(ctx context.Context, logger *zerolog.Logger, region string, registryNames []string) (*ECRCreationResult, error) {
	if len(registryNames) == 0 {
		return &ECRCreationResult{}, nil
	}

	logger.Info().Msgf("Creating %d ECR registr(ies)...", len(registryNames))

	// Create ECR service
	ecrService, err := services.NewECRService(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECR service: %w", err)
	}

	// Check if account is in an organization
	logger.Info().Msg("Checking if AWS account is in an organization...")
	orgID, err := ecrService.GetOrganizationID(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to check organization status (will skip org-wide permissions)")
		orgID = ""
	}

	if orgID != "" {
		logger.Info().Msgf("✓ Account is in organization: %s", orgID)
		logger.Info().Msg("  Will configure org-wide read permissions")
	} else {
		logger.Info().Msg("✓ Account is not in an organization")
		logger.Info().Msg("  Skipping org-wide permissions")
	}

	// Create repositories
	var repositories []*services.RepositoryInfo
	for _, registryName := range registryNames {
		logger.Info().Msgf("Creating repository: %s", registryName)
		repoInfo, err := ecrService.CreateRepository(ctx, registryName)
		if err != nil {
			return nil, fmt.Errorf("failed to create repository %q: %w", registryName, err)
		}

		logger.Info().Msgf("  ✓ Created: %s", repoInfo.Name)
		logger.Info().Msgf("    ARN: %s", repoInfo.ARN)
		logger.Info().Msgf("    URI: %s", repoInfo.URI)

		// Set organization-wide policy if in an org
		if orgID != "" {
			logger.Info().Msgf("  Setting org-wide read permissions...")
			if err := ecrService.SetRepositoryPolicy(ctx, registryName, orgID); err != nil {
				logger.Warn().Err(err).Msgf("    Failed to set org-wide policy (repository still created)")
			} else {
				logger.Info().Msgf("  ✓ Org-wide read permissions configured")
			}
		}

		repositories = append(repositories, repoInfo)
	}

	return &ECRCreationResult{
		Repositories:   repositories,
		OrganizationID: orgID,
	}, nil
}

// addECRPermissionsToRole adds ECR push permissions to an IAM role
func addECRPermissionsToRole(ctx context.Context, logger *zerolog.Logger, roleName string, repositories []*services.RepositoryInfo) error {
	if roleName == "" || len(repositories) == 0 {
		return nil
	}

	logger.Info().Msgf("Adding ECR push permissions to IAM role: %s", roleName)

	// Create IAM service
	iamService, err := services.NewIAMService()
	if err != nil {
		return fmt.Errorf("failed to create IAM service: %w", err)
	}

	// Collect repository ARNs
	var repositoryARNs []string
	for _, repo := range repositories {
		repositoryARNs = append(repositoryARNs, repo.ARN)
	}

	// Add ECR push permissions
	if err := iamService.AddECRPushPermissions(ctx, roleName, repositoryARNs); err != nil {
		return fmt.Errorf("failed to add ECR permissions to role: %w", err)
	}

	logger.Info().Msg("  ✓ ECR push permissions added to role")
	return nil
}
