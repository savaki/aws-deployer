package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
	"github.com/urfave/cli/v2"
)

// TargetsCommand returns the targets command for managing deployment targets
func TargetsCommand(logger *zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:    "targets",
		Aliases: []string{"t"},
		Usage:   "Manage multi-account CloudFormation deployment targets",
		Description: `Manage deployment targets that define which AWS accounts and regions to deploy to.

Two environment concepts:
  --env:        AWS Deployer instance environment (dev/stg/prd) - determines which DynamoDB table to use
  --target-env: Target deployment environment - the environment being deployed to

Targets can be configured as:
  - Default targets (apply to all repos unless overridden)
  - Per-repository targets (repo-specific configuration)`,
		Subcommands: []*cli.Command{
			{
				Name:    "set",
				Aliases: []string{"s"},
				Usage:   "Set deployment targets",
				Description: `Set deployment targets for a repo/env or default configuration.

Examples:
  # Set default targets using dev aws-deployer for dev target environment
  aws-deployer targets set --env dev --target-env dev --default \
    --accounts "123456789012,987654321098" \
    --regions "us-east-1,us-west-2"

  # Set targets for a specific repo using prd aws-deployer for prd target environment
  aws-deployer targets set --env prd --target-env prd --repo my-app \
    --accounts "123456789012" \
    --regions "us-east-1,us-west-2,eu-west-1"

  # Use JSON for complex targets
  aws-deployer targets set --env stg --target-env stg --default \
    --targets-json '[{"account_ids":["123456789012"],"regions":["us-east-1","us-west-2"]}]'

  # Overwrite existing targets
  aws-deployer targets set --env dev --target-env dev --repo my-app \
    --accounts "123456789012" \
    --regions "us-east-1" \
    --overwrite`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Aliases:  []string{"e"},
						Usage:    "AWS Deployer environment (dev, stg, or prd) - determines which DynamoDB table to use",
						Required: true,
						EnvVars:  []string{"ENV"},
					},
					&cli.StringFlag{
						Name:     "target-env",
						Aliases:  []string{"t"},
						Usage:    "Target deployment environment (dev, stg, or prd) - the environment being deployed to",
						Required: true,
						EnvVars:  []string{"TARGET_ENV"},
					},
					&cli.BoolFlag{
						Name:    "default",
						Aliases: []string{"d"},
						Usage:   "Set default targets (applies to all repos)",
					},
					&cli.StringFlag{
						Name:    "repo",
						Aliases: []string{"r"},
						Usage:   "Repository name (for repo-specific targets)",
						EnvVars: []string{"REPO"},
					},
					&cli.StringFlag{
						Name:    "accounts",
						Aliases: []string{"a"},
						Usage:   "Comma-separated list of AWS account IDs",
						EnvVars: []string{"ACCOUNTS"},
					},
					&cli.StringFlag{
						Name:    "regions",
						Aliases: []string{"g"},
						Usage:   "Comma-separated list of AWS regions",
						EnvVars: []string{"REGIONS"},
					},
					&cli.StringFlag{
						Name:    "targets-json",
						Aliases: []string{"j"},
						Usage:   "Targets as JSON array (alternative to --accounts and --regions)",
						EnvVars: []string{"TARGETS_JSON"},
					},
					&cli.StringFlag{
						Name:    "downstream-env",
						Aliases: []string{"p"},
						Usage:   "Comma-separated list of downstream environments (e.g., 'stg' for dev, 'prd' for stg)",
						EnvVars: []string{"DOWNSTREAM_ENV"},
					},
					&cli.BoolFlag{
						Name:    "overwrite",
						Aliases: []string{"o"},
						Usage:   "Overwrite existing targets if they exist",
					},
				},
				Action: setAction,
			},
			{
				Name:    "list",
				Aliases: []string{"l", "get", "show"},
				Usage:   "List deployment targets",
				Description: `List deployment targets for a repo/env or default configuration.

When no repo is specified, shows default targets. When a repo is specified, shows repo-specific
targets and automatically falls back to default targets if none are configured.

Examples:
  # List all target environments for default targets
  aws-deployer targets list --env dev

  # List all target environments for a specific repo (falls back to default if not configured)
  aws-deployer targets list --env prd --repo my-app

  # List specific target environment for a repo (falls back to default if not configured)
  aws-deployer targets list --env stg --target-env stg --repo my-app

  # List all targets as JSON
  aws-deployer targets list --env dev --json`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Aliases:  []string{"e"},
						Usage:    "AWS Deployer environment (dev, stg, or prd) - determines which DynamoDB table to use",
						Required: true,
						EnvVars:  []string{"ENV"},
					},
					&cli.StringFlag{
						Name:     "target-env",
						Aliases:  []string{"t"},
						Usage:    "Target deployment environment (dev, stg, or prd) - if not specified, shows all environments",
						Required: false,
						EnvVars:  []string{"TARGET_ENV"},
					},
					&cli.StringFlag{
						Name:    "repo",
						Aliases: []string{"r"},
						Usage:   "Repository name (if not specified, shows default targets)",
						EnvVars: []string{"REPO"},
					},
					&cli.BoolFlag{
						Name:    "with-fallback",
						Aliases: []string{"f"},
						Usage:   "Fallback to default targets if repo-specific ones don't exist (default: true for repo queries)",
						Value:   false,
					},
					&cli.BoolFlag{
						Name:    "json",
						Aliases: []string{"j"},
						Usage:   "Output as JSON",
					},
				},
				Action: listAction,
			},
			{
				Name:    "config",
				Aliases: []string{"c", "cfg"},
				Usage:   "Manage initial environment configuration",
				Description: `Configure the initial environment for deployments.

The initial environment determines which target environment is used first in the deployment progression.
Default is 'dev' unless configured otherwise.

Examples:
  # Set default initial environment to dev
  aws-deployer targets config --env dev --default --initial-env dev

  # Set repo-specific initial environment to stg
  aws-deployer targets config --env prd --repo my-app --initial-env stg

  # View default initial environment configuration
  aws-deployer targets config --env dev --default

  # View repo-specific initial environment configuration
  aws-deployer targets config --env prd --repo my-app`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Aliases:  []string{"e"},
						Usage:    "AWS Deployer environment (dev, stg, or prd) - determines which DynamoDB table to use",
						Required: true,
						EnvVars:  []string{"ENV"},
					},
					&cli.BoolFlag{
						Name:    "default",
						Aliases: []string{"d"},
						Usage:   "Configure default initial environment (applies to all repos)",
					},
					&cli.StringFlag{
						Name:    "repo",
						Aliases: []string{"r"},
						Usage:   "Repository name (for repo-specific configuration)",
						EnvVars: []string{"REPO"},
					},
					&cli.StringFlag{
						Name:    "initial-env",
						Aliases: []string{"i"},
						Usage:   "Initial environment to set (dev, stg, or prd)",
						EnvVars: []string{"INITIAL_ENV"},
					},
					&cli.BoolFlag{
						Name:    "json",
						Aliases: []string{"j"},
						Usage:   "Output as JSON",
					},
				},
				Action: configAction,
			},
			{
				Name:    "delete",
				Aliases: []string{"del", "rm", "remove"},
				Usage:   "Delete deployment targets",
				Description: `Delete deployment targets for a repo/env or default configuration.

Examples:
  # Delete default targets using dev aws-deployer for dev target environment
  aws-deployer targets delete --env dev --target-env dev --default

  # Delete targets for a specific repo using prd aws-deployer for prd target environment
  aws-deployer targets delete --env prd --target-env prd --repo my-app

  # Delete without confirmation prompt
  aws-deployer targets delete --env dev --target-env dev --repo my-app --force`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "env",
						Aliases:  []string{"e"},
						Usage:    "AWS Deployer environment (dev, stg, or prd) - determines which DynamoDB table to use",
						Required: true,
						EnvVars:  []string{"ENV"},
					},
					&cli.StringFlag{
						Name:     "target-env",
						Aliases:  []string{"t"},
						Usage:    "Target deployment environment (dev, stg, or prd) - the environment being deployed to",
						Required: true,
						EnvVars:  []string{"TARGET_ENV"},
					},
					&cli.BoolFlag{
						Name:    "default",
						Aliases: []string{"d"},
						Usage:   "Delete default targets (applies to all repos)",
					},
					&cli.StringFlag{
						Name:    "repo",
						Aliases: []string{"r"},
						Usage:   "Repository name (for repo-specific targets)",
						EnvVars: []string{"REPO"},
					},
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage:   "Skip confirmation prompt",
					},
				},
				Action: deleteAction,
			},
		},
	}
}

