package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	dbService *services.DynamoDBService
}

type UpdateStatusInput struct {
	Repo     string  `json:"repo"`
	Env      string  `json:"env"`
	SK       string  `json:"sk"` // KSUID - DynamoDB sort key
	Status   string  `json:"status"`
	ErrorMsg *string `json:"error_msg,omitempty"`
}

func NewHandler(env string) (*Handler, error) {
	dbService, err := services.NewDynamoDBService(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB service: %w", err)
	}

	return &Handler{
		dbService: dbService,
	}, nil
}

func (h *Handler) HandleUpdateBuildStatus(ctx context.Context, input *UpdateStatusInput) error {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("repo", input.Repo).
		Str("env", input.Env).
		Str("sk", input.SK).
		Str("status", input.Status).
		Msg("Updating build status")

	pk := builddao.NewPK(input.Repo, input.Env)
	status := builddao.BuildStatus(input.Status)

	_, err := h.dbService.UpdateBuildStatus(ctx, builddao.UpdateInput{
		PK:       pk,
		SK:       input.SK,
		Status:   &status,
		ErrorMsg: input.ErrorMsg,
	})
	if err != nil {
		return fmt.Errorf("failed to update build status: %w", err)
	}

	logger.Info().
		Str("repo", input.Repo).
		Str("env", input.Env).
		Str("sk", input.SK).
		Msg("Successfully updated build status")

	return nil
}

func main() {
	logger := di.ProvideLogger().With().Str("lambda", "update-build-status").Logger()

	// Get environment from ENV variable
	env := os.Getenv("ENV")
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	if env == "" {
		logger.Error().Msg("ENV or ENVIRONMENT variable is required")
		os.Exit(1)
	}

	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		// Lambda mode
		handler, err := NewHandler(env)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create handler")
			os.Exit(1)
		}

		// Wrap handler to inject logger into context
		wrappedHandler := func(ctx context.Context, input *UpdateStatusInput) error {
			ctx = logger.WithContext(ctx)
			return handler.HandleUpdateBuildStatus(ctx, input)
		}
		lambda.Start(wrappedHandler)
		return
	}

	// CLI mode
	handler, err := NewHandler(env)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create handler")
		os.Exit(1)
	}

	app := &cli.App{
		Name:  "update-build-status",
		Usage: "Update build status in DynamoDB",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "repo",
				Usage:    "Repository name",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "env",
				Usage:    "Environment name",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "sk",
				Usage:    "Sort key (KSUID)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "status",
				Usage:    "Build status (PENDING, IN_PROGRESS, SUCCESS, FAILED)",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "error-msg",
				Usage: "Error message (optional)",
			},
		},
		Action: func(c *cli.Context) error {
			input := &UpdateStatusInput{
				Repo:   c.String("repo"),
				Env:    c.String("env"),
				SK:     c.String("sk"),
				Status: c.String("status"),
			}

			if errorMsg := c.String("error-msg"); errorMsg != "" {
				input.ErrorMsg = &errorMsg
			}

			return handler.HandleUpdateBuildStatus(context.Background(), input)
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
}
