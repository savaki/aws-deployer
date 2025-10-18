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
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	deploymentDAO *deploymentdao.DAO
}

type DeploymentTarget struct {
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
}

type Input struct {
	Env     string             `json:"env"`
	Repo    string             `json:"repo"`
	SK      string             `json:"sk"` // Build KSUID
	Targets []DeploymentTarget `json:"targets"`
}

type Output struct {
	InitializedCount int `json:"initialized_count"`
}

func NewHandler(tableName string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)
	deploymentDAO := deploymentdao.New(client, tableName)

	return &Handler{
		deploymentDAO: deploymentDAO,
	}, nil
}

func (h *Handler) HandleInitializeDeployments(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Str("build_id", input.SK).
		Int("target_count", len(input.Targets)).
		Msg("Initializing deployment records")

	// Create PENDING records for each target
	for _, target := range input.Targets {
		_, err := h.deploymentDAO.Create(ctx, deploymentdao.CreateInput{
			Env:     input.Env,
			Repo:    input.Repo,
			Account: target.AccountID,
			Region:  target.Region,
			BuildID: input.SK,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create deployment record for %s/%s: %w", target.AccountID, target.Region, err)
		}

		logger.Debug().
			Str("account", target.AccountID).
			Str("region", target.Region).
			Msg("Created deployment record")
	}

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Int("initialized_count", len(input.Targets)).
		Msg("Deployment records initialized successfully")

	return &Output{
		InitializedCount: len(input.Targets),
	}, nil
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "initialize-deployments").Logger()
	tableName := deploymentdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleInitializeDeployments(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "initialize-deployments").Logger()

	tableName := deploymentdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// CLI mode for testing
	var targets []DeploymentTarget
	if err := json.Unmarshal([]byte(c.String("targets")), &targets); err != nil {
		return fmt.Errorf("failed to parse targets: %w", err)
	}

	input := &Input{
		Env:     c.String("env"),
		Repo:    c.String("repo"),
		SK:      c.String("build-id"),
		Targets: targets,
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleInitializeDeployments(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "initialize-deployments",
		Usage:          "Initialize deployment state records",
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
					&cli.StringFlag{
						Name:     "targets",
						Usage:    "Targets JSON array",
						EnvVars:  []string{"TARGETS"},
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
