package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/gox/slicex"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	cfClient      *cloudformation.Client
	deploymentDAO *deploymentdao.DAO
}

type DeploymentTarget struct {
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
}

type Input struct {
	Env          string             `json:"env"`
	Repo         string             `json:"repo"`
	StackSetName string             `json:"stack_set_name"`
	OperationID  string             `json:"operation_id"`
	Targets      []DeploymentTarget `json:"targets"`
}

type DeploymentStatus struct {
	AccountID      string   `json:"account_id"`
	Region         string   `json:"region"`
	Status         string   `json:"status"`          // CURRENT, OUTDATED, INOPERABLE
	DetailedStatus string   `json:"detailed_status"` // PENDING, RUNNING, SUCCEEDED, FAILED, etc.
	StatusReason   string   `json:"status_reason,omitempty"`
	StackID        string   `json:"stack_id,omitempty"`
	StackEvents    []string `json:"stack_events,omitempty"`
}

type Output struct {
	OperationStatus string             `json:"operation_status"` // RUNNING, SUCCEEDED, FAILED, STOPPING, STOPPED
	Deployments     []DeploymentStatus `json:"deployments"`
	IsComplete      bool               `json:"is_complete"`
	HasFailures     bool               `json:"has_failures"`
}

func NewHandler(deploymentsTableName string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	cfClient := cloudformation.NewFromConfig(cfg)
	dbClient := dynamodb.NewFromConfig(cfg)
	deploymentDAO := deploymentdao.New(dbClient, deploymentsTableName)

	return &Handler{
		cfClient:      cfClient,
		deploymentDAO: deploymentDAO,
	}, nil
}

