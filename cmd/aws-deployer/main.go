package main

import (
	"context"
	"os"

	"github.com/savaki/aws-deployer/cmd/aws-deployer/commands"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/urfave/cli/v2"
)

func main() {
	logger := di.ProvideLogger()
	ctx := logger.WithContext(context.Background())

	app := &cli.App{
		Name:  "aws-deployer",
		Usage: "AWS deployment automation toolkit",
		Description: `A unified CLI tool for managing AWS deployments with multi-account support.

This tool provides commands for:
  - Setting up AWS accounts for multi-account deployments
  - Configuring GitHub repositories with OIDC authentication
  - Managing deployment targets across accounts and regions`,
		Commands: []*cli.Command{
			commands.SetupAWSCommand(&logger),
			commands.SetupGitHubCommand(&logger),
			commands.SetupECRCommand(&logger),
			commands.SetupSigningCommand(&logger),
			commands.TargetsCommand(&logger),
			commands.SyncCommand(&logger),
		},
	}

	if err := app.RunContext(ctx, os.Args); err != nil {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
}
