package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/models"
	"github.com/savaki/aws-deployer/internal/policy"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

type Handler struct {
	cfClient  *cloudformation.Client
	s3Client  *s3.Client
	dbService *services.DynamoDBService
	validator *policy.Validator
}

type DownloadResult struct {
	CloudFormationTemplate string            `json:"cloudformation_template"`
	Parameters             []types.Parameter `json:"parameters"`
	S3Objects              []string          `json:"s3_objects"`
}

type DeployResult struct {
	StackName string `json:"stack_name"`
	StackID   string `json:"stack_id"`
	Operation string `json:"operation"`
}

func NewHandler(env string) (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	dbService, err := services.NewDynamoDBService(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB service: %w", err)
	}

	// DISABLED: Rego policy validation temporarily disabled
	// validator, err := policy.NewValidator()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create policy validator: %w", err)
	// }

	return &Handler{
		cfClient:  cloudformation.NewFromConfig(cfg),
		s3Client:  s3.NewFromConfig(cfg),
		dbService: dbService,
		validator: nil, // Disabled for now
	}, nil
}

func (h *Handler) HandleDeployCloudFormation(
	ctx context.Context,
	input *models.StepFunctionInput,
) (result *DeployResult, err error) {
	logger := zerolog.Ctx(ctx)

	defer func(begin time.Time) {
		logger.Info().
			Interface("error", err).
			Str("repo", input.Repo).
			Str("version", input.Version).
			Msg("HandleDeployCloudFormation completed")
	}(time.Now())

	// Step 1: Download S3 content and parse parameters
	logger.Info().Msg("Step 1: Downloading S3 content")
	prefix := strings.TrimRight(input.S3Key, "/") + "/"
	template, err := h.downloadCloudFormationTemplate(ctx, input.S3Bucket, prefix+"cloudformation.template")
	if err != nil {
		return nil, fmt.Errorf("failed to download CloudFormation template: %w", err)
	}

	// Download and merge base + env-specific parameters
	params, err := h.downloadAndParseParams(ctx, input.S3Bucket, prefix+"cloudformation-params.json", input.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to download and parse params: %w", err)
	}

	// Step 1.5: Validate CloudFormation template against policy
	// DISABLED: Rego policy validation temporarily disabled
	// logger.Info().Msg("Step 1.5: Validating CloudFormation template against policy")
	// err = h.validateTemplate(ctx, template, input.Env, input.Repo)
	// if err != nil {
	// 	return nil, fmt.Errorf("CloudFormation template policy validation failed: %w", err)
	// }

	// Step 2: Update build status to IN_PROGRESS
	logger.Info().Msg("Step 2: Updating build status to IN_PROGRESS")
	pk := builddao.NewPK(input.Repo, input.Env)
	status := builddao.BuildStatusInProgress
	_, err = h.dbService.UpdateBuildStatus(ctx, builddao.UpdateInput{
		PK:     pk,
		SK:     input.SK,
		Status: &status,
	})
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to update build status")
		// Continue with deployment even if status update fails
	}

	// Step 3: Deploy CloudFormation stack
	logger.Info().Msg("Step 3: Deploying CloudFormation stack")
	stackName := fmt.Sprintf("%s-%s", input.Env, input.Repo)

	logger.Info().
		Str("stack_name", stackName).
		Str("repo", input.Repo).
		Str("version", input.Version).
		Msg("Deploying stack")

	stackExists, err := h.stackExists(ctx, stackName)
	if err != nil {
		return nil, fmt.Errorf("failed to check if stack exists: %w", err)
	}

	if stackExists {
		result, err = h.updateStack(ctx, stackName, template, params)
		if err != nil {
			return nil, fmt.Errorf("failed to update stack: %w", err)
		}
		result.Operation = "UPDATE"
	} else {
		result, err = h.createStack(ctx, stackName, template, params)
		if err != nil {
			return nil, fmt.Errorf("failed to create stack: %w", err)
		}
		result.Operation = "CREATE"
	}

	logger.Info().
		Str("operation", result.Operation).
		Str("stack_name", stackName).
		Msg("Stack deployment completed")
	return result, nil
}