func (h *Handler) HandleCheckStackSetStatus(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("stack_set_name", input.StackSetName).
		Str("operation_id", input.OperationID).
		Msg("Calling DescribeStackSetOperation API")

	// Describe the StackSet operation
	opResult, err := h.cfClient.DescribeStackSetOperation(ctx, &cloudformation.DescribeStackSetOperationInput{
		StackSetName: aws.String(input.StackSetName),
		OperationId:  aws.String(input.OperationID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe StackSet operation: %w", err)
	}

	operation := opResult.StackSetOperation
	operationStatus := string(operation.Status)

	logger.Info().
		Str("stack_set_name", input.StackSetName).
		Str("operation_id", input.OperationID).
		Str("operation_status", operationStatus).
		Msg("DescribeStackSetOperation API call succeeded")

	// Check status for all stack instances concurrently with concurrency of 8
	callback := func(ctx context.Context, target DeploymentTarget) (*DeploymentStatus, error) {
		return h.getInstanceStatus(ctx, input.StackSetName, target.AccountID, target.Region)
	}
	statuses, err := slicex.MapConcurrent(callback).
		Concurrency(8).
		CollectErrors().
		DoValues(ctx, input.Targets...)

	if err != nil {
		return nil, fmt.Errorf("failed to check stack instance statuses: %w", err)
	}

	// Process results
	var deployments []DeploymentStatus
	hasFailures := false
	allInstancesComplete := true

	for i, status := range statuses {
		target := input.Targets[i]

		if status == nil {
			logger.Warn().
				Str("account", target.AccountID).
				Str("region", target.Region).
				Msg("Failed to get instance status")
			allInstancesComplete = false
			continue
		}

		deployments = append(deployments, *status)

		// Check if instance is in a terminal state by examining the DetailedStatus
		// This tells us if an operation is actively running (PENDING/RUNNING) vs completed
		if !isTerminalStatus(status.DetailedStatus) {
			allInstancesComplete = false
			logger.Info().
				Str("account", target.AccountID).
				Str("region", target.Region).
				Str("status", status.Status).
				Str("detailed_status", status.DetailedStatus).
				Msg("Stack instance still in progress")
		}

		// Update deployment state in DynamoDB
		deploymentStatus := deploymentdao.StatusInProgress
		if status.Status == "CURRENT" {
			deploymentStatus = deploymentdao.StatusSuccess
		} else if status.Status == "OUTDATED" || status.Status == "FAILED" {
			deploymentStatus = deploymentdao.StatusFailed
			hasFailures = true
		}

		err := h.deploymentDAO.UpdateStatus(ctx, deploymentdao.UpdateInput{
			Env:          input.Env,
			Repo:         input.Repo,
			Account:      target.AccountID,
			Region:       target.Region,
			Status:       deploymentStatus,
			StackID:      status.StackID,
			StatusReason: status.StatusReason,
			StackEvents:  status.StackEvents,
		})
		if err != nil {
			logger.Warn().
				Err(err).
				Str("account", target.AccountID).
				Str("region", target.Region).
				Msg("Failed to update deployment status")
		}
	}

	// Operation is complete only when both:
	// 1. The overall operation has finished (SUCCEEDED, FAILED, or STOPPED)
	// 2. ALL individual stack instances are in terminal states (not RUNNING, STOPPING, etc.)
	operationComplete := operationStatus == "SUCCEEDED" || operationStatus == "FAILED" || operationStatus == "STOPPED"
	isComplete := operationComplete && allInstancesComplete

	logger.Info().
		Str("operation_status", operationStatus).
		Bool("operation_complete", operationComplete).
		Bool("all_instances_complete", allInstancesComplete).
		Bool("is_complete", isComplete).
		Bool("has_failures", hasFailures).
		Int("deployment_count", len(deployments)).
		Msg("StackSet status check complete")

	return &Output{
		OperationStatus: operationStatus,
		Deployments:     deployments,
		IsComplete:      isComplete,
		HasFailures:     hasFailures,
	}, nil
}

// getInstanceStatus retrieves the status of a single stack instance
func (h *Handler) getInstanceStatus(ctx context.Context, stackSetName, account, region string) (*DeploymentStatus, error) {
	logger := zerolog.Ctx(ctx)

	logger.Debug().
		Str("stack_set_name", stackSetName).
		Str("account", account).
		Str("region", region).
		Msg("Calling DescribeStackInstance API")

	result, err := h.cfClient.DescribeStackInstance(ctx, &cloudformation.DescribeStackInstanceInput{
		StackSetName:         aws.String(stackSetName),
		StackInstanceAccount: aws.String(account),
		StackInstanceRegion:  aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe stack instance: %w", err)
	}

	instance := result.StackInstance
	status := string(instance.Status)

	logger.Debug().
		Str("stack_set_name", stackSetName).
		Str("account", account).
		Str("region", region).
		Str("status", status).
		Msg("DescribeStackInstance API call succeeded")
	statusReason := ""
	if instance.StatusReason != nil {
		statusReason = *instance.StatusReason
	}

	stackID := ""
	if instance.StackId != nil {
		stackID = *instance.StackId
	}

	// Check detailed status to see if operation is in progress
	// The Status field shows the sync state (CURRENT/OUTDATED/INOPERABLE)
	// The DetailedStatus shows if an operation is actively running (PENDING/RUNNING/etc)
	detailedStatus := ""
	if instance.StackInstanceStatus != nil && instance.StackInstanceStatus.DetailedStatus != "" {
		detailedStatus = string(instance.StackInstanceStatus.DetailedStatus)
		logger.Debug().
			Str("account", account).
			Str("region", region).
			Str("status", status).
			Str("detailed_status", detailedStatus).
			Msg("Stack instance status details")
	}

	deploymentStatus := &DeploymentStatus{
		AccountID:      account,
		Region:         region,
		Status:         status,
		DetailedStatus: detailedStatus,
		StatusReason:   statusReason,
		StackID:        stackID,
	}

	// If failed, get stack events for additional context
	if status == "OUTDATED" || status == "FAILED" || status == "INOPERABLE" {
		events := h.getFailedStackEvents(ctx, stackID, logger)
		deploymentStatus.StackEvents = events
	}

	return deploymentStatus, nil
}

// getFailedStackEvents retrieves recent failed stack events
func (h *Handler) getFailedStackEvents(ctx context.Context, stackID string, logger *zerolog.Logger) []string {
	if stackID == "" {
		return nil
	}

	logger.Debug().
		Str("stack_id", stackID).
		Msg("Calling DescribeStackEvents API")

	result, err := h.cfClient.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(stackID),
	})
	if err != nil {
		logger.Warn().Err(err).Str("stack_id", stackID).Msg("Failed to get stack events")
		return nil
	}

	logger.Debug().
		Str("stack_id", stackID).
		Int("event_count", len(result.StackEvents)).
		Msg("DescribeStackEvents API call succeeded")

	var events []string
	for i, event := range result.StackEvents {
		if i >= 5 { // Limit to 5 most recent failed events
			break
		}

		// Only include failed events
		if isFailedStatus(event.ResourceStatus) {
			eventStr := fmt.Sprintf("%s: %s - %s",
				*event.LogicalResourceId,
				event.ResourceStatus,
				getStringValue(event.ResourceStatusReason))
			events = append(events, eventStr)
		}
	}

	return events
}

func isFailedStatus(status types.ResourceStatus) bool {
	failedStatuses := []types.ResourceStatus{
		types.ResourceStatusCreateFailed,
		types.ResourceStatusUpdateFailed,
		types.ResourceStatusDeleteFailed,
	}

	for _, fs := range failedStatuses {
		if status == fs {
			return true
		}
	}
	return false
}

func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isTerminalStatus returns true if the stack instance is in a terminal state
// Checks the DetailedStatus for operation progress (PENDING, RUNNING are non-terminal)
// Terminal DetailedStatus values: SUCCEEDED, FAILED, CANCELLED, INOPERABLE, SKIPPED_SUSPENDED_ACCOUNT
// Non-terminal DetailedStatus values: PENDING, RUNNING
func isTerminalStatus(detailedStatus string) bool {
	// If no detailed status is provided, consider it terminal
	// (this handles cases where the field might not be populated)
	if detailedStatus == "" {
		return true
	}

	// Non-terminal statuses indicate an operation is still in progress
	nonTerminalStatuses := []string{
		"PENDING", // Operation is queued
		"RUNNING", // Operation is actively executing
	}

	for _, nts := range nonTerminalStatuses {
		if detailedStatus == nts {
			return false
		}
	}

	// All other statuses are terminal: SUCCEEDED, FAILED, CANCELLED, INOPERABLE, SKIPPED_SUSPENDED_ACCOUNT
	return true
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "check-stackset-status").Logger()
	tableName := deploymentdao.TableName(c.String("env"))
	handler, err := NewHandler(tableName)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleCheckStackSetStatus(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "check-stackset-status").Logger()

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
		Env:          c.String("env"),
		Repo:         c.String("repo"),
		StackSetName: c.String("stack-set-name"),
		OperationID:  c.String("operation-id"),
		Targets:      targets,
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleCheckStackSetStatus(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "check-stackset-status",
		Usage:          "Check StackSet operation status and update deployment states",
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
						Name:     "stack-set-name",
						Usage:    "StackSet name",
						EnvVars:  []string{"STACK_SET_NAME"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "operation-id",
						Usage:    "StackSet operation ID",
						EnvVars:  []string{"OPERATION_ID"},
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
