package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

type SecretVersion struct {
	Secret    string `json:"secret"`
	Timestamp string `json:"timestamp"`
}

type RotationEvent struct {
	Step               string `json:"Step"`
	Token              string `json:"Token"`
	SecretId           string `json:"SecretId"`
	ClientRequestToken string `json:"ClientRequestToken"`
}

type Handler struct {
	client *secretsmanager.Client
}

func NewHandler(ctx context.Context) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Handler{
		client: secretsmanager.NewFromConfig(cfg),
	}, nil
}

func generateSecureSecret() (string, error) {
	// Generate 256 bits (32 bytes) of secure random data
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func (h *Handler) HandleRotation(ctx context.Context, event RotationEvent) error {
	switch event.Step {
	case "createSecret":
		return h.createSecret(ctx, event)
	case "setSecret":
		return h.setSecret(ctx, event)
	case "testSecret":
		return h.testSecret(ctx, event)
	case "finishSecret":
		return h.finishSecret(ctx, event)
	default:
		return fmt.Errorf("unknown rotation step: %s", event.Step)
	}
}

func (h *Handler) createSecret(ctx context.Context, event RotationEvent) error {
	logger := zerolog.Ctx(ctx)

	// Generate new secure 256-bit secret (base64 encoded)
	newSecret, err := generateSecureSecret()
	if err != nil {
		return err
	}

	// Get current secret value to retrieve existing versions
	versions := []SecretVersion{}
	getCurrentOutput, err := h.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &event.SecretId,
	})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get current secret - starting fresh")
	} else if getCurrentOutput.SecretString == nil || *getCurrentOutput.SecretString == "" {
		logger.Warn().Msg("Secret is empty - starting fresh")
	} else {
		if err := json.Unmarshal([]byte(*getCurrentOutput.SecretString), &versions); err != nil {
			logger.Warn().Err(err).Msg("Current secret is corrupt (invalid JSON) - overwriting with fresh secret")
			versions = []SecretVersion{}
		} else {
			// Validate existing secrets are valid base64 and correct length
			validVersions := []SecretVersion{}
			for i, v := range versions {
				decoded, err := base64.StdEncoding.DecodeString(v.Secret)
				if err != nil {
					logger.Warn().Err(err).Int("index", i).Msg("Secret is not valid base64 - discarding")
					continue
				}
				// Validate it's 32 bytes (256 bits)
				if len(decoded) != 32 {
					logger.Warn().Int("index", i).Int("length", len(decoded)).Msg("Secret has invalid length (expected 32) - discarding")
					continue
				}
				validVersions = append(validVersions, v)
			}
			versions = validVersions
		}
	}

	// Add new version at the beginning
	newVersion := SecretVersion{
		Secret:    newSecret,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	versions = append([]SecretVersion{newVersion}, versions...)

	// Keep only the most recent 3 versions
	if len(versions) > 3 {
		versions = versions[:3]
	}

	secretJSON, err := json.Marshal(versions)
	if err != nil {
		return fmt.Errorf("failed to marshal secret: %w", err)
	}

	logger.Info().Int("version_count", len(versions)).Msg("Creating secret with valid versions")

	// Store the new secret version
	_, err = h.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:           &event.SecretId,
		SecretString:       stringPtr(string(secretJSON)),
		ClientRequestToken: &event.ClientRequestToken,
		VersionStages:      []string{"AWSPENDING"},
	})
	if err != nil {
		return fmt.Errorf("failed to put secret value: %w", err)
	}

	return nil
}

func (h *Handler) setSecret(ctx context.Context, event RotationEvent) error {
	// Nothing to set for this use case - we're just storing the secret
	return nil
}