func (h *Handler) stackExists(ctx context.Context, stackName string) (bool, error) {
	_, err := h.cfClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "ValidationError" || strings.Contains(apiErr.ErrorMessage(), "does not exist") {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func (h *Handler) createStack(
	ctx context.Context,
	stackName, template string,
	parameters []types.Parameter,
) (*DeployResult, error) {
	input := &cloudformation.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(template),
		Parameters:   parameters,
		Capabilities: []types.Capability{
			types.CapabilityCapabilityIam,
			types.CapabilityCapabilityNamedIam,
		},
		Tags: []types.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("aws-deployer"),
			},
		},
	}

	result, err := h.cfClient.CreateStack(ctx, input)
	if err != nil {
		return nil, err
	}

	return &DeployResult{
		StackName: stackName,
		StackID:   *result.StackId,
	}, nil
}

func (h *Handler) updateStack(
	ctx context.Context,
	stackName, template string,
	parameters []types.Parameter,
) (*DeployResult, error) {
	logger := zerolog.Ctx(ctx)

	input := &cloudformation.UpdateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(template),
		Parameters:   parameters,
		Capabilities: []types.Capability{
			types.CapabilityCapabilityIam,
			types.CapabilityCapabilityNamedIam,
		},
	}

	result, err := h.cfClient.UpdateStack(ctx, input)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "ValidationError" &&
				(strings.Contains(apiErr.ErrorMessage(), "No updates are to be performed") ||
					strings.Contains(apiErr.ErrorMessage(), "No updates to be performed")) {
				logger.Info().Str("stack_name", stackName).Msg("No updates needed for stack")
				return &DeployResult{
					StackName: stackName,
					StackID:   stackName,
				}, nil
			}
		}
		return nil, err
	}

	return &DeployResult{
		StackName: stackName,
		StackID:   *result.StackId,
	}, nil
}

func (h *Handler) downloadAndParseParams(ctx context.Context, bucket, key, env string) ([]types.Parameter, error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("bucket", bucket).
		Str("key", key).
		Str("env", env).
		Msg("Downloading and parsing parameters")

	defer func() {
		logger.Info().
			Str("bucket", bucket).
			Str("key", key).
			Msg("Finished downloading parameters")
	}()

	// Download base parameters (cloudformation-params.json)
	content, err := h.downloadS3Object(ctx, bucket, key)
	if err != nil {
		return nil, err
	}

	var baseParams []types.Parameter
	if err := json.Unmarshal([]byte(content), &baseParams); err != nil {
		return nil, fmt.Errorf("failed to parse JSON parameters: %w", err)
	}

	// Try to download env-specific parameters (cloudformation-params.{env}.json)
	envKey := replaceFilename(key, fmt.Sprintf("cloudformation-params.%s.json", env))
	envContent, err := h.downloadS3Object(ctx, bucket, envKey)
	if err != nil {
		logger.Info().
			Str("env_key", envKey).
			Msg("No env-specific parameters found, using base parameters only")
		return baseParams, nil
	}

	// Parse env-specific parameters
	var envParams []types.Parameter
	if err := json.Unmarshal([]byte(envContent), &envParams); err != nil {
		logger.Warn().
			Err(err).
			Str("env_key", envKey).
			Msg("Failed to parse env-specific parameters, using base parameters only")
		return baseParams, nil
	}

	// Merge env-specific params on top of base params
	merged := mergeParameters(baseParams, envParams)

	logger.Info().
		Int("base_count", len(baseParams)).
		Int("env_count", len(envParams)).
		Int("merged_count", len(merged)).
		Msg("Merged base and env-specific parameters")

	return merged, nil
}

