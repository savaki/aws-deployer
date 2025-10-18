package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/constants"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	cfClient              *cloudformation.Client
	s3Client              *s3.Client
	administrationRoleARN string
}

type Input struct {
	Env      string `json:"env"`
	Repo     string `json:"repo"`
	SK       string `json:"sk"` // Build KSUID
	S3Bucket string `json:"s3_bucket"`
	S3Key    string `json:"s3_key"` // Prefix like "repo/version/"
}

type Output struct {
	StackSetName string `json:"stack_set_name"`
	Operation    string `json:"operation"` // "CREATE" or "UPDATE"
}

func NewHandler() (*Handler, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Get administration role ARN from environment
	administrationRoleARN := os.Getenv("ADMINISTRATION_ROLE_ARN")
	if administrationRoleARN == "" {
		return nil, fmt.Errorf("ADMINISTRATION_ROLE_ARN environment variable is required")
	}

	return &Handler{
		cfClient:              cloudformation.NewFromConfig(cfg),
		s3Client:              s3.NewFromConfig(cfg),
		administrationRoleARN: administrationRoleARN,
	}, nil
}

func (h *Handler) HandleCreateStackSet(ctx context.Context, input *Input) (*Output, error) {
	logger := zerolog.Ctx(ctx)

	stackSetName := fmt.Sprintf("%s-%s", input.Env, input.Repo)
	templateURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%scloudformation.template",
		input.S3Bucket,
		strings.TrimRight(input.S3Key, "/")+"/")

	logger.Info().
		Str("stack_set_name", stackSetName).
		Str("template_url", templateURL).
		Msg("Creating or updating StackSet")

	// Fetch parameters from S3
	parameters, err := h.fetchParametersFromS3(ctx, input.S3Bucket, input.S3Key, input.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parameters from S3: %w", err)
	}

	// Inject/override the Environment parameter to ensure it matches the deployment environment
	parameters = injectEnvironmentParameter(parameters, input.Env)

	// Check if StackSet exists
	logger.Info().
		Str("stack_set_name", stackSetName).
		Msg("Calling DescribeStackSet API")

	_, err = h.cfClient.DescribeStackSet(ctx, &cloudformation.DescribeStackSetInput{
		StackSetName: aws.String(stackSetName),
	})

	stackSetExists := err == nil

	if stackSetExists {
		// Update existing StackSet (template only, no instance updates)
		// Removing OperationPreferences ensures only the template is updated
		// Instance updates will be handled by DeployStackInstances step
		logger.Info().
			Str("stack_set_name", stackSetName).
			Str("template_url", templateURL).
			Str("administration_role_arn", h.administrationRoleARN).
			Str("execution_role_name", constants.ExecutionRoleName).
			Int("parameter_count", len(parameters)).
			Msg("Calling UpdateStackSet API")

		_, err = h.cfClient.UpdateStackSet(ctx, &cloudformation.UpdateStackSetInput{
			StackSetName:          aws.String(stackSetName),
			TemplateURL:           aws.String(templateURL),
			Parameters:            parameters,
			AdministrationRoleARN: aws.String(h.administrationRoleARN),
			ExecutionRoleName:     aws.String(constants.ExecutionRoleName),
			Capabilities: []types.Capability{
				types.CapabilityCapabilityIam,
				types.CapabilityCapabilityNamedIam,
			},
			// NOTE: No OperationPreferences - this ensures we only update the StackSet template
			// and don't trigger instance updates, which would conflict with DeployStackInstances
		})

		if err != nil {
			// Check for "no updates" error
			var apiErr smithy.APIError
			if ok := asAPIError(err, &apiErr); ok {
				if contains(apiErr.ErrorMessage(), "No updates are to be performed") {
					logger.Info().Str("stack_set_name", stackSetName).Msg("No updates needed for StackSet")
					return &Output{
						StackSetName: stackSetName,
						Operation:    "UPDATE",
					}, nil
				}
			}
			return nil, fmt.Errorf("failed to update StackSet: %w", err)
		}

		logger.Info().
			Str("stack_set_name", stackSetName).
			Msg("UpdateStackSet API call succeeded")
		return &Output{
			StackSetName: stackSetName,
			Operation:    "UPDATE",
		}, nil
	}

	// Create new StackSet
	logger.Info().
		Str("stack_set_name", stackSetName).
		Str("template_url", templateURL).
		Str("administration_role_arn", h.administrationRoleARN).
		Str("execution_role_name", constants.ExecutionRoleName).
		Int("parameter_count", len(parameters)).
		Msg("Calling CreateStackSet API")

	_, err = h.cfClient.CreateStackSet(ctx, &cloudformation.CreateStackSetInput{
		StackSetName:          aws.String(stackSetName),
		TemplateURL:           aws.String(templateURL),
		Parameters:            parameters,
		AdministrationRoleARN: aws.String(h.administrationRoleARN),
		ExecutionRoleName:     aws.String(constants.ExecutionRoleName),
		Capabilities: []types.Capability{
			types.CapabilityCapabilityIam,
			types.CapabilityCapabilityNamedIam,
		},
		Tags: []types.Tag{
			{
				Key:   aws.String("Environment"),
				Value: aws.String(input.Env),
			},
			{
				Key:   aws.String("Repository"),
				Value: aws.String(input.Repo),
			},
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("aws-deployer"),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create StackSet: %w", err)
	}

	logger.Info().
		Str("stack_set_name", stackSetName).
		Msg("CreateStackSet API call succeeded")
	return &Output{
		StackSetName: stackSetName,
		Operation:    "CREATE",
	}, nil
}

// fetchParametersFromS3 reads cloudformation-params.json from S3 and returns CloudFormation parameters
// It first loads cloudformation-params.json (base), then loads cloudformation-params.{env}.json (overrides)
// and merges them, with env-specific values overriding base values for matching keys
// Returns empty parameters if no files exist (parameters are optional)
func (h *Handler) fetchParametersFromS3(ctx context.Context, bucket, key, env string) ([]types.Parameter, error) {
	logger := zerolog.Ctx(ctx)

	keyPrefix := strings.TrimRight(key, "/")

	// Load base params first
	baseKey := fmt.Sprintf("%s/cloudformation-params.json", keyPrefix)
	base, _, err := h.fetchParamsFromKey(ctx, bucket, baseKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", baseKey, err)
	}

	// Load env-specific params
	overrideKey := fmt.Sprintf("%s/cloudformation-params.%s.json", keyPrefix, env)
	override, _, err := h.fetchParamsFromKey(ctx, bucket, overrideKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", overrideKey, err)
	}

	// Both exist - merge them
	merged := mergeParameters(base, override)
	logger.Info().
		Str("env", env).
		Any("base", base).
		Any("override", override).
		Any("merged", merged).
		Msg("Merged base and environment-specific CloudFormation parameters")

	return merged, nil
}

// fetchParamsFromKey fetches and parses a CloudFormation params file from S3
// Returns (parameters, found, error) where found indicates if the file exists
func (h *Handler) fetchParamsFromKey(ctx context.Context, bucket, key string) (map[string]string, bool, error) {
	// Get the object from S3
	result, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if file doesn't exist
		var notFound *s3types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer result.Body.Close()

	// Read the body
	body, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse the JSON as a map (object format: {"Key": "Value"})
	var params map[string]string
	if err := json.Unmarshal(body, &params); err != nil {
		return nil, false, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return params, true, nil
}

func lambdaAction(c *cli.Context) error {
	container, err := di.New(c.String("env"),
		di.WithProviders(
			di.ProvideLogger,
			di.ProvideBuildDAO,
		),
	)
	if err != nil {
		return err
	}

	var (
		logger = di.MustGet[zerolog.Logger](container).With().Str("lambda", "create-stackset").Logger()
		build  = di.MustGet[*builddao.DAO](container)
	)

	handler, err := NewHandler()
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	createStackSet := handler.HandleCreateStackSet
	createStackSet = withLogger(createStackSet, logger)
	createStackSet = withFailBuildOnError(createStackSet, build)

	lambda.Start(createStackSet)
	return nil
}

type HandlerFunc func(context.Context, *Input) (*Output, error)

func withLogger(handler HandlerFunc, logger zerolog.Logger) HandlerFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		ctx = logger.WithContext(ctx)
		return handler(ctx, input)
	}
}