func (h *Handler) testSecret(ctx context.Context, event RotationEvent) error {
	// Verify the pending secret can be retrieved and is valid JSON
	output, err := h.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     &event.SecretId,
		VersionStage: stringPtr("AWSPENDING"),
	})
	if err != nil {
		return fmt.Errorf("failed to get pending secret: %w", err)
	}

	var versions []SecretVersion
	if err := json.Unmarshal([]byte(*output.SecretString), &versions); err != nil {
		return fmt.Errorf("pending secret is not valid JSON: %w", err)
	}

	if len(versions) == 0 {
		return fmt.Errorf("pending secret has no versions")
	}

	// Verify the newest secret is valid base64
	if _, err := base64.StdEncoding.DecodeString(versions[0].Secret); err != nil {
		return fmt.Errorf("pending secret is not valid base64: %w", err)
	}

	return nil
}

func (h *Handler) finishSecret(ctx context.Context, event RotationEvent) error {
	// Move AWSPENDING to AWSCURRENT
	_, err := h.client.UpdateSecretVersionStage(ctx, &secretsmanager.UpdateSecretVersionStageInput{
		SecretId:            &event.SecretId,
		VersionStage:        stringPtr("AWSCURRENT"),
		MoveToVersionId:     &event.ClientRequestToken,
		RemoveFromVersionId: stringPtr("AWSCURRENT"),
	})
	if err != nil {
		return fmt.Errorf("failed to update version stage: %w", err)
	}

	return nil
}

func stringPtr(s string) *string {
	return &s
}

func handleRotateCommand(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "rotator").Logger()

	// Check if running in Lambda environment
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		ctx := context.Background()
		handler, err := NewHandler(ctx)
		if err != nil {
			return fmt.Errorf("failed to create handler: %w", err)
		}

		// Wrap handler to inject logger into context
		wrappedHandler := func(ctx context.Context, event RotationEvent) error {
			ctx = logger.WithContext(ctx)
			return handler.HandleRotation(ctx, event)
		}
		lambda.Start(wrappedHandler)
		return nil
	}

	// CLI mode for local testing
	ctx := context.Background()
	handler, err := NewHandler(ctx)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	secretID := c.String("secret-id")
	clientRequestToken := fmt.Sprintf("manual-%d", time.Now().Unix())

	// Run each rotation step
	steps := []string{"createSecret", "setSecret", "testSecret", "finishSecret"}
	for _, step := range steps {
		event := RotationEvent{
			Step:               step,
			SecretId:           secretID,
			ClientRequestToken: clientRequestToken,
		}

		if err := handler.HandleRotation(ctx, event); err != nil {
			return fmt.Errorf("%s step failed: %w", step, err)
		}
	}

	fmt.Println("Rotation completed successfully")
	return nil
}

func handleCancelRotationCommand(c *cli.Context) error {
	ctx := context.Background()
	handler, err := NewHandler(ctx)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	secretID := c.String("secret-id")
	versionID := c.String("version-id")

	fmt.Printf("Cancelling pending rotation for secret: %s\n", secretID)

	// Remove AWSPENDING stage from the version
	_, err = handler.client.UpdateSecretVersionStage(ctx, &secretsmanager.UpdateSecretVersionStageInput{
		SecretId:            &secretID,
		VersionStage:        stringPtr("AWSPENDING"),
		RemoveFromVersionId: &versionID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove AWSPENDING stage: %w", err)
	}

	fmt.Println("Successfully cancelled pending rotation")
	return nil
}

func main() {
	app := &cli.App{
		Name:           "rotator",
		Usage:          "Secrets Manager rotation function for session tokens",
		DefaultCommand: "rotate",
		Commands: []*cli.Command{
			{
				Name:  "rotate",
				Usage: "Manually trigger a rotation",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "secret-id",
						Usage:    "Secret ID to rotate",
						Required: true,
						EnvVars:  []string{"SECRET_ID"},
					},
				},
				Action: handleRotateCommand,
			},
			{
				Name:  "cancel-rotation",
				Usage: "Cancel a pending rotation",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "secret-id",
						Usage:    "Secret ID with pending rotation",
						Required: true,
						EnvVars:  []string{"SECRET_ID"},
					},
					&cli.StringFlag{
						Name:     "version-id",
						Usage:    "Version ID of the pending rotation to cancel",
						Required: true,
					},
				},
				Action: handleCancelRotationCommand,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
