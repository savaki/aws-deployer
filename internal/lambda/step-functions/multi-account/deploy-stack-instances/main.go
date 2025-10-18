package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	cfClient *cloudformation.Client
}

type DeploymentTarget struct {
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
}

type Input struct {
	StackSetName string             `json:"stack_set_name"`
	Targets      []DeploymentTarget `json:"targets"`
}

type Output struct {
	OperationID string   `json:"operation_id"`
	AccountIDs  []string `json:"account_ids"`
	Regions     []string `json:"regions"`
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

func (h *Handler) HandleDeployStackInstances(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	// Extract unique accounts and regions
	accountSet := make(map[string]bool)
	regionSet := make(map[string]bool)

	for _, target := range input.Targets {
		accountSet[target.AccountID] = true
		regionSet[target.Region] = true
	}

	accounts := make([]string, 0, len(accountSet))
	for account := range accountSet {
		accounts = append(accounts, account)
	}

	regions := make([]string, 0, len(regionSet))
	for region := range regionSet {
		regions = append(regions, region)
	}

	logger.Info().
		Str("stack_set_name", input.StackSetName).
		Int("account_count", len(accounts)).
		Int("region_count", len(regions)).
		Int("total_instances", len(input.Targets)).
		Msg("Deploying stack instances")

	// Create or update stack instances with retry on OperationInProgressException
	operationID, err := h.createStackInstancesWithRetry(ctx, input.StackSetName, accounts, regions)
	if err != nil {
		return nil, err
	}

	logger.Info().
		Str("stack_set_name", input.StackSetName).
		Str("operation_id", operationID).
		Int("total_instances", len(input.Targets)).
		Msg("Stack instances deployment initiated")

	return &Output{
		OperationID: operationID,
		AccountIDs:  accounts,
		Regions:     regions,
	}, nil
}

// createStackInstancesWithRetry attempts to create stack instances
func (h *Handler) createStackInstancesWithRetry(ctx context.Context, stackSetName string, accounts, regions []string) (string, error) {
	logger := zerolog.Ctx(ctx)

	// First, check which instances already exist
	existingInstances, err := h.getExistingInstances(ctx, stackSetName, accounts, regions)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get existing instances, will attempt creation anyway")
		existingInstances = make(map[string]bool)
	}

	// Filter out accounts/regions that already have instances
	newAccounts := make([]string, 0)
	newRegions := make([]string, 0)

	for _, account := range accounts {
		for _, region := range regions {
			key := fmt.Sprintf("%s/%s", account, region)
			if !existingInstances[key] {
				// Only add account/region pairs that don't exist
				newAccounts = appendUnique(newAccounts, account)
				newRegions = appendUnique(newRegions, region)
			}
		}
	}

	// If no new instances to create, just update existing ones
	if len(newAccounts) == 0 || len(newRegions) == 0 {
		logger.Info().
			Str("stack_set_name", stackSetName).
			Msg("All instances already exist, updating instead of creating")
		return h.updateStackInstancesWithRetry(ctx, stackSetName, accounts, regions)
	}

	logger.Info().
		Str("stack_set_name", stackSetName).
		Int("new_accounts", len(newAccounts)).
		Int("new_regions", len(newRegions)).
		Int("existing_instances", len(existingInstances)).
		Msg("Calling CreateStackInstances API")

	result, err := h.cfClient.CreateStackInstances(ctx, &cloudformation.CreateStackInstancesInput{
		StackSetName: aws.String(stackSetName),
		Accounts:     newAccounts,
		Regions:      newRegions,
		OperationPreferences: &types.StackSetOperationPreferences{
			MaxConcurrentCount:    aws.Int32(10),
			FailureToleranceCount: aws.Int32(0), // Fail fast - we want to track all failures
		},
	})

	if err == nil {
		// Success - return operation ID
		if result.OperationId != nil {
			logger.Info().
				Str("stack_set_name", stackSetName).
				Str("operation_id", *result.OperationId).
				Msg("CreateStackInstances API call succeeded")
			return *result.OperationId, nil
		}
		return "", nil
	}

	// Check if it's an OperationInProgressException - return as retryable error
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == "OperationInProgressException" {
			operationID := extractOperationID(apiErr.ErrorMessage())
			logger.Info().
				Str("stack_set_name", stackSetName).
				Str("in_progress_operation_id", operationID).
				Msg("Another operation in progress, returning error for Step Functions retry")

			// Return the error - Step Functions will retry
			return "", fmt.Errorf("operation in progress (will retry): %w", err)
		}
	}

	// Not an OperationInProgressException - fail
	return "", fmt.Errorf("failed to create stack instances: %w", err)
}

