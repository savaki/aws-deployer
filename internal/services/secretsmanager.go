package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type SecretsManagerService struct {
	client *secretsmanager.Client
}

// OAuthConfig represents OAuth/OIDC provider configuration.
// Supports multiple providers: Auth0, Google CIAM.
type OAuthConfig struct {
	Provider     string `json:"provider"`      // "auth0" or "google-ciam"
	ClientID     string `json:"client_id"`     // OAuth client ID
	ClientSecret string `json:"client_secret"` // OAuth client secret
	Domain       string `json:"domain"`        // For Auth0: tenant domain (e.g., "tenant.us.auth0.com")
	ProjectID    string `json:"project_id"`    // For Google CIAM: GCP project ID
}

func NewSecretsManagerService() (*SecretsManagerService, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &SecretsManagerService{
		client: secretsmanager.NewFromConfig(cfg),
	}, nil
}

// GetOAuthConfig retrieves OAuth provider configuration from AWS Secrets Manager.
// Supports multiple providers: "auth0", "google-ciam".
// For backward compatibility, defaults to "auth0" if provider field is missing.
func (s *SecretsManagerService) GetOAuthConfig(ctx context.Context) (*OAuthConfig, error) {
	env := os.Getenv("ENV")
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	if env == "" {
		env = "dev"
	}

	secretName := fmt.Sprintf("aws-deployer/%s/secrets", env)

	result, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("secret %s has no string value", secretName)
	}

	var oauthConfig OAuthConfig
	if err := json.Unmarshal([]byte(*result.SecretString), &oauthConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OAuth config: %w", err)
	}

	// Backward compatibility: default to auth0 if provider not specified
	if oauthConfig.Provider == "" {
		oauthConfig.Provider = "auth0"
	}

	return &oauthConfig, nil
}

type GitHubPATSecret struct {
	GitHubPAT string `json:"github_pat"`
}

// GetSecret retrieves a secret value by path from AWS Secrets Manager
func (s *SecretsManagerService) GetSecret(ctx context.Context, secretPath string) (string, error) {
	result, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretPath),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretPath, err)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret %s has no string value", secretPath)
	}

	return *result.SecretString, nil
}

// GetGitHubPAT retrieves a GitHub PAT token from AWS Secrets Manager
func (s *SecretsManagerService) GetGitHubPAT(ctx context.Context, secretPath string) (string, error) {
	result, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretPath),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretPath, err)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret %s has no string value", secretPath)
	}

	var patSecret GitHubPATSecret
	if err := json.Unmarshal([]byte(*result.SecretString), &patSecret); err != nil {
		return "", fmt.Errorf("failed to unmarshal GitHub PAT secret: %w", err)
	}

	if patSecret.GitHubPAT == "" {
		return "", fmt.Errorf("github_pat field is empty in secret %s", secretPath)
	}

	return patSecret.GitHubPAT, nil
}