// mergeParameters merges env-specific parameters on top of base parameters
// If a parameter exists in both, env-specific wins
func mergeParameters(base, override []types.Parameter) []types.Parameter {
	// Create map of override parameters by key
	overrideMap := make(map[string]types.Parameter)
	for _, param := range override {
		if param.ParameterKey != nil {
			overrideMap[*param.ParameterKey] = param
		}
	}

	// Build result: base params with overrides applied
	result := make([]types.Parameter, 0, len(base)+len(override))
	seen := make(map[string]bool)

	for _, param := range base {
		if param.ParameterKey == nil {
			continue
		}
		key := *param.ParameterKey

		if overrideParam, exists := overrideMap[key]; exists {
			result = append(result, overrideParam)
		} else {
			result = append(result, param)
		}
		seen[key] = true
	}

	// Add any new parameters from override that weren't in base
	for _, param := range override {
		if param.ParameterKey != nil {
			key := *param.ParameterKey
			if !seen[key] {
				result = append(result, param)
			}
		}
	}

	return result
}

// replaceFilename replaces the filename in an S3 key
func replaceFilename(key, newFilename string) string {
	lastSlash := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash >= 0 {
		return key[:lastSlash+1] + newFilename
	}
	return newFilename
}

func (h *Handler) downloadCloudFormationTemplate(ctx context.Context, bucket, key string) (s string, err error) {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("bucket", bucket).
		Str("key", key).
		Msg("Downloading CloudFormation template")

	return h.downloadS3Object(ctx, bucket, key)
}

func (h *Handler) downloadS3Object(ctx context.Context, bucket, key string) (s string, err error) {
	logger := zerolog.Ctx(ctx)

	defer func(begin time.Time) {
		logger.Info().
			Int("length", len(s)).
			Interface("error", err).
			Str("bucket", bucket).
			Str("key", key).
			Dur("duration", time.Since(begin)).
			Msg("Downloaded S3 object")
	}(time.Now())

	result, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get object %s from bucket %s: %w", key, bucket, err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer result.Body.Close()

	content, err := io.ReadAll(result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read object content: %w", err)
	}

	return string(content), nil
}

func (h *Handler) validateTemplate(ctx context.Context, templateString, env, repo string) error {
	logger := zerolog.Ctx(ctx)

	var template map[string]interface{}
	if err := yaml.Unmarshal([]byte(templateString), &template); err != nil {
		return fmt.Errorf("failed to parse CloudFormation template: %w", err)
	}

	result, err := h.validator.ValidateTemplate(template, env, repo)
	if err != nil {
		return fmt.Errorf("policy validation error: %w", err)
	}

	if !result.Allowed {
		violationsStr := strings.Join(result.Violations, "; ")
		logger.Error().
			Str("repo", repo).
			Str("env", env).
			Str("violations", violationsStr).
			Msg("CloudFormation template policy validation failed")
		return fmt.Errorf("policy violations: %s", violationsStr)
	}

	logger.Info().
		Str("repo", repo).
		Str("env", env).
		Msg("CloudFormation template validated successfully")
	return nil
}

func main() {
	logger := di.ProvideLogger().With().Str("lambda", "deploy-cloudformation").Logger()

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
		wrappedHandler := func(ctx context.Context, input *models.StepFunctionInput) (*DeployResult, error) {
			ctx = logger.WithContext(ctx)
			return handler.HandleDeployCloudFormation(ctx, input)
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
		Name:  "deploy-cloudformation",
		Usage: "Deploy CloudFormation stacks from S3 content",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "env",
				Usage:   "Environment (dev, staging, prod)",
				EnvVars: []string{"ENV"},
				Value:   "dev",
			},
			&cli.StringFlag{
				Name:     "repo",
				Usage:    "Repository name",
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
				Name:     "s3-bucket",
				Usage:    "S3 bucket containing deployment files",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "s3-key",
				Usage:    "S3 key prefix (e.g., repo/version)",
				Required: true,
			},
		},
		Action: func(c *cli.Context) error {
			input := &models.StepFunctionInput{
				Env:        c.String("env"),
				Repo:       c.String("repo"),
				Version:    c.String("version"),
				SK:         c.String("sk"),
				CommitHash: c.String("commit-hash"),
				S3Bucket:   c.String("s3-bucket"),
				S3Key:      c.String("s3-key"),
			}

			result, err := handler.HandleDeployCloudFormation(context.Background(), input)
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
