package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/orchestrator"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
)

type Handler struct {
	singleAccountOrchestrator *orchestrator.Orchestrator
	multiAccountOrchestrator  *orchestrator.Orchestrator
	config                    *services.Config
	dao                       *builddao.DAO
	targetDAO                 *targetdao.DAO
}

func NewHandler(env string) (*Handler, error) {
	ctx := context.TODO()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create Parameter Store service based on DISABLE_SSM flag
	var paramStore services.ParameterStore
	if os.Getenv("DISABLE_SSM") == "true" {
		paramStore = services.NewEnvParameterStore(env)
	} else {
		ssmClient := di.ProvideSSMClient(cfg)
		paramStore = services.NewSSMParameterStore(ssmClient, env)
	}

	// Load configuration
	appConfig, err := paramStore.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate required config
	if appConfig.StateMachineArn == "" {
		return nil, fmt.Errorf("STATE_MACHINE_ARN required")
	}

	// Create AWS clients and DAOs using derived table names
	dynamoClient := dynamodb.NewFromConfig(cfg)
	dao := builddao.New(dynamoClient, builddao.TableName(env))
	sfnClient := sfn.NewFromConfig(cfg)

	// Create single-account orchestrator
	singleAccountOrch := orchestrator.New(sfnClient, appConfig.StateMachineArn, dao)

	// Create multi-account orchestrator if in multi mode
	var multiAccountOrch *orchestrator.Orchestrator
	var targetDAO *targetdao.DAO
	if appConfig.DeploymentMode == "multi" {
		if appConfig.MultiAccountStateMachineArn == "" {
			return nil, fmt.Errorf("MULTI_ACCOUNT_STATE_MACHINE_ARN required in multi deployment mode")
		}

		multiAccountOrch = orchestrator.New(sfnClient, appConfig.MultiAccountStateMachineArn, dao)
		targetDAO = targetdao.New(dynamoClient, targetdao.TableName(env))
	}

	return &Handler{
		singleAccountOrchestrator: singleAccountOrch,
		multiAccountOrchestrator:  multiAccountOrch,
		config:                    appConfig,
		dao:                       dao,
		targetDAO:                 targetDAO,
	}, nil
}

func (h *Handler) HandleDynamoDBEvent(ctx context.Context, event events.DynamoDBEvent) error {
	logger := zerolog.Ctx(ctx)

	for i := range event.Records {
		record := &event.Records[i]

		// Only process INSERT events (new build records)
		if record.EventName != "INSERT" {
			logger.Info().Str("event_name", record.EventName).Msg("Skipping non-INSERT event")
			continue
		}

		if err := h.processRecord(ctx, record); err != nil {
			logger.Error().
				Err(err).
				Str("event_id", record.EventID).
				Msg("Error processing DynamoDB record")
			return err
		}
	}
	return nil
}

func (h *Handler) processRecord(ctx context.Context, record *events.DynamoDBEventRecord) error {
	logger := zerolog.Ctx(ctx)

	// Convert DynamoDBAttributeValue to types.AttributeValue
	newImage := make(map[string]types.AttributeValue)
	for k, v := range record.Change.NewImage {
		newImage[k] = convertDynamoDBAttributeValue(v)
	}

	// Unmarshal the NewImage to a build record
	var buildRecord builddao.Record
	if err := unmarshalMap(newImage, &buildRecord); err != nil {
		return fmt.Errorf("failed to unmarshal DynamoDB record: %w", err)
	}

	// Skip "latest" magic records (PK starts with "latest/")
	// These are metadata records and should not trigger step function executions
	pkStr := buildRecord.PK.String()
	if len(pkStr) >= 7 && pkStr[:7] == "latest/" {
		logger.Info().
			Str("pk", pkStr).
			Msg("Skipping latest magic record")
		return nil
	}

	logger.Info().
		Str("repo", buildRecord.Repo).
		Str("base_repo", buildRecord.BaseRepo).
		Str("template_name", buildRecord.TemplateName).
		Str("env", buildRecord.Env).
		Str("sk", buildRecord.SK).
		Str("version", buildRecord.Version).
		Msg("Processing new build record")

	// Determine the base repo for S3 key construction
	// For sub-templates, BaseRepo is set; for main templates, use Repo
	baseRepo := buildRecord.BaseRepo
	if baseRepo == "" {
		baseRepo = buildRecord.Repo
	}

	// Construct Step Function input from build record
	input := orchestrator.StepFunctionInput{
		Repo:         buildRecord.Repo,
		Env:          buildRecord.Env,
		Branch:       buildRecord.Branch,
		Version:      buildRecord.Version,
		SK:           buildRecord.SK,
		CommitHash:   buildRecord.CommitHash,
		S3Bucket:     h.config.S3Bucket,
		S3Key:        fmt.Sprintf("%s/%s/%s", baseRepo, buildRecord.Branch, buildRecord.Version),
		TemplateName: buildRecord.TemplateName,
		BaseRepo:     baseRepo,
	}

	// Route to appropriate deployment handler based on mode
	if h.config.DeploymentMode == "multi" {
		return h.processMultiAccountDeployment(ctx, buildRecord, input)
	}
	return h.processSingleAccountDeployment(ctx, buildRecord, input)
}

