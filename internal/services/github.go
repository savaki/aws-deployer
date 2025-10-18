package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/crypto/nacl/box"
)

type GitHubService struct {
	token      string
	httpClient *http.Client
}

type GitHubPublicKey struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"`
}

type GitHubSecretRequest struct {
	EncryptedValue string `json:"encrypted_value"`
	KeyID          string `json:"key_id"`
}

func NewGitHubService(token string) *GitHubService {
	return &GitHubService{
		token:      token,
		httpClient: &http.Client{},
	}
}

// GetPublicKey fetches the repository's public key for encrypting secrets
func (g *GitHubService) GetPublicKey(ctx context.Context, owner, repo string) (*GitHubPublicKey, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/secrets/public-key", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch public key: status %d, body: %s", resp.StatusCode, string(body))
	}

	var publicKey GitHubPublicKey
	if err := json.NewDecoder(resp.Body).Decode(&publicKey); err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	return &publicKey, nil
}

// encryptSecret encrypts a secret value using libsodium sealed box
func (g *GitHubService) encryptSecret(publicKeyBase64, secretValue string) (string, error) {
	// Decode the public key from base64
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode public key: %w", err)
	}

	if len(publicKeyBytes) != 32 {
		return "", fmt.Errorf("invalid public key length: expected 32, got %d", len(publicKeyBytes))
	}

	// Convert to [32]byte for NaCl box
	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	// Encrypt using sealed box (anonymous encryption)
	secretBytes := []byte(secretValue)
	encrypted, err := box.SealAnonymous(nil, secretBytes, &publicKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Encode to base64
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// CreateOrUpdateSecret creates or updates a repository secret
func (g *GitHubService) CreateOrUpdateSecret(ctx context.Context, owner, repo, secretName, secretValue string) error {
	// Get the public key
	publicKey, err := g.GetPublicKey(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Encrypt the secret
	encryptedValue, err := g.encryptSecret(publicKey.Key, secretValue)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Create the request body
	requestBody := GitHubSecretRequest{
		EncryptedValue: encryptedValue,
		KeyID:          publicKey.KeyID,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create or update the secret
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/secrets/%s", owner, repo, secretName)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create/update secret: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create/update secret: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
