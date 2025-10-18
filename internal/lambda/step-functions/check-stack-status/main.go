package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/errors"
	"github.com/savaki/aws-deployer/internal/models"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	cfClient *cloudformation.Client
}

type CheckStatusInput struct {
	*models.StepFunctionInput
	DeployResult struct {
		StackName string `json:"stack_name"`
		StackID   string `json:"stack_id"`
		Operation string `json:"operation"`
	} `json:"deployResult"`
}

type StackStatusResult struct {
	Status       string  `json:"status"`
	StatusReason *string `json:"status_reason,omitempty"`
	StackName    string  `json:"stack_name"`
}

func NewHandler() (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &Handler{
		cfClient: cloudformation.NewFromConfig(cfg),
	}, nil
}

func (h *Handler) HandleCheckStackStatus(ctx context.Context, input *CheckStatusInput) (*StackStatusResult, error) {
	logger := zerolog.Ctx(ctx)

	envVar := os.Getenv("ENV")
	if envVar == "" {
		envVar = "dev"
	}
	stackName := fmt.Sprintf("%s-%s", envVar, input.Repo)

	logger.Info().Str("stack_name", stackName).Msg("Checking status of CloudFormation stack")

	result, err := h.cfClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe stack %s: %w", stackName, err)
	}

	if len(result.Stacks) == 0 {
		return nil, fmt.Errorf("%w: %s", errors.ErrStackNotFound, stackName)
	}

	stack := result.Stacks[0]
	status := string(stack.StackStatus)

	statusResult := &StackStatusResult{
		Status:       status,
		StackName:    stackName,
		StatusReason: stack.StackStatusReason,
	}

	logger.Info().
		Str("stack_name", stackName).
		Str("status", status).
		Msg("Stack status")

	if statusResult.StatusReason != nil {
		logger.Info().
			Str("stack_name", stackName).
			Str("reason", *statusResult.StatusReason).
			Msg("Stack status reason")
	}

	if h.isFailedStatus(types.StackStatus(status)) {
		events, err := h.getStackEvents(ctx, stackName)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to get stack events")
		} else {
			logger.Info().Str("stack_name", stackName).Msg("Recent stack events")
			for i := range events {
				event := &events[i]
				if event.ResourceStatusReason != nil {
					logger.Info().
						Str("resource_id", *event.LogicalResourceId).
						Str("status", string(event.ResourceStatus)).
						Str("reason", *event.ResourceStatusReason).
						Msg("Stack event")
				}
			}
		}
	}

	return statusResult, nil
}

func (h *Handler) isFailedStatus(status types.StackStatus) bool {
	failedStatuses := []types.StackStatus{
		types.StackStatusCreateFailed,
		types.StackStatusUpdateFailed,
		types.StackStatusDeleteFailed,
		types.StackStatusRollbackFailed,
		types.StackStatusUpdateRollbackFailed,
		types.StackStatusRollbackComplete,
		types.StackStatusUpdateRollbackComplete,
	}

	for _, failedStatus := range failedStatuses {
		if status == failedStatus {
			return true
		}
	}
	return false
}

func (h *Handler) getStackEvents(ctx context.Context, stackName string) ([]types.StackEvent, error) {
	result, err := h.cfClient.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return nil, err
	}

	var recentEvents []types.StackEvent
	count := 0
	for i := range result.StackEvents {
		if count >= 10 {
			break
		}
		event := &result.StackEvents[i]
		if event.ResourceStatus == types.ResourceStatusCreateFailed ||
			event.ResourceStatus == types.ResourceStatusUpdateFailed ||
			event.ResourceStatus == types.ResourceStatusDeleteFailed {
			recentEvents = append(recentEvents, *event)
			count++
		}
	}

	return recentEvents, nil
}

func main() {
	logger := di.ProvideLogger().With().Str("lambda", "check-stack-status").Logger()

	handler, err := NewHandler()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create handler")
		os.Exit(1)
	}

	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		// Wrap handler to inject logger into context
		wrappedHandler := func(ctx context.Context, input *CheckStatusInput) (*StackStatusResult, error) {
			ctx = logger.WithContext(ctx)
			return handler.HandleCheckStackStatus(ctx, input)
		}
		lambda.Start(wrappedHandler)
		return
	}

	app := &cli.App{
		Name:  "check-stack-status",
		Usage: "Check CloudFormation stack status",
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
				Name:     "version",
				Usage:    "Version (build_number.commit_hash)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "sk",
				Usage:    "Sort key (KSUID)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "commit-hash",
				Usage:    "Commit hash",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "stack-name",
				Usage:    "Stack name from deploy result",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "stack-id",
				Usage:    "Stack ID from deploy result",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "operation",
				Usage:    "Deploy operation (CREATE or UPDATE)",
				Required: true,
			},
		},
		Action: func(c *cli.Context) error {
			input := &CheckStatusInput{
				StepFunctionInput: &models.StepFunctionInput{
					Repo:       c.String("repo"),
					Env:        c.String("env"),
					Version:    c.String("version"),
					SK:         c.String("sk"),
					CommitHash: c.String("commit-hash"),
				},
				DeployResult: struct {
					StackName string `json:"stack_name"`
					StackID   string `json:"stack_id"`
					Operation string `json:"operation"`
				}{
					StackName: c.String("stack-name"),
					StackID:   c.String("stack-id"),
					Operation: c.String("operation"),
				},
			}

			result, err := handler.HandleCheckStackStatus(context.Background(), input)
			if err != nil {
				return err
			}

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
}
