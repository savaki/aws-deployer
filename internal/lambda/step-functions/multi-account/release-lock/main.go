package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/lockdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	lockDAO *lockdao.DAO
}

type Input struct {
	Env  string `json:"env"`
	Repo string `json:"repo"`
	SK   string `json:"sk"` // Build KSUID
}

type Output struct {
	Released bool   `json:"released"`
	Message  string `json:"message"`
}

func NewHandler(tableName string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)
	lockDAO := lockdao.New(client, tableName)

	return &Handler{
		lockDAO: lockDAO,
	}, nil
}

func (h *Handler) HandleReleaseLock(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Str("build_id", input.SK).
		Msg("Releasing deployment lock")

	id := lockdao.NewID(input.Env, input.Repo)
	err := h.lockDAO.Release(ctx, lockdao.ReleaseInput{
		ID:      id,
		BuildID: input.SK,
	})
	if err != nil {
		logger.Error().
			Err(err).
			Str("env", input.Env).
			Str("repo", input.Repo).
			Str("build_id", input.SK).
			Msg("Failed to release lock")
		return nil, fmt.Errorf("failed to release lock: %w", err)
	}

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Str("build_id", input.SK).
		Msg("Lock released successfully")

	return &Output{
		Released: true,
		Message:  "Lock released",
	}, nil
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "release-lock").Logger()
	tableName := lockdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleReleaseLock(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "release-lock").Logger()

	tableName := lockdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// CLI mode for testing
	input := &Input{
		Env:  c.String("env"),
		Repo: c.String("repo"),
		SK:   c.String("build-id"),
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleReleaseLock(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "release-lock",
		Usage:          "Release deployment lock for a build",
		DefaultCommand: "lambda",
		Commands: []*cli.Command{
			{
				Name:   "lambda",
				Usage:  "Start Lambda handler",
				Action: lambdaAction,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "env",
						Usage:   "Environment",
						EnvVars: []string{"ENV"},
						Value:   "dev",
					},
				},
			},
			{
				Name:  "run",
				Usage: "Run locally for testing",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "env",
						Usage:   "Environment",
						EnvVars: []string{"ENV"},
						Value:   "dev",
					},
					&cli.StringFlag{
						Name:     "repo",
						Usage:    "Repository name",
						EnvVars:  []string{"REPO"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "build-id",
						Usage:    "Build KSUID",
						EnvVars:  []string{"BUILD_ID"},
						Required: true,
					},
				},
				Action: runAction,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
