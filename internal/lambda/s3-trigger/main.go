package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/errors"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/segmentio/ksuid"
	"github.com/urfave/cli/v2"
)

// templateNameRegex matches cloudformation-{name}-params.json
// but excludes cloudformation-params.json and cloudformation-params.{env}.json
var templateNameRegex = regexp.MustCompile(`^cloudformation-(.+)-params\.json$`)

type Handler struct {
	dbService *services.DynamoDBService
	targetDAO *targetdao.DAO
}

func NewHandler(dbService *services.DynamoDBService, targetDAO *targetdao.DAO) *Handler {
	return &Handler{
		dbService: dbService,
		targetDAO: targetDAO,
	}
}

// extractTemplateName extracts the template name from a params filename.
// Returns the template name for sub-templates (e.g., "worker" from "cloudformation-worker-params.json")
// Returns empty string for the main template ("cloudformation-params.json") or env-specific files.
func extractTemplateName(filename string) string {
	// Main template params file
	if filename == "cloudformation-params.json" {
		return ""
	}

	// Check for sub-template params file: cloudformation-{name}-params.json
	match := templateNameRegex.FindStringSubmatch(filename)
	if match == nil {
		return ""
	}

	name := match[1]
	// Exclude env-specific files like "cloudformation-params.dev.json"
	// These would match as "params.dev" which contains a dot
	if strings.Contains(name, ".") {
		return ""
	}

	return name
}

func (h *Handler) HandleS3Event(ctx context.Context, event events.S3Event) error {
	logger := zerolog.Ctx(ctx)

	for i := range event.Records {
		if err := h.processS3Record(ctx, &event.Records[i]); err != nil {
			logger.Error().Err(err).Msg("Error processing S3 record")
			return err
		}
	}
	return nil
}