// envRecord represents a target configuration for a specific environment
type envRecord struct {
	env      string
	record   *targetdao.Record
	fallback bool
}

// setAction sets deployment targets
func setAction(c *cli.Context) error {
	logger := zerolog.Ctx(c.Context)

	env := c.String("env")
	targetEnv := c.String("target-env")
	repo := c.String("repo")
	accountsStr := c.String("accounts")
	regionsStr := c.String("regions")
	targetsJSON := c.String("targets-json")
	downstreamEnvStr := c.String("downstream-env")
	overwrite := c.Bool("overwrite")
	isDefault := c.Bool("default")

	if env == "" {
		return fmt.Errorf("--env is required")
	}
	if targetEnv == "" {
		return fmt.Errorf("--target-env is required")
	}

	// Validate that we have either default flag or repo
	if isDefault && repo != "" {
		return fmt.Errorf("cannot specify both --default and --repo")
	}
	if !isDefault && repo == "" {
		return fmt.Errorf("must specify either --default or --repo")
	}

	// If default, use DefaultRepo as the repo
	if isDefault {
		repo = targetdao.DefaultRepo
	}

	// Parse targets from JSON or accounts/regions
	var targets []targetdao.Target
	if targetsJSON != "" {
		if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
			return fmt.Errorf("failed to parse targets JSON: %w", err)
		}
	} else {
		// Parse accounts and regions
		if accountsStr == "" || regionsStr == "" {
			return fmt.Errorf("must provide either --targets-json or both --accounts and --regions")
		}

		accounts := parseCommaSeparated(accountsStr)
		regions := parseCommaSeparated(regionsStr)

		if len(accounts) == 0 {
			return fmt.Errorf("at least one account ID is required")
		}
		if len(regions) == 0 {
			return fmt.Errorf("at least one region is required")
		}

		targets = []targetdao.Target{
			{
				AccountIDs: accounts,
				Regions:    regions,
			},
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}

	// Parse downstream environments
	var downstreamEnv []string
	if downstreamEnvStr != "" {
		downstreamEnv = parseCommaSeparated(downstreamEnvStr)
	}

	// Create DAO
	dao, err := createDAO(env)
	if err != nil {
		return err
	}

	// Check if targets already exist
	id := targetdao.NewID(repo, targetEnv)
	existing, err := dao.Find(c.Context, id)
	if err != nil {
		return fmt.Errorf("failed to check existing targets: %w", err)
	}

	if existing != nil && !overwrite {
		return fmt.Errorf("targets already exist for %s:%s. Use --overwrite to replace them", repo, targetEnv)
	}

	// Create or update the targets
	var record *targetdao.Record
	if existing == nil {
		record, err = dao.Create(c.Context, targetdao.CreateInput{
			Repo:          repo,
			Env:           targetEnv,
			Targets:       targets,
			DownstreamEnv: downstreamEnv,
		})
		if err != nil {
			return fmt.Errorf("failed to create targets: %w", err)
		}
		logger.Info().Msg("Targets created successfully")
	} else {
		record, err = dao.Update(c.Context, targetdao.UpdateInput{
			ID:            id,
			Targets:       targets,
			DownstreamEnv: downstreamEnv,
		})
		if err != nil {
			return fmt.Errorf("failed to update targets: %w", err)
		}
		logger.Info().Msg("Targets updated successfully")
	}

	// Display the targets
	displayTargets(record, isDefault, false)

	return nil
}

