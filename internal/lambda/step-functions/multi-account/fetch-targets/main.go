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
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	targetDAO *targetdao.DAO
}

type Input struct {
	Env  string `json:"env"`
	Repo string `json:"repo"`
	SK   string `json:"sk"` // Build KSUID
}

type DeploymentTarget struct {
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
}

type Output struct {
	Targets []DeploymentTarget `json:"targets"`
	Count   int                `json:"count"`
}

func NewHandler(tableName string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)
	targetDAO := targetdao.New(client, tableName)

	return &Handler{
		targetDAO: targetDAO,
	}, nil
}

func (h *Handler) HandleFetchTargets(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Msg("Fetching deployment targets")

	// Get targets with default fallback
	record, err := h.targetDAO.GetWithDefault(ctx, input.Repo, input.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to get targets: %w", err)
	}

	if record == nil {
		return nil, fmt.Errorf("no deployment targets configured for repo=%s, env=%s (and no default targets found)", input.Repo, input.Env)
	}

	// Expand targets into all account/region combinations
	expanded := targetdao.ExpandTargets(record.Targets)

	targets := make([]DeploymentTarget, len(expanded))
	for i, t := range expanded {
		targets[i] = DeploymentTarget{
			AccountID: t.AccountID,
			Region:    t.Region,
		}
	}

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Int("target_count", len(targets)).
		Msg("Deployment targets fetched successfully")

	return &Output{
		Targets: targets,
		Count:   len(targets),
	}, nil
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "fetch-targets").Logger()
	tableName := targetdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleFetchTargets(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "fetch-targets").Logger()

	tableName := targetdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// CLI mode for testing
	input := &Input{
		Env:  c.String("env"),
		Repo: c.String("repo"),
		SK:   c.String("sk"),
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleFetchTargets(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "fetch-targets",
		Usage:          "Fetch deployment targets for a repo/env",
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
						Name:    "sk",
						Usage:   "Build KSUID",
						EnvVars: []string{"SK"},
						Value:   "test",
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
