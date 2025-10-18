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

const (
	maxRetries       = 10
	retryWaitSeconds = 30
)

type Handler struct {
	lockDAO *lockdao.DAO
}

type Input struct {
	Env          string `json:"env"`
	Repo         string `json:"repo"`
	SK           string `json:"sk"`            // Build KSUID
	ExecutionArn string `json:"execution_arn"` // Step Function execution ARN
	RetryCount   int    `json:"retry_count"`   // Number of retries so far
}

type Output struct {
	LockAcquired bool   `json:"lock_acquired"`
	RetryCount   int    `json:"retry_count"`
	ShouldRetry  bool   `json:"should_retry"`
	Message      string `json:"message"`
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

func (h *Handler) HandleAcquireLock(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Str("build_id", input.SK).
		Int("retry_count", input.RetryCount).
		Msg("Attempting to acquire deployment lock")

	// Try to acquire the lock
	_, acquired, err := h.lockDAO.Acquire(ctx, lockdao.AcquireInput{
		Env:          input.Env,
		Repo:         input.Repo,
		BuildID:      input.SK,
		ExecutionArn: input.ExecutionArn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to try acquire lock: %w", err)
	}

	if acquired {
		logger.Info().
			Str("env", input.Env).
			Str("repo", input.Repo).
			Str("build_id", input.SK).
			Msg("Lock acquired successfully")

		return &Output{
			LockAcquired: true,
			RetryCount:   input.RetryCount,
			ShouldRetry:  false,
			Message:      "Lock acquired",
		}, nil
	}

	// Lock is held by another build
	id := lockdao.NewID(input.Env, input.Repo)
	currentLock, err := h.lockDAO.Find(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get current lock: %w", err)
	}

	lockHolder := "unknown"
	if currentLock != nil {
		lockHolder = currentLock.BuildID
	}

	// Check if we should retry
	retryCount := input.RetryCount + 1
	shouldRetry := retryCount < maxRetries

	logger.Warn().
		Str("env", input.Env).
		Str("repo", input.Repo).
		Str("build_id", input.SK).
		Str("lock_holder", lockHolder).
		Int("retry_count", retryCount).
		Bool("should_retry", shouldRetry).
		Msg("Lock held by another build")

	if !shouldRetry {
		return nil, fmt.Errorf("failed to acquire lock after %d retries (held by build %s)", maxRetries, lockHolder)
	}

	return &Output{
		LockAcquired: false,
		RetryCount:   retryCount,
		ShouldRetry:  true,
		Message:      fmt.Sprintf("Lock held by build %s, retry %d/%d", lockHolder, retryCount, maxRetries),
	}, nil
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "acquire-lock").Logger()
	tableName := lockdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleAcquireLock(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "acquire-lock").Logger()

	tableName := lockdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// CLI mode for testing
	input := &Input{
		Env:          c.String("env"),
		Repo:         c.String("repo"),
		SK:           c.String("build-id"),
		ExecutionArn: c.String("execution-arn"),
		RetryCount:   c.Int("retry-count"),
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleAcquireLock(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "acquire-lock",
		Usage:          "Acquire deployment lock for a build",
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
						Name:     "execution-arn",
						Usage:    "Step Function execution ARN",
						EnvVars:  []string{"EXECUTION_ARN"},
						Required: true,
					},
					&cli.IntFlag{
						Name:    "retry-count",
						Usage:   "Retry count",
						EnvVars: []string{"RETRY_COUNT"},
						Value:   0,
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
