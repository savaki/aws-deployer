package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/smithy-go"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/dao/lockdao"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
	"github.com/urfave/cli/v2"
)

// SyncCommand returns the sync command for cleaning up orphaned repos
func SyncCommand(logger *zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Clean up repos with no associated CloudFormation StackSets",
		Description: `Discovers repos that have no CloudFormation StackSets and removes their
data from all aws-deployer tables.

A repo is considered orphaned if it has no StackSets in any environment (dev, stg, prd).
StackSets are named: {env}-{repo}

Deletion order (for repeatability if interrupted):
  1. lockdao - ephemeral lock records
  2. deploymentdao - per-account/region deployment history
  3. builddao - build records and "latest" magic records
  4. targetdao - deployment target configuration (deleted last so sync can be re-run)

Examples:
  # Dry run - show what would be deleted (default)
  aws-deployer sync --env dev

  # Check specific repos only
  aws-deployer sync --env dev --repo my-app --repo other-app

  # Execute deletion
  aws-deployer sync --env dev --execute

  # Delete specific repos without StackSets
  aws-deployer sync --env dev --repo my-app --execute

  # Skip confirmation prompt
  aws-deployer sync --env dev --execute --force`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "env",
				Aliases:  []string{"e"},
				Usage:    "AWS Deployer environment (dev, stg, or prd) - determines which DynamoDB tables to use",
				Required: true,
				EnvVars:  []string{"ENV"},
			},
			&cli.StringSliceFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Specific repo(s) to check/delete (can be specified multiple times). If not specified, checks all repos.",
			},
			&cli.BoolFlag{
				Name:    "execute",
				Aliases: []string{"x"},
				Usage:   "Actually perform deletions (default is dry-run)",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Skip confirmation prompt",
			},
			&cli.IntFlag{
				Name:    "concurrency",
				Aliases: []string{"c"},
				Usage:   "Max concurrent StackSet queries",
				Value:   3,
			},
		},
		Action: func(c *cli.Context) error {
			return syncAction(c, logger)
		},
	}
}

// orphanedRepo represents a repo that has no StackSets
type orphanedRepo struct {
	name string
}