// updateStackInstancesWithRetry updates existing stack instances
func (h *Handler) updateStackInstancesWithRetry(ctx context.Context, stackSetName string, accounts, regions []string) (string, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("stack_set_name", stackSetName).
		Int("account_count", len(accounts)).
		Int("region_count", len(regions)).
		Msg("Calling UpdateStackInstances API")

	result, err := h.cfClient.UpdateStackInstances(ctx, &cloudformation.UpdateStackInstancesInput{
		StackSetName: aws.String(stackSetName),
		Accounts:     accounts,
		Regions:      regions,
		OperationPreferences: &types.StackSetOperationPreferences{
			MaxConcurrentCount:    aws.Int32(10),
			FailureToleranceCount: aws.Int32(0),
		},
	})

	if err == nil {
		// Success - return operation ID
		if result.OperationId != nil {
			logger.Info().
				Str("stack_set_name", stackSetName).
				Str("operation_id", *result.OperationId).
				Msg("UpdateStackInstances API call succeeded")
			return *result.OperationId, nil
		}
		return "", nil
	}

	// Check if it's an OperationInProgressException - return as retryable error
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == "OperationInProgressException" {
			operationID := extractOperationID(apiErr.ErrorMessage())
			logger.Info().
				Str("stack_set_name", stackSetName).
				Str("in_progress_operation_id", operationID).
				Msg("Another operation in progress, returning error for Step Functions retry")

			// Return the error - Step Functions will retry
			return "", fmt.Errorf("operation in progress (will retry): %w", err)
		}
	}

	return "", fmt.Errorf("failed to update stack instances: %w", err)
}

// getExistingInstances returns a map of account/region keys for existing instances
func (h *Handler) getExistingInstances(ctx context.Context, stackSetName string, accounts, regions []string) (map[string]bool, error) {
	logger := zerolog.Ctx(ctx)
	existing := make(map[string]bool)

	logger.Debug().
		Str("stack_set_name", stackSetName).
		Msg("Calling ListStackInstances API")

	// List stack instances for this StackSet
	paginator := cloudformation.NewListStackInstancesPaginator(h.cfClient, &cloudformation.ListStackInstancesInput{
		StackSetName: aws.String(stackSetName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			// If StackSet doesn't exist yet, no instances exist
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) {
				if apiErr.ErrorCode() == "StackSetNotFoundException" {
					logger.Info().
						Str("stack_set_name", stackSetName).
						Msg("StackSet not found, no existing instances")
					return existing, nil
				}
			}
			return nil, fmt.Errorf("failed to list stack instances: %w", err)
		}

		for _, instance := range page.Summaries {
			if instance.Account != nil && instance.Region != nil {
				key := fmt.Sprintf("%s/%s", *instance.Account, *instance.Region)
				existing[key] = true
			}
		}
	}

	logger.Info().
		Str("stack_set_name", stackSetName).
		Int("existing_count", len(existing)).
		Msg("Found existing stack instances")

	return existing, nil
}

// appendUnique appends a string to a slice only if it's not already present
func appendUnique(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

// extractOperationID extracts the operation ID from the error message
// Example: "Another Operation on StackSet ... is in progress: 4639ab1f-c0d1-4a31-9939-19f101f968a7"
func extractOperationID(errorMsg string) string {
	re := regexp.MustCompile(`is in progress: ([a-f0-9-]+)`)
	matches := re.FindStringSubmatch(errorMsg)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func lambdaAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "deploy-stack-instances").Logger()
	handler, err := NewHandler()
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	wrappedHandler := func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler.HandleDeployStackInstances(ctx, input)
	}
	lambda.Start(wrappedHandler)
	return nil
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "deploy-stack-instances").Logger()

	handler, err := NewHandler()
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// CLI mode for testing
	var targets []DeploymentTarget
	if err := json.Unmarshal([]byte(c.String("targets")), &targets); err != nil {
		return fmt.Errorf("failed to parse targets: %w", err)
	}

	input := &Input{
		StackSetName: c.String("stack-set-name"),
		Targets:      targets,
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleDeployStackInstances(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "deploy-stack-instances",
		Usage:          "Deploy StackSet instances to target accounts/regions",
		DefaultCommand: "lambda",
		Commands: []*cli.Command{
			{
				Name:   "lambda",
				Usage:  "Start Lambda handler",
				Action: lambdaAction,
			},
			{
				Name:  "run",
				Usage: "Run locally for testing",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "stack-set-name",
						Usage:    "StackSet name",
						EnvVars:  []string{"STACK_SET_NAME"},
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