// listAction lists deployment targets
func listAction(c *cli.Context) error {
	logger := zerolog.Ctx(c.Context)

	env := c.String("env")
	targetEnv := c.String("target-env")
	repo := c.String("repo")
	showJSON := c.Bool("json")
	withFallback := c.Bool("with-fallback")

	if env == "" {
		return fmt.Errorf("--env is required")
	}

	// If no repo specified, query default targets
	isDefault := repo == ""
	if isDefault {
		repo = targetdao.DefaultRepo
	}

	// Create DAO
	dao, err := createDAO(env)
	if err != nil {
		return err
	}

	// Default to fallback for repo-specific queries (not default queries)
	if !isDefault && !withFallback {
		withFallback = true
	}

	// If no target-env specified, show all environments
	if targetEnv == "" {
		return listAllEnvironments(c.Context, dao, repo, isDefault, showJSON, withFallback, logger)
	}

	// Get the targets for specific target environment
	var record *targetdao.Record
	var usedFallback bool
	if withFallback && !isDefault {
		// Try with fallback to default
		record, err = dao.GetWithDefault(c.Context, repo, targetEnv)
		if err != nil {
			return fmt.Errorf("failed to get targets: %w", err)
		}
		if record != nil && record.PK.String() == targetdao.DefaultRepo {
			usedFallback = true
		}
	} else {
		// Get specific targets only
		id := targetdao.NewID(repo, targetEnv)
		record, err = dao.Find(c.Context, id)
		if err != nil {
			return fmt.Errorf("failed to get targets: %w", err)
		}
	}

	if record == nil {
		if isDefault {
			fmt.Printf("No default targets configured for target environment: %s\n", targetEnv)
		} else {
			fmt.Printf("No targets configured for %s:%s (and no default targets found)\n", repo, targetEnv)
		}
		return nil
	}

	// Display the targets
	if showJSON {
		displayJSON(record)
	} else {
		displayTargets(record, isDefault, usedFallback)
	}

	logger.Info().
		Str("env", env).
		Str("repo", record.PK.String()).
		Bool("used_fallback", usedFallback).
		Msg("Retrieved deployment targets")

	return nil
}