func (h *Handler) processSingleAccountDeployment(ctx context.Context, buildRecord builddao.Record, input orchestrator.StepFunctionInput) error {
	logger := zerolog.Ctx(ctx)

	logger.Info().
		Str("repo", buildRecord.Repo).
		Str("env", buildRecord.Env).
		Msg("Using single-account deployment")

	// Start Step Functions execution
	executionArn, err := h.singleAccountOrchestrator.StartExecution(ctx, input)
	if err != nil {
		// Update build status to FAILED
		pk := builddao.NewPK(buildRecord.Repo, buildRecord.Env)
		status := builddao.BuildStatusFailed
		errorMsg := fmt.Sprintf("Failed to start step function: %v", err)
		if updateErr := h.dao.UpdateStatus(ctx, builddao.UpdateInput{
			PK:       pk,
			SK:       buildRecord.SK,
			Status:   &status,
			ErrorMsg: &errorMsg,
		}); updateErr != nil {
			logger.Error().Err(updateErr).Msg("Failed to update build status")
		}
		return fmt.Errorf("failed to start execution: %w", err)
	}

	logger.Info().
		Str("execution_arn", executionArn).
		Str("repo", buildRecord.Repo).
		Str("env", buildRecord.Env).
		Str("sk", buildRecord.SK).
		Str("deployment_type", "single-account").
		Msg("Started Step Functions execution")

	return nil
}

func (h *Handler) processMultiAccountDeployment(ctx context.Context, buildRecord builddao.Record, input orchestrator.StepFunctionInput) error {
	logger := zerolog.Ctx(ctx)

	if h.multiAccountOrchestrator == nil {
		return fmt.Errorf("multi-account orchestrator not initialized but deployment mode is multi")
	}

	// Log target information if available
	if h.targetDAO != nil {
		targets, err := h.targetDAO.GetWithDefault(ctx, buildRecord.Repo, buildRecord.Env)
		if err != nil {
			logger.Warn().
				Err(err).
				Str("repo", buildRecord.Repo).
				Str("env", buildRecord.Env).
				Msg("Failed to fetch multi-account targets (will use defaults)")
		} else if targets != nil && len(targets.Targets) > 0 {
			logger.Info().
				Str("repo", buildRecord.Repo).
				Str("env", buildRecord.Env).
				Int("target_count", len(targets.Targets)).
				Msg("Using multi-account deployment with configured targets")
		} else {
			logger.Info().
				Str("repo", buildRecord.Repo).
				Str("env", buildRecord.Env).
				Msg("Using multi-account deployment (no specific targets configured)")
		}
	}

	// Start Step Functions execution
	executionArn, err := h.multiAccountOrchestrator.StartExecution(ctx, input)
	if err != nil {
		// Update build status to FAILED
		pk := builddao.NewPK(buildRecord.Repo, buildRecord.Env)
		status := builddao.BuildStatusFailed
		errorMsg := fmt.Sprintf("Failed to start step function: %v", err)
		if updateErr := h.dao.UpdateStatus(ctx, builddao.UpdateInput{
			PK:       pk,
			SK:       buildRecord.SK,
			Status:   &status,
			ErrorMsg: &errorMsg,
		}); updateErr != nil {
			logger.Error().Err(updateErr).Msg("Failed to update build status")
		}
		return fmt.Errorf("failed to start execution: %w", err)
	}

	logger.Info().
		Str("execution_arn", executionArn).
		Str("repo", buildRecord.Repo).
		Str("env", buildRecord.Env).
		Str("sk", buildRecord.SK).
		Str("deployment_type", "multi-account").
		Msg("Started Step Functions execution")

	return nil
}

