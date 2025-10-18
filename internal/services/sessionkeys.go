package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/rs/zerolog"
)

// SecretVersion represents a single rotated secret version
type SecretVersion struct {
	Secret    string `json:"secret"`
	Timestamp string `json:"timestamp"`
}

// SessionKeyFetcher is a function that fetches session encryption keys
type SessionKeyFetcher func(ctx context.Context) ([][]byte, error)

// SessionKeyService provides session encryption keys from Secrets Manager
type SessionKeyService struct {
	client     *secretsmanager.Client
	secretName string
	onceFunc   func() ([][]byte, error)
}

// NewSessionKeyService creates a new session key service
func NewSessionKeyService(client *secretsmanager.Client, secretName string) *SessionKeyService {
	s := &SessionKeyService{
		client:     client,
		secretName: secretName,
	}

	// Wrap fetchSessionKeys with sync.OnceValues for single fetch per Lambda lifecycle
	// We capture the service in the closure
	s.onceFunc = sync.OnceValues(func() ([][]byte, error) {
		return s.fetchSessionKeys(context.Background())
	})

	return s
}

// GetSessionKeys returns the current session encryption keys from Secrets Manager.
// Uses sync.OnceValues to ensure keys are fetched only once per Lambda lifecycle.
// Lambda restarts every few hours naturally refresh the keys.
func (s *SessionKeyService) GetSessionKeys(ctx context.Context) ([][]byte, error) {
	return s.onceFunc()
}

func (s *SessionKeyService) fetchSessionKeys(ctx context.Context) ([][]byte, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().Str("secret_name", s.secretName).Msg("Fetching session keys from Secrets Manager")

	// Fetch the secret
	result, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(s.secretName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", s.secretName, err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("secret %s has no string value", s.secretName)
	}

	// Parse the secret JSON (array of versions)
	var versions []SecretVersion
	if err := json.Unmarshal([]byte(*result.SecretString), &versions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret versions: %w", err)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no secret versions found in %s", s.secretName)
	}

	// Decode all versions (most recent first, up to 3)
	keys := make([][]byte, 0, len(versions))
	for i, version := range versions {
		decoded, err := base64.StdEncoding.DecodeString(version.Secret)
		if err != nil {
			logger.Warn().
				Int("index", i).
				Str("timestamp", version.Timestamp).
				Err(err).
				Msg("Failed to decode secret version, skipping")
			continue
		}

		// Validate key length (should be 32 bytes for AES-256)
		if len(decoded) != 32 {
			logger.Warn().
				Int("index", i).
				Int("length", len(decoded)).
				Str("timestamp", version.Timestamp).
				Msg("Secret version has invalid length, skipping")
			continue
		}

		keys = append(keys, decoded)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no valid session keys found in secret %s", s.secretName)
	}

	logger.Info().Int("key_count", len(keys)).Msg("Successfully loaded session keys")

	return keys, nil
}