// Remaining functions are identical to deployment-targets/main.go but adapted
// Continue with all helper functions from the original file...

// I'll include the essential helper functions that are referenced above.
// The full file would include all the functions from deployment-targets/main.go

// createDAO creates a targetdao.DAO instance
func createDAO(env string) (*targetdao.DAO, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	dbClient := dynamodb.NewFromConfig(cfg)
	tableName := targetdao.TableName(env)
	return targetdao.New(dbClient, tableName), nil
}

// parseCommaSeparated splits a comma-separated string and trims whitespace
func parseCommaSeparated(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// displayTargets prints the deployment targets in a readable format
func displayTargets(record *targetdao.Record, isDefault, usedFallback bool) {
	fmt.Println()
	if isDefault || record.PK.String() == targetdao.DefaultRepo {
		fmt.Printf("Default deployment targets for target environment: %s\n", record.SK)
		if usedFallback {
			fmt.Println("(Using default targets - no repo-specific targets configured)")
		}
	} else {
		fmt.Printf("Deployment targets for %s (target environment: %s)\n", record.PK, record.SK)
	}
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Show initial environment if configured
	if record.InitialEnv != "" {
		fmt.Printf("Initial Environment: %s\n", record.InitialEnv)
		fmt.Println()
	}

	// Show downstream environments if configured
	if len(record.DownstreamEnv) > 0 {
		fmt.Printf("Downstream: %s → %s\n", record.SK, strings.Join(record.DownstreamEnv, " → "))
		fmt.Println()
	}

	// Show expanded targets
	expanded := targetdao.ExpandTargets(record.Targets)
	fmt.Printf("Total deployments: %d\n", len(expanded))
	fmt.Println()

	// Group by account for display
	accountMap := make(map[string][]string)
	for _, target := range expanded {
		accountMap[target.AccountID] = append(accountMap[target.AccountID], target.Region)
	}

	fmt.Println("Targets by account:")
	for accountID, regions := range accountMap {
		fmt.Printf("  Account: %s\n", accountID)
		fmt.Printf("    Regions: %s\n", strings.Join(regions, ", "))
	}
	fmt.Println()

	// Show raw targets structure
	fmt.Println("Target groups:")
	for i, target := range record.Targets {
		fmt.Printf("  Group %d:\n", i+1)
		fmt.Printf("    Accounts: %s\n", strings.Join(target.AccountIDs, ", "))
		fmt.Printf("    Regions:  %s\n", strings.Join(target.Regions, ", "))
	}
}

// displayJSON prints the targets as JSON
func displayJSON(record *targetdao.Record) {
	output := map[string]interface{}{
		"repo":       record.PK.String(),
		"target_env": record.SK,
		"targets":    record.Targets,
	}
	if record.InitialEnv != "" {
		output["initial_env"] = record.InitialEnv
	}
	if len(record.DownstreamEnv) > 0 {
		output["downstream_env"] = record.DownstreamEnv
	}
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonBytes))
}