func withFailBuildOnError(handler HandlerFunc, build *builddao.DAO) HandlerFunc {
	return func(ctx context.Context, input *Input) (*Output, error) {
		output, err := handler(ctx, input)
		if err != nil {
			status := builddao.BuildStatusFailed
			updateStatus := builddao.UpdateInput{
				PK:       builddao.NewPK(input.Repo, input.Env),
				SK:       input.SK,
				Status:   &status,
				ErrorMsg: aws.String(err.Error()),
			}
			if err := build.UpdateStatus(ctx, updateStatus); err != nil {
				zerolog.Ctx(ctx).Error().
					Err(err).
					Stringer("id", builddao.NewID(updateStatus.PK, updateStatus.SK)).
					Msg("failed to update build status to FAILED")
			}
			return nil, err
		}

		return output, nil
	}
}

func runAction(c *cli.Context) error {
	logger := di.ProvideLogger().With().Str("lambda", "create-stackset").Logger()

	handler, err := NewHandler()
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}

	// CLI mode for testing
	input := &Input{
		Env:      c.String("env"),
		Repo:     c.String("repo"),
		SK:       c.String("build-id"),
		S3Bucket: c.String("s3-bucket"),
		S3Key:    c.String("s3-key"),
	}

	ctx := logger.WithContext(context.Background())
	result, err := handler.HandleCreateStackSet(ctx, input)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func main() {
	app := &cli.App{
		Name:           "create-stack",
		Usage:          "Create or update CloudFormation StackSet",
		DefaultCommand: "lambda",
		Commands: []*cli.Command{
			{
				Name:  "lambda",
				Usage: "Start Lambda handler",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Usage:    "Environment",
						EnvVars:  []string{"ENV"},
						Required: true,
					},
				},
				Action: lambdaAction,
			},
			{
				Name:  "run",
				Usage: "Run locally for testing",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Usage:    "Environment",
						EnvVars:  []string{"ENV"},
						Required: true,
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
						Name:     "s3-bucket",
						Usage:    "S3 bucket name",
						EnvVars:  []string{"S3_BUCKET"},
						Required: true,
					},
					&cli.StringFlag{
						Name:     "s3-key",
						Usage:    "S3 key prefix",
						EnvVars:  []string{"S3_KEY"},
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

func asAPIError(err error, target *smithy.APIError) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	ok := errors.As(err, &apiErr)
	if ok && target != nil {
		*target = apiErr
	}
	return ok
}

// injectEnvironmentParameter injects or overrides the Env parameter in the parameters list
// This ensures the deployed stack always has the correct environment parameter, regardless of what's in the params file
func injectEnvironmentParameter(parameters []types.Parameter, env string) []types.Parameter {
	// Check if Env parameter already exists
	for i, param := range parameters {
		if aws.ToString(param.ParameterKey) == "Env" {
			// Override the existing value
			parameters[i].ParameterValue = aws.String(env)
			return parameters
		}
	}

	// Env parameter doesn't exist, add it
	return append(parameters, types.Parameter{
		ParameterKey:   aws.String("Env"),
		ParameterValue: aws.String(env),
	})
}

// mergeParameters merges maps provided with the later map having higher precedence
// Returns a new parameter list with merged results
func mergeParameters(pp ...map[string]string) (results []types.Parameter) {
	m := map[string]string{}
	for _, p := range pp {
		for k, v := range p {
			m[k] = v
		}
	}

	for _, k := range slices.Collect(maps.Keys(m)) {
		v := m[k]
		results = append(results, types.Parameter{
			ParameterKey:   aws.String(k),
			ParameterValue: aws.String(v),
		})
	}

	return results
}
