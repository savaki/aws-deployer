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
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	deploymentDAO *deploymentdao.DAO
	dbService     *services.DynamoDBService
}

type Input struct {
	Env  string `json:"env"`
	Repo string `json:"repo"`
	SK   string `json:"sk"` // Build KSUID
}

type DeploymentSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

type Output struct {
	BuildStatus       string            `json:"build_status"` // SUCCESS, PARTIAL_SUCCESS, FAILED
	FailedDeployments []string          `json:"failed_deployments,omitempty"`
	PartialSuccess    bool              `json:"partial_success"`
	DeploymentSummary DeploymentSummary `json:"deployment_summary"`
	ErrorMsg          string            `json:"error_msg,omitempty"`
}

func NewHandler(env string, deploymentsTableName string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	dbClient := dynamodb.NewFromConfig(cfg)
	deploymentDAO := deploymentdao.New(dbClient, deploymentsTableName)

	dbService, err := services.NewDynamoDBService(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB service: %w", err)
	}

	return &Handler{
		deploymentDAO: deploymentDAO,
		dbService:     dbService,
	}, nil
}

func (h *Handler) HandleAggregateResults(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	// Validate input
	if input.Env == "" {
		return nil, fmt.Errorf("env is required but was empty")
	}
	if input.Repo == "" {
		return nil, fmt.Errorf("repo is required but was empty")
	}
	if input.SK == "" {
		return nil, fmt.Errorf("sk (build ID) is required but was empty")
	}

	// Construct full build ID for logging
	pk := builddao.NewPK(input.Repo, input.Env)
	buildID := builddao.NewID(pk, input.SK)

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Str("build_id", string(buildID)).
		Str("ksuid", input.SK).
		Msg("Aggregating deployment results")

	// Query all deployments for this build
	deployments, err := h.deploymentDAO.QueryByBuild(ctx, input.Env, input.Repo, input.SK)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployments: %w", err)
	}

	// Count statuses
	total := len(deployments)
	succeeded := 0
	failed := 0
	var failedDeployments []string

	for _, deployment := range deployments {
		switch deployment.Status {
		case deploymentdao.StatusSuccess:
			succeeded++
		case deploymentdao.StatusFailed:
			failed++
			failedKey := deployment.SK.String() // Account/Region
			failedDeployments = append(failedDeployments, failedKey)
		}
	}

	logger.Info().
		Int("total", total).
		Int("succeeded", succeeded).
		Int("failed", failed).
		Msg("Deployment results aggregated")

	// Determine overall build status
	buildStatus := builddao.BuildStatusSuccess
	partialSuccess := false
	var errorMsg string

	if failed == total {
		buildStatus = builddao.BuildStatusFailed
		errorMsg = fmt.Sprintf("All %d deployments failed", total)
	} else if failed > 0 {
		buildStatus = builddao.BuildStatusFailed
		partialSuccess = true
		errorMsg = fmt.Sprintf("%d of %d deployments failed", failed, total)
	}

	// Update build record with aggregated results
	// Build update with new multi-account fields
	// Note: We'll need to extend builddao.UpdateInput to support these new fields
	// For now, just update status
	_, err = h.dbService.UpdateBuildStatus(ctx, builddao.UpdateInput{
		PK:     pk,
		SK:     input.SK,
		Status: &buildStatus,
		ErrorMsg: func() *string {
			if errorMsg != "" {
				return &errorMsg
			}
			return nil
		}(),
	})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update build status")
		// Continue - we'll still return the aggregated results
	}

	return &Output{
		BuildStatus:       string(buildStatus),
		FailedDeployments: failedDeployments,
		PartialSuccess:    partialSuccess,
		DeploymentSummary: DeploymentSummary{
			Total:     total,
			Succeeded: succeeded,
			Failed:    failed,
		},
		ErrorMsg: errorMsg,
	}, nil
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "aggregate-results").Logger()
	env := c.String("env")
	tableName := deploymentdao.TableName(env)
	handler, err := NewHandler(env, tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleAggregateResults(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "aggregate-results").Logger()

	env := c.String("env")
	tableName := deploymentdao.TableName(env)
	handler, err := NewHandler(env, tableName)
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
	result, err := handler.HandleAggregateResults(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "aggregate-results",
		Usage:          "Aggregate deployment results and update build status",
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