// listAllEnvironments lists targets across all environments
func listAllEnvironments(ctx context.Context, dao *targetdao.DAO, repo string, isDefault, showJSON, withFallback bool, logger *zerolog.Logger) error {
	// Fetch config to get initial environment
	config, err := dao.GetConfig(ctx, repo)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to get config")
	}
	if (config == nil || config.InitialEnv == "") && !isDefault {
		// Fall back to default config if no repo-specific config exists
		defaultConfig, err := dao.GetConfig(ctx, targetdao.DefaultRepo)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to get default config")
		}
		if defaultConfig != nil && defaultConfig.InitialEnv != "" {
			config = defaultConfig
		}
	}

	// Determine initial environment (default to "dev")
	initialEnv := "dev"
	if config != nil && config.InitialEnv != "" {
		initialEnv = config.InitialEnv
	}

	// Build a map of all environment records for quick lookup
	allEnvs := map[string]*targetdao.Record{}
	for _, targetEnv := range []string{"dev", "stg", "prd"} {
		var record *targetdao.Record
		var err error

		if withFallback && !isDefault {
			record, err = dao.GetWithDefault(ctx, repo, targetEnv)
			if err != nil {
				logger.Warn().Err(err).Str("target_env", targetEnv).Msg("Failed to get targets")
				continue
			}
		} else {
			id := targetdao.NewID(repo, targetEnv)
			record, err = dao.Find(ctx, id)
			if err != nil {
				logger.Warn().Err(err).Str("target_env", targetEnv).Msg("Failed to get targets")
				continue
			}
		}

		if record != nil {
			allEnvs[targetEnv] = record
		}
	}

	// Build the deployment progression by following the chain
	var records []envRecord
	visited := make(map[string]bool)
	currentEnv := initialEnv

	for {
		if visited[currentEnv] {
			logger.Warn().Str("env", currentEnv).Msg("Circular reference detected in downstream environments")
			break
		}

		record, exists := allEnvs[currentEnv]
		if !exists {
			break
		}

		visited[currentEnv] = true
		usedFallback := record.PK.String() == "$"

		records = append(records, envRecord{
			env:      currentEnv,
			record:   record,
			fallback: usedFallback,
		})

		if len(record.DownstreamEnv) == 0 {
			break
		}
		currentEnv = record.DownstreamEnv[0]
	}

	if len(records) == 0 {
		if isDefault {
			fmt.Println("No default targets configured for any environment")
		} else {
			fmt.Printf("No targets configured for %s in any environment (and no default targets found)\n", repo)
		}
		return nil
	}

	if showJSON {
		displayMultipleJSON(records, config)
	} else {
		displayMultipleTargets(records, isDefault, config)
	}

	logger.Info().
		Str("repo", repo).
		Int("env_count", len(records)).
		Msg("Retrieved deployment targets for all environments")

	return nil
}

// displayMultipleTargets displays targets across multiple environments
func displayMultipleTargets(records []envRecord, isDefault bool, config *targetdao.Record) {
	fmt.Println()
	if isDefault {
		fmt.Println("Default deployment progression")
	} else {
		fmt.Printf("Deployment progression for %s\n", records[0].record.PK)
	}
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// Show deployment progression overview
	if len(records) > 0 {
		envNames := make([]string, len(records))
		for i, rec := range records {
			envNames[i] = rec.env
		}
		fmt.Printf("Progression: %s\n", strings.Join(envNames, " → "))
		fmt.Println()
	}

	for i, rec := range records {
		if i > 0 {
			fmt.Println()
			fmt.Println(strings.Repeat("-", 80))
			fmt.Println()
		}

		fmt.Printf("Step %d: %s", i+1, rec.env)
		if i == 0 {
			fmt.Print(" (initial environment)")
		}
		if rec.fallback {
			fmt.Print(" [using default targets]")
		}
		fmt.Println()

		if len(rec.record.DownstreamEnv) > 0 {
			fmt.Printf("Next: %s\n", strings.Join(rec.record.DownstreamEnv, " → "))
		}
		fmt.Println()

		expanded := targetdao.ExpandTargets(rec.record.Targets)
		fmt.Printf("Total deployments: %d\n", len(expanded))

		accountMap := make(map[string][]string)
		for _, target := range expanded {
			accountMap[target.AccountID] = append(accountMap[target.AccountID], target.Region)
		}

		fmt.Println("Targets by account:")
		for accountID, regions := range accountMap {
			fmt.Printf("  Account: %s\n", accountID)
			fmt.Printf("    Regions: %s\n", strings.Join(regions, ", "))
		}
	}

	fmt.Println()
}

// displayMultipleJSON displays multiple environment targets as JSON
func displayMultipleJSON(records []envRecord, config *targetdao.Record) {
	output := make(map[string]interface{})

	if len(records) > 0 {
		output["repo"] = records[0].record.PK.String()
	}

	if len(records) > 0 {
		progression := make([]string, len(records))
		for i, rec := range records {
			progression[i] = rec.env
		}
		output["progression"] = progression
		output["initial_env"] = progression[0]
	}

	steps := make([]map[string]interface{}, len(records))
	for i, rec := range records {
		step := map[string]interface{}{
			"step":          i + 1,
			"env":           rec.env,
			"targets":       rec.record.Targets,
			"using_default": rec.fallback,
		}
		if i == 0 {
			step["is_initial"] = true
		}
		if len(rec.record.DownstreamEnv) > 0 {
			step["next"] = rec.record.DownstreamEnv
		}
		steps[i] = step
	}
	output["steps"] = steps

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonBytes))
}