// convertDynamoDBAttributeValue converts events.DynamoDBAttributeValue to types.AttributeValue
func convertDynamoDBAttributeValue(av events.DynamoDBAttributeValue) types.AttributeValue {
	switch av.DataType() {
	case events.DataTypeString:
		return &types.AttributeValueMemberS{Value: av.String()}
	case events.DataTypeNumber:
		return &types.AttributeValueMemberN{Value: av.Number()}
	case events.DataTypeBoolean:
		return &types.AttributeValueMemberBOOL{Value: av.Boolean()}
	case events.DataTypeBinary:
		return &types.AttributeValueMemberB{Value: av.Binary()}
	case events.DataTypeNull:
		return &types.AttributeValueMemberNULL{Value: true}
	case events.DataTypeStringSet:
		return &types.AttributeValueMemberSS{Value: av.StringSet()}
	case events.DataTypeNumberSet:
		return &types.AttributeValueMemberNS{Value: av.NumberSet()}
	case events.DataTypeBinarySet:
		return &types.AttributeValueMemberBS{Value: av.BinarySet()}
	case events.DataTypeList:
		list := av.List()
		convertedList := make([]types.AttributeValue, len(list))
		for i, item := range list {
			convertedList[i] = convertDynamoDBAttributeValue(item)
		}
		return &types.AttributeValueMemberL{Value: convertedList}
	case events.DataTypeMap:
		m := av.Map()
		convertedMap := make(map[string]types.AttributeValue)
		for k, v := range m {
			convertedMap[k] = convertDynamoDBAttributeValue(v)
		}
		return &types.AttributeValueMemberM{Value: convertedMap}
	default:
		return &types.AttributeValueMemberNULL{Value: true}
	}
}

// unmarshalMap is a simple wrapper around the SDK's UnmarshalMap for clarity
func unmarshalMap(m map[string]types.AttributeValue, out interface{}) error {
	decoder := func(av types.AttributeValue, v interface{}) error {
		switch val := av.(type) {
		case *types.AttributeValueMemberS:
			if ptr, ok := v.(*string); ok {
				*ptr = val.Value
				return nil
			}
		case *types.AttributeValueMemberN:
			if ptr, ok := v.(*string); ok {
				*ptr = val.Value
				return nil
			}
		}
		return fmt.Errorf("unsupported type conversion")
	}

	// Use a simple manual unmarshaling for the build record
	if buildRecord, ok := out.(*builddao.Record); ok {
		if v, exists := m["pk"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.PK = builddao.PK(s.Value)
			}
		}
		if v, exists := m["sk"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.SK = s.Value
			}
		}
		if v, exists := m["repo"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.Repo = s.Value
			}
		}
		if v, exists := m["env"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.Env = s.Value
			}
		}
		if v, exists := m["branch"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.Branch = s.Value
			}
		}
		if v, exists := m["version"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.Version = s.Value
			}
		}
		if v, exists := m["commit_hash"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.CommitHash = s.Value
			}
		}
		if v, exists := m["template_name"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.TemplateName = s.Value
			}
		}
		if v, exists := m["base_repo"]; exists {
			if s, ok := v.(*types.AttributeValueMemberS); ok {
				buildRecord.BaseRepo = s.Value
			}
		}
		return nil
	}

	_ = decoder
	return fmt.Errorf("unsupported output type")
}

func main() {
	logger := di.ProvideLogger().With().Str("lambda", "trigger-build").Logger()

	// Get environment from ENV or ENVIRONMENT variable
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
		wrappedHandler := func(ctx context.Context, event events.DynamoDBEvent) error {
			ctx = logger.WithContext(ctx)
			return handler.HandleDynamoDBEvent(ctx, event)
		}
		lambda.Start(wrappedHandler)
		return
	}

	// CLI mode
	app := &cli.App{
		Name:  "trigger-build",
		Usage: "Process DynamoDB stream events to trigger Step Functions executions",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "disable-ssm",
				Usage:   "Disable AWS Systems Manager Parameter Store (use environment variables)",
				EnvVars: []string{"DISABLE_SSM"},
			},
		},
		Action: func(c *cli.Context) error {
			handler, err := NewHandler(env)
			if err != nil {
				return fmt.Errorf("failed to create handler: %w", err)
			}

			logger.Info().
				Str("env", env).
				Str("deployment_mode", handler.config.DeploymentMode).
				Msg("CLI mode - handler initialized successfully")
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
}
