package services

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Config holds all application configuration values from Parameter Store
type Config struct {
	StateMachineArn              string
	MultiAccountStateMachineArn  string
	DeploymentMode               string
	S3Bucket                     string
	AllowedEmail                 string
	SessionTokenSecretName       string
	CustomDomain                 string
	APIGatewayID                 string
}

// ParameterStore defines the interface for accessing configuration parameters
type ParameterStore interface {
	// GetParameter retrieves a single parameter by name
	GetParameter(ctx context.Context, name string) (string, error)

	// GetConfig loads all application configuration from Parameter Store
	GetConfig(ctx context.Context) (*Config, error)
}

// SSMParameterStore implements ParameterStore using AWS Systems Manager Parameter Store
type SSMParameterStore struct {
	client *ssm.Client
	env    string
	mu     sync.RWMutex
	cache  map[string]string
}

// NewSSMParameterStore creates a new SSM-backed parameter store
func NewSSMParameterStore(client *ssm.Client, env string) *SSMParameterStore {
	return &SSMParameterStore{
		client: client,
		env:    env,
		cache:  make(map[string]string),
	}
}

// GetParameter retrieves a single parameter from SSM Parameter Store
func (s *SSMParameterStore) GetParameter(ctx context.Context, name string) (string, error) {
	// Check cache first
	s.mu.RLock()
	if value, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return value, nil
	}
	s.mu.RUnlock()

	// Fetch from SSM
	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: boolPtr(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get parameter %s: %w", name, err)
	}

	if result.Parameter == nil || result.Parameter.Value == nil {
		return "", fmt.Errorf("parameter %s not found", name)
	}

	value := *result.Parameter.Value

	// Cache the value
	s.mu.Lock()
	s.cache[name] = value
	s.mu.Unlock()

	return value, nil
}

// GetConfig loads all application configuration from Parameter Store
func (s *SSMParameterStore) GetConfig(ctx context.Context) (*Config, error) {
	path := fmt.Sprintf("/%s/aws-deployer", s.env)

	// Use GetParametersByPath for efficient batch retrieval
	result, err := s.client.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
		Path:           &path,
		Recursive:      boolPtr(true),
		WithDecryption: boolPtr(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameters by path %s: %w", path, err)
	}

	// Build a map of parameter names to values
	params := make(map[string]string)
	for _, param := range result.Parameters {
		if param.Name != nil && param.Value != nil {
			params[*param.Name] = *param.Value
		}
	}

	// Cache all retrieved parameters
	s.mu.Lock()
	for k, v := range params {
		s.cache[k] = v
	}
	s.mu.Unlock()

	// Build config from parameters
	config := &Config{
		StateMachineArn:              params[fmt.Sprintf("/%s/aws-deployer/state-machine-arn", s.env)],
		MultiAccountStateMachineArn:  params[fmt.Sprintf("/%s/aws-deployer/multi-account-state-machine-arn", s.env)],
		DeploymentMode:               params[fmt.Sprintf("/%s/aws-deployer/deployment-mode", s.env)],
		S3Bucket:                     params[fmt.Sprintf("/%s/aws-deployer/s3-bucket", s.env)],
		AllowedEmail:                 params[fmt.Sprintf("/%s/aws-deployer/allowed-email", s.env)],
		SessionTokenSecretName:       params[fmt.Sprintf("/%s/aws-deployer/session-token-secret-name", s.env)],
		CustomDomain:                 params[fmt.Sprintf("/%s/aws-deployer/custom-domain", s.env)],
		APIGatewayID:                 params[fmt.Sprintf("/%s/aws-deployer/api-gateway-id", s.env)],
	}

	// Set defaults
	if config.DeploymentMode == "" {
		config.DeploymentMode = "single"
	}

	return config, nil
}

// EnvParameterStore implements ParameterStore using environment variables
// This is a NoOp implementation for local development without AWS connection
type EnvParameterStore struct {
	env string
}

// NewEnvParameterStore creates a new environment variable-backed parameter store
func NewEnvParameterStore(env string) *EnvParameterStore {
	return &EnvParameterStore{
		env: env,
	}
}

// GetParameter retrieves a parameter from environment variables
// This is a fallback implementation that reads from env vars
func (e *EnvParameterStore) GetParameter(ctx context.Context, name string) (string, error) {
	// For env var implementation, we don't use the full path
	// Just return the value if set
	return os.Getenv(name), nil
}

// GetConfig loads all application configuration from environment variables
func (e *EnvParameterStore) GetConfig(ctx context.Context) (*Config, error) {
	config := &Config{
		StateMachineArn:              os.Getenv("STATE_MACHINE_ARN"),
		MultiAccountStateMachineArn:  os.Getenv("MULTI_ACCOUNT_STATE_MACHINE_ARN"),
		DeploymentMode:               os.Getenv("DEPLOYMENT_MODE"),
		S3Bucket:                     os.Getenv("S3_BUCKET_NAME"),
		AllowedEmail:                 os.Getenv("ALLOWED_EMAIL"),
		SessionTokenSecretName:       os.Getenv("SESSION_TOKEN_SECRET_NAME"),
		CustomDomain:                 os.Getenv("CUSTOM_DOMAIN"),
		APIGatewayID:                 os.Getenv("API_GATEWAY_ID"),
	}

	// Set defaults
	if config.DeploymentMode == "" {
		config.DeploymentMode = "single"
	}

	// Set default session token secret name if not provided
	if config.SessionTokenSecretName == "" {
		config.SessionTokenSecretName = fmt.Sprintf("aws-deployer/%s/session-token", e.env)
	}

	return config, nil
}

func boolPtr(b bool) *bool {
	return &b
}