// configAction manages the initial environment configuration
func configAction(c *cli.Context) error {
	logger := zerolog.Ctx(c.Context)

	env := c.String("env")
	repo := c.String("repo")
	initialEnv := c.String("initial-env")
	isDefault := c.Bool("default")
	showJSON := c.Bool("json")

	if env == "" {
		return fmt.Errorf("--env is required")
	}

	if isDefault && repo != "" {
		return fmt.Errorf("cannot specify both --default and --repo")
	}
	if !isDefault && repo == "" {
		return fmt.Errorf("must specify either --default or --repo")
	}

	if isDefault {
		repo = targetdao.DefaultRepo
	}

	dao, err := createDAO(env)
	if err != nil {
		return err
	}

	if initialEnv != "" {
		record, err := dao.SetConfig(c.Context, repo, initialEnv)
		if err != nil {
			return fmt.Errorf("failed to set initial environment: %w", err)
		}

		logger.Info().
			Str("repo", repo).
			Str("initial_env", initialEnv).
			Msg("Initial environment configured")

		fmt.Println()
		if isDefault {
			fmt.Printf("✓ Default initial environment set to: %s\n", initialEnv)
		} else {
			fmt.Printf("✓ Initial environment for %s set to: %s\n", repo, initialEnv)
		}
		fmt.Println()

		if showJSON {
			displayJSON(record)
		}

		return nil
	}

	config, err := dao.GetConfig(c.Context, repo)
	if err != nil {
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	if config == nil || config.InitialEnv == "" {
		if isDefault {
			fmt.Println("No default initial environment configured (will use 'dev')")
		} else {
			fmt.Printf("No initial environment configured for %s (will use default or 'dev')\n", repo)
		}
		return nil
	}

	fmt.Println()
	if isDefault {
		fmt.Printf("Default initial environment: %s\n", config.InitialEnv)
	} else {
		fmt.Printf("Initial environment for %s: %s\n", repo, config.InitialEnv)
	}
	fmt.Println()

	if showJSON {
		displayJSON(config)
	}

	return nil
}

// deleteAction deletes deployment targets
func deleteAction(c *cli.Context) error {
	logger := zerolog.Ctx(c.Context)

	env := c.String("env")
	targetEnv := c.String("target-env")
	repo := c.String("repo")
	isDefault := c.Bool("default")
	force := c.Bool("force")

	if env == "" {
		return fmt.Errorf("--env is required")
	}
	if targetEnv == "" {
		return fmt.Errorf("--target-env is required")
	}

	if isDefault && repo != "" {
		return fmt.Errorf("cannot specify both --default and --repo")
	}
	if !isDefault && repo == "" {
		return fmt.Errorf("must specify either --default or --repo")
	}

	if isDefault {
		repo = targetdao.DefaultRepo
	}

	dao, err := createDAO(env)
	if err != nil {
		return err
	}

	id := targetdao.NewID(repo, targetEnv)
	existing, err := dao.Find(c.Context, id)
	if err != nil {
		return fmt.Errorf("failed to check existing targets: %w", err)
	}

	if existing == nil {
		if isDefault {
			fmt.Printf("No default targets found for target environment: %s\n", targetEnv)
		} else {
			fmt.Printf("No targets found for %s:%s\n", repo, targetEnv)
		}
		return nil
	}

	fmt.Println()
	if isDefault {
		fmt.Printf("About to delete default targets for target environment: %s\n", targetEnv)
	} else {
		fmt.Printf("About to delete targets for %s:%s\n", repo, targetEnv)
	}
	fmt.Println()

	expanded := targetdao.ExpandTargets(existing.Targets)
	fmt.Printf("This will remove %d deployment target(s)\n", len(expanded))

	if !force {
		fmt.Print("\nAre you sure? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "yes" && response != "y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	err = dao.Delete(c.Context, id)
	if err != nil {
		return fmt.Errorf("failed to delete targets: %w", err)
	}

	logger.Info().
		Str("env", env).
		Str("repo", repo).
		Msg("Targets deleted successfully")

	fmt.Println("\n✓ Targets deleted successfully")

	return nil
}