func (h *Handler) processS3Record(ctx context.Context, record *events.S3EventRecord) error {
	logger := zerolog.Ctx(ctx)
	key := record.S3.Object.Key

	// Extract filename from path
	filename := filepath.Base(key)

	// Check if this is a params file we should process
	// Main template: cloudformation-params.json
	// Sub-template: cloudformation-{name}-params.json
	templateName := extractTemplateName(filename)
	isMainTemplate := filename == "cloudformation-params.json"
	isSubTemplate := templateName != ""

	if !isMainTemplate && !isSubTemplate {
		logger.Info().Str("key", key).Msg("Ignoring non-params file")
		return nil
	}

	pathParts := strings.Split(key, "/")
	if len(pathParts) < 4 {
		return fmt.Errorf("%w: %s, expected format: {repo}/{branch}/{version}/cloudformation-params.json",
			errors.ErrInvalidS3KeyFormat, key)
	}

	baseRepo := pathParts[0]
	branch := pathParts[1]
	version := pathParts[2]

	versionParts := strings.Split(version, ".")
	if len(versionParts) < 2 {
		return fmt.Errorf("%w: %s, expected format: {build_number}.{commit_hash}",
			errors.ErrInvalidVersionFormat, version)
	}

	buildNumber := versionParts[0]
	commitHash := strings.Join(versionParts[1:], ".")

	// For sub-templates, repo includes the template name (e.g., "myapp:worker")
	repo := baseRepo
	if templateName != "" {
		repo = fmt.Sprintf("%s:%s", baseRepo, templateName)
	}

	// Query pipeline config to get initial environment
	// Use base repo for config lookup (not sub-template name)
	initialEnv := "dev" // Default fallback
	config, err := h.targetDAO.GetConfig(ctx, baseRepo)
	if err != nil {
		logger.Warn().
			Err(err).
			Str("repo", baseRepo).
			Msg("Failed to get repo-specific config, trying default config")
	}

	// If no repo-specific config or no initialEnv set, try default config
	if config == nil || config.InitialEnv == "" {
		defaultConfig, err := h.targetDAO.GetConfig(ctx, targetdao.DefaultRepo)
		if err != nil {
			logger.Warn().
				Err(err).
				Msg("Failed to get default config, using 'dev' as initial environment")
		}
		if defaultConfig != nil && defaultConfig.InitialEnv != "" {
			config = defaultConfig
		}
	}

	// Use configured initialEnv if available
	if config != nil && config.InitialEnv != "" {
		initialEnv = config.InitialEnv
		logger.Info().
			Str("repo", repo).
			Str("base_repo", baseRepo).
			Str("initial_env", initialEnv).
			Bool("using_default", config.PK.String() == targetdao.DefaultRepo).
			Msg("Using configured initial environment")
	} else {
		logger.Info().
			Str("repo", repo).
			Str("initial_env", initialEnv).
			Msg("No initial environment configured, using default 'dev'")
	}

	// Stack name includes template name for sub-templates
	// Main: {env}-{repo} (e.g., "dev-myapp")
	// Sub:  {env}-{repo}-{template} (e.g., "dev-myapp-worker")
	stackName := fmt.Sprintf("%s-%s", initialEnv, baseRepo)
	if templateName != "" {
		stackName = fmt.Sprintf("%s-%s-%s", initialEnv, baseRepo, templateName)
	}

	// Generate KSUID for this build
	buildKSUID := ksuid.New().String()

	createInput := builddao.CreateInput{
		Repo:         repo,
		Env:          initialEnv,
		SK:           buildKSUID,
		BuildNumber:  buildNumber,
		Branch:       branch,
		Version:      version,
		CommitHash:   commitHash,
		StackName:    stackName,
		TemplateName: templateName,
		BaseRepo:     baseRepo,
	}

	_, err = h.dbService.PutBuild(ctx, createInput)
	if err != nil {
		return fmt.Errorf("failed to save build record: %w", err)
	}

	logger.Info().
		Str("repo", repo).
		Str("base_repo", baseRepo).
		Str("template_name", templateName).
		Str("env", initialEnv).
		Str("ksuid", buildKSUID).
		Str("version", version).
		Str("stack_name", stackName).
		Msg("Created build record with PENDING status")
	return nil
}

func main() {
	logger := di.ProvideLogger().With().Str("lambda", "s3-trigger").Logger()

	// Get ENV to determine which DynamoDB tables to use
	env := os.Getenv("ENV")
	if env == "" {
		env = "dev"
	}

	// Create DI container with necessary providers
	container, err := di.New(env,
		di.WithProviders(
			di.ProvideBuildDAO,
			di.ProvideTargetDAO,
		),
	)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create DI container")
		os.Exit(1)
	}

	// Get services from DI container
	dbService := di.MustGet[*services.DynamoDBService](container)
	targetDAO := di.MustGet[*targetdao.DAO](container)

	// Create handler with injected dependencies
	handler := NewHandler(dbService, targetDAO)

	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		// Wrap handler to inject logger into context
		wrappedHandler := func(ctx context.Context, event events.S3Event) error {
			ctx = logger.WithContext(ctx)
			return handler.HandleS3Event(ctx, event)
		}
		lambda.Start(wrappedHandler)
		return
	}

	app := &cli.App{
		Name:  "s3-trigger",
		Usage: "Simulate S3 event to trigger step function",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "bucket",
				Usage:    "S3 bucket name",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "key",
				Usage:    "S3 object key (e.g., repo/branch/version/cloudformation-params.json)",
				Required: true,
			},
		},
		Action: func(c *cli.Context) error {
			event := events.S3Event{
				Records: []events.S3EventRecord{
					{
						S3: events.S3Entity{
							Bucket: events.S3Bucket{
								Name: c.String("bucket"),
							},
							Object: events.S3Object{
								Key: c.String("key"),
							},
						},
					},
				},
			}

			ctx := logger.WithContext(context.Background())
			return handler.HandleS3Event(ctx, event)
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
}
