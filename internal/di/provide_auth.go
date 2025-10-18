package di

import (
	"context"
	"fmt"
	"os"
	"strings"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/auth"
	"github.com/savaki/aws-deployer/internal/authz"
	"github.com/savaki/aws-deployer/internal/services"
)

func ProvideSessionKeyService(ctx context.Context, config *services.Config) (*services.SessionKeyService, error) {
	// Load AWS config
	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	return services.NewSessionKeyService(client, config.SessionTokenSecretName), nil
}

func ProvideSessionKeys(ctx context.Context, keyService *services.SessionKeyService) ([][]byte, error) {
	logger := zerolog.Ctx(ctx)

	keys, err := keyService.GetSessionKeys(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch session keys from Secrets Manager")

		// In production (Lambda), we must fail fast rather than using ephemeral keys
		// Ephemeral keys break sessions across Lambda containers causing auth loops
		if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
			return nil, fmt.Errorf("session keys required in Lambda environment: %w", err)
		}

		// For local development, allow fallback to ephemeral key
		logger.Warn().Msg("Using ephemeral session key for local development only")
		return [][]byte{}, nil
	}
	return keys, nil
}

func ProvideAuthenticator(ctx context.Context, secretsService *services.SecretsManagerService, authorizer *authz.Authorizer, callbackURL CallbackURL, sessionKeys [][]byte, disableAuth DisableAuth) (*auth.Authenticator, error) {
	logger := zerolog.Ctx(ctx)

	// If auth is disabled, return NoOp authenticator
	if bool(disableAuth) {
		logger.Warn().Msg("⚠️  Authentication is DISABLED - using NoOp authenticator (development only)")
		return auth.NewNoOpAuthenticator(), nil
	}

	oauthConfig, err := secretsService.GetOAuthConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth config: %w", err)
	}

	// Create provider based on config
	var provider auth.Provider
	switch oauthConfig.Provider {
	case "auth0":
		provider = &auth.Auth0Provider{
			Domain: oauthConfig.Domain,
		}
	case "google-ciam":
		// Google Cloud Identity Platform / Firebase Auth
		provider = &auth.GoogleCIAMProvider{
			ProjectID: oauthConfig.ProjectID,
		}
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", oauthConfig.Provider)
	}

	// Detect local development: if callback URL uses http://localhost or http://127.0.0.1
	// In local dev, we need to disable Secure cookie flag since we're on HTTP
	callbackURLStr := string(callbackURL)
	isLocalDev := strings.HasPrefix(callbackURLStr, "http://localhost") ||
		strings.HasPrefix(callbackURLStr, "http://127.0.0.1")

	authenticator, err := auth.NewAuthenticator(ctx, auth.AuthenticatorInput{
		Provider:     provider,
		ClientID:     oauthConfig.ClientID,
		ClientSecret: oauthConfig.ClientSecret,
		CallbackURL:  callbackURLStr,
		Authorizer:   authorizer,
		SessionKeys:  sessionKeys,
		IsLocalDev:   isLocalDev,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticator: %w", err)
	}

	return authenticator, nil
}

func ProvideAuthorizer(ctx context.Context, logger zerolog.Logger, secretsService *services.SecretsManagerService, config *services.Config) *authz.Authorizer {
	if config.AllowedEmail == "" {
		logger.Info().Msg("Email authorization disabled - all authenticated users allowed")
		return nil
	}

	// Get OAuth config to determine provider type
	oauthConfig, err := secretsService.GetOAuthConfig(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get OAuth config for authorizer, disabling authorization")
		return nil
	}

	logger.Info().
		Str("allowed_email", config.AllowedEmail).
		Str("provider_type", oauthConfig.Provider).
		Msg("Email authorization enabled")

	return authz.NewGoogleEmailAuthorizer(true, config.AllowedEmail, oauthConfig.Provider)
}