func syncAction(c *cli.Context, logger *zerolog.Logger) error {
	ctx := c.Context
	env := c.String("env")
	specifiedRepos := c.StringSlice("repo")
	execute := c.Bool("execute")
	force := c.Bool("force")
	concurrency := c.Int("concurrency")

	if env == "" {
		return fmt.Errorf("--env is required")
	}

	if concurrency < 1 {
		concurrency = 3
	}

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create clients and DAOs
	dbClient := dynamodb.NewFromConfig(cfg)
	cfnClient := cloudformation.NewFromConfig(cfg)

	targetDAO := targetdao.New(dbClient, targetdao.TableName(env))
	buildDAO := builddao.New(dbClient, builddao.TableName(env))
	deploymentDAO := deploymentdao.New(dbClient, deploymentdao.TableName(env))
	lockDAO := lockdao.New(dbClient, lockdao.TableName(env))

	// Get target records for later deletion
	targetRecords, err := targetDAO.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to query targets: %w", err)
	}

	var repos []string
	envs := []string{"dev", "stg", "prd"}

	// If specific repos are provided, use those; otherwise discover all repos
	if len(specifiedRepos) > 0 {
		repos = specifiedRepos
		logger.Info().Int("count", len(repos)).Msg("Using specified repos")
	} else {
		// Step 1: Discover all repos from both targetdao and builddao
		logger.Info().Msg("Discovering repos...")

		repoSet := make(map[string]bool)

		// Get repos from targetdao (repos with explicit target configurations)
		for _, record := range targetRecords {
			repo := record.PK.String()
			if repo != targetdao.DefaultRepo {
				repoSet[repo] = true
			}
		}
		logger.Info().Int("count", len(repoSet)).Msg("Found repos from targets table")

		// Get repos from builddao by querying latest records for each environment
		// Latest records have pk=latest/{env} and sk={repo}/{env}
		for _, targetEnv := range envs {
			latestPK := builddao.NewPK("latest", targetEnv)
			latestRecords, err := buildDAO.Query(ctx, latestPK)
			if err != nil {
				logger.Warn().Err(err).Str("env", targetEnv).Msg("Failed to query latest builds")
				continue
			}
			logger.Debug().Int("count", len(latestRecords)).Str("env", targetEnv).Msg("Found latest records")
			for _, record := range latestRecords {
				// Try Repo field first, then parse from SK (which is {repo}/{env})
				repo := record.Repo
				if repo == "" {
					// SK format is {repo}/{env}, parse out the repo
					repo, _, _ = builddao.ParsePK(builddao.PK(record.SK))
				}
				if repo != "" && repo != "latest" {
					repoSet[repo] = true
				}
			}
		}
		logger.Info().Int("count", len(repoSet)).Msg("Found total unique repos")

		repos = make([]string, 0, len(repoSet))
		for repo := range repoSet {
			repos = append(repos, repo)
		}
	}

	if len(repos) == 0 {
		fmt.Println("No repos found to check")
		return nil
	}

	// Step 2: Check which repos have no StackSets
	orphaned := findOrphanedRepos(ctx, cfnClient, repos, concurrency, logger)

	if len(orphaned) == 0 {
		fmt.Println("\nNo orphaned repos found. All repos have at least one StackSet.")
		return nil
	}

	// Display orphaned repos
	fmt.Println()
	fmt.Printf("Found %d orphaned repo(s) with no StackSets:\n", len(orphaned))
	for _, o := range orphaned {
		fmt.Printf("  - %s\n", o.name)
	}
	fmt.Println()

	if !execute {
		fmt.Println("DRY RUN: No data was deleted. Use --execute to actually delete.")
		return nil
	}

	// Confirmation prompt
	if !force {
		fmt.Printf("About to delete all data for %d repo(s) from all tables.\n", len(orphaned))
		fmt.Print("Are you sure? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "yes" && response != "y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Step 3: Delete data for orphaned repos
	for _, o := range orphaned {
		logger.Info().Str("repo", o.name).Msg("Deleting data for orphaned repo")

		// Delete in reverse order for repeatability:
		// 1. lockdao
		// 2. deploymentdao
		// 3. builddao
		// 4. targetdao

		// Delete locks
		for _, targetEnv := range envs {
			lockID := lockdao.NewID(targetEnv, o.name)
			if err := lockDAO.Delete(ctx, lockID); err != nil {
				logger.Warn().Err(err).Str("repo", o.name).Str("env", targetEnv).Msg("Failed to delete lock")
			}
		}

		// Delete deployments
		for _, targetEnv := range envs {
			deployments, err := deploymentDAO.QueryByPK(ctx, targetEnv, o.name)
			if err != nil {
				logger.Warn().Err(err).Str("repo", o.name).Str("env", targetEnv).Msg("Failed to query deployments")
				continue
			}
			for _, d := range deployments {
				if err := deploymentDAO.Delete(ctx, d.GetID()); err != nil {
					logger.Warn().Err(err).Str("id", d.GetID().String()).Msg("Failed to delete deployment")
				}
			}
		}

		// Delete builds and latest records
		for _, targetEnv := range envs {
			// Delete regular builds
			pk := builddao.NewPK(o.name, targetEnv)
			builds, err := buildDAO.Query(ctx, pk)
			if err != nil {
				logger.Warn().Err(err).Str("repo", o.name).Str("env", targetEnv).Msg("Failed to query builds")
				continue
			}
			for _, b := range builds {
				if err := buildDAO.Delete(ctx, b.GetID()); err != nil {
					logger.Warn().Err(err).Str("id", b.GetID().String()).Msg("Failed to delete build")
				}
			}

			// Delete latest magic record (pk=latest/{env}, sk={repo}/{env})
			latestID := builddao.NewID(builddao.NewPK("latest", targetEnv), pk.String())
			if err := buildDAO.Delete(ctx, latestID); err != nil {
				// This may fail if no latest record exists, which is fine
				logger.Debug().Err(err).Str("id", latestID.String()).Msg("Failed to delete latest record (may not exist)")
			}
		}

		// Delete target records (all envs)
		for _, record := range targetRecords {
			if record.PK.String() == o.name {
				if err := targetDAO.Delete(ctx, record.GetID()); err != nil {
					logger.Warn().Err(err).Str("id", record.GetID().String()).Msg("Failed to delete target")
				}
			}
		}

		fmt.Printf("✓ Deleted data for %s\n", o.name)
	}

	fmt.Printf("\n✓ Successfully cleaned up %d orphaned repo(s)\n", len(orphaned))
	return nil
}

// findOrphanedRepos checks which repos have no StackSets across all environments
func findOrphanedRepos(ctx context.Context, cfnClient *cloudformation.Client, repos []string, concurrency int, logger *zerolog.Logger) []orphanedRepo {
	envs := []string{"dev", "stg", "prd"}

	// Track repos that have at least one StackSet
	hasStackSet := make(map[string]bool)

	// Process sequentially with delay to avoid rate limiting
	// CloudFormation DescribeStackSet has strict rate limits (~5 TPS)
	requestDelay := 300 * time.Millisecond

	total := len(repos) * len(envs)
	checked := 0

	for _, repo := range repos {
		// If we already know this repo has a StackSet, skip remaining env checks
		if hasStackSet[repo] {
			checked += len(envs)
			continue
		}

		for _, env := range envs {
			// Skip if we already found a StackSet for this repo
			if hasStackSet[repo] {
				checked++
				continue
			}

			stackSetName := fmt.Sprintf("%s-%s", env, repo)
			exists := checkStackSetExists(ctx, cfnClient, stackSetName, logger)

			if exists {
				hasStackSet[repo] = true
			}

			checked++
			if checked%10 == 0 {
				logger.Info().Int("checked", checked).Int("total", total).Msg("Progress")
			}

			// Delay between requests
			time.Sleep(requestDelay)
		}
	}

	// Build list of orphaned repos
	var orphaned []orphanedRepo
	for _, repo := range repos {
		if !hasStackSet[repo] {
			orphaned = append(orphaned, orphanedRepo{name: repo})
		}
	}

	return orphaned
}

// checkStackSetExists checks if a CloudFormation StackSet exists with retry and backoff
func checkStackSetExists(ctx context.Context, cfnClient *cloudformation.Client, stackSetName string, logger *zerolog.Logger) bool {
	maxRetries := 5
	baseDelay := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}
			logger.Debug().Str("stackset", stackSetName).Int("attempt", attempt+1).Dur("delay", delay).Msg("Retrying after throttle")
			time.Sleep(delay)
		}

		_, err := cfnClient.DescribeStackSet(ctx, &cloudformation.DescribeStackSetInput{
			StackSetName: &stackSetName,
		})

		if err != nil {
			// Check for StackSetNotFoundException - definitive "does not exist"
			var notFoundErr *types.StackSetNotFoundException
			if errors.As(err, &notFoundErr) {
				logger.Debug().Str("stackset", stackSetName).Msg("StackSet does not exist")
				return false
			}

			// Check for throttling error - retry with backoff
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) && apiErr.ErrorCode() == "Throttling" {
				if attempt < maxRetries-1 {
					continue // Retry
				}
				// Final attempt failed due to throttling - assume it might exist to be safe
				logger.Warn().Err(err).Str("stackset", stackSetName).Msg("Throttled after max retries, assuming StackSet may exist")
				return true
			}

			// Other errors - log and assume it might exist
			logger.Warn().Err(err).Str("stackset", stackSetName).Msg("Error checking StackSet, assuming it may exist")
			return true
		}

		logger.Debug().Str("stackset", stackSetName).Msg("StackSet exists")
		return true
	}

	return true // Should not reach here, but be safe
}
