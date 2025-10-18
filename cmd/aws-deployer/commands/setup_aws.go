package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/constants"
	"github.com/urfave/cli/v2"
)

type awsHandler struct {
	iamClient *iam.Client
	stsClient *sts.Client
}

func newAWSHandler(ctx context.Context, region string) (*awsHandler, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &awsHandler{
		iamClient: iam.NewFromConfig(cfg),
		stsClient: sts.NewFromConfig(cfg),
	}, nil
}

// SetupAWSCommand returns the aws command for configuring multi-account AWS deployments
func SetupAWSCommand(logger *zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:    "aws",
		Usage:   "Setup AWS accounts for multi-account CloudFormation deployments",
		Description: `Configure IAM roles for multi-account CloudFormation StackSet deployments.

This command group sets up the necessary IAM roles in your deployer account and target accounts
to enable cross-account CloudFormation StackSet deployments.`,
		Subcommands: []*cli.Command{
			{
				Name:  "setup-deployer",
				Usage: "Create administration and execution roles in deployer account",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "deployer-account",
						Usage:    "Deployer AWS account ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "region",
						Usage: "AWS region",
						Value: "us-east-1",
					},
					&cli.StringFlag{
						Name:  "execution-role-name",
						Usage: "Execution role name",
						Value: constants.ExecutionRoleName,
					},
					&cli.StringFlag{
						Name:  "admin-role-name",
						Usage: "Administration role name",
						Value: constants.AdministrationRoleName,
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be created without creating it",
					},
				},
				Action: func(c *cli.Context) error {
					ctx := c.Context
					handler, err := newAWSHandler(ctx, c.String("region"))
					if err != nil {
						return err
					}

					return handler.setupDeployerAccount(
						ctx,
						c.String("deployer-account"),
						c.String("execution-role-name"),
						c.String("admin-role-name"),
						c.Bool("dry-run"),
					)
				},
			},
			{
				Name:  "setup-target",
				Usage: "Create StackSet execution role in target account",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "deployer-account",
						Usage:    "Deployer AWS account ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "region",
						Usage: "AWS region",
						Value: "us-east-1",
					},
					&cli.StringFlag{
						Name:  "execution-role-name",
						Usage: "Execution role name",
						Value: constants.ExecutionRoleName,
					},
					&cli.StringFlag{
						Name:  "admin-role-name",
						Usage: "Administration role name in deployer account",
						Value: constants.AdministrationRoleName,
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be created without creating it",
					},
				},
				Action: func(c *cli.Context) error {
					ctx := c.Context
					handler, err := newAWSHandler(ctx, c.String("region"))
					if err != nil {
						return err
					}

					return handler.setupTargetAccount(
						ctx,
						c.String("deployer-account"),
						c.String("execution-role-name"),
						c.String("admin-role-name"),
						c.Bool("dry-run"),
					)
				},
			},
			{
				Name:  "verify",
				Usage: "Verify cross-account access is configured correctly",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "deployer-account",
						Usage:    "Deployer AWS account ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "target-account",
						Usage:    "Target AWS account ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "region",
						Usage: "AWS region",
						Value: "us-east-1",
					},
					&cli.StringFlag{
						Name:  "role-name",
						Usage: "Execution role name",
						Value: constants.ExecutionRoleName,
					},
				},
				Action: func(c *cli.Context) error {
					ctx := c.Context
					handler, err := newAWSHandler(ctx, c.String("region"))
					if err != nil {
						return err
					}

					return handler.verifySetup(
						ctx,
						c.String("deployer-account"),
						c.String("target-account"),
						c.String("role-name"),
					)
				},
			},
			{
				Name:  "teardown",
				Usage: "Remove StackSet execution role from target account",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "target-account",
						Usage: "Target AWS account ID (defaults to current account)",
					},
					&cli.StringFlag{
						Name:  "region",
						Usage: "AWS region",
						Value: "us-east-1",
					},
					&cli.StringFlag{
						Name:  "role-name",
						Usage: "Execution role name",
						Value: constants.ExecutionRoleName,
					},
				},
				Action: func(c *cli.Context) error {
					ctx := c.Context
					handler, err := newAWSHandler(ctx, c.String("region"))
					if err != nil {
						return err
					}

					return handler.teardown(
						ctx,
						c.String("role-name"),
					)
				},
			},
		},
	}
}

// getAdminRoleTrustPolicy creates the trust policy for the administration role
func getAdminRoleTrustPolicy() string {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Service": "cloudformation.amazonaws.com",
				},
				"Action": "sts:AssumeRole",
			},
		},
	}

	policyJSON, _ := json.Marshal(policy)
	return string(policyJSON)
}

// getAdminRolePolicy creates the permissions policy for the administration role
func getAdminRolePolicy() string {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect":   "Allow",
				"Action":   "sts:AssumeRole",
				"Resource": "arn:aws:iam::*:role/AWSCloudFormationStackSetExecutionRole",
			},
		},
	}

	policyJSON, _ := json.Marshal(policy)
	return string(policyJSON)
}

// getTrustPolicy creates the trust policy document for the execution role
func getTrustPolicy(deployerAccountID, adminRoleName string) string {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:role/%s", deployerAccountID, adminRoleName),
				},
				"Action": "sts:AssumeRole",
			},
		},
	}

	policyJSON, _ := json.Marshal(policy)
	return string(policyJSON)
}

// waitForRole waits for a role to be available (propagated across AWS)
func (h *awsHandler) waitForRole(ctx context.Context, roleName string, maxWait time.Duration) error {
	fmt.Printf("Waiting for role %s to propagate...\n", roleName)

	deadline := time.Now().Add(maxWait)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		_, err := h.iamClient.GetRole(ctx, &iam.GetRoleInput{
			RoleName: aws.String(roleName),
		})
		if err == nil {
			fmt.Printf("Role is available after %d attempts\n", attempt)
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("role %s did not become available within %v", roleName, maxWait)
}

// ensureAdminRole creates the administration role in the deployer account if it doesn't exist
func (h *awsHandler) ensureAdminRole(ctx context.Context, deployerAccountID, adminRoleName, executionRoleName string, dryRun bool) error {
	fmt.Printf("Ensuring administration role exists in deployer account %s...\n", deployerAccountID)

	if dryRun {
		fmt.Printf("DRY RUN: Would ensure admin role in account %s:\n", deployerAccountID)
		fmt.Printf("Role Name: %s\n", adminRoleName)
		fmt.Printf("Trust Policy:\n%s\n", prettyJSON(getAdminRoleTrustPolicy()))
		fmt.Printf("Permissions Policy:\n%s\n", prettyJSON(getAdminRolePolicy()))
		return nil
	}

	// Check if role already exists
	_, err := h.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(adminRoleName),
	})

	if err == nil {
		fmt.Printf("Administration role %s already exists in deployer account\n", adminRoleName)
		return nil
	}

	// Role doesn't exist, create it
	fmt.Printf("Creating administration role %s in deployer account...\n", adminRoleName)

	trustPolicy := getAdminRoleTrustPolicy()
	_, err = h.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(adminRoleName),
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		Description:              aws.String("Administration role for CloudFormation StackSets"),
		Tags: []iamtypes.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("aws-deployer"),
			},
			{
				Key:   aws.String("Purpose"),
				Value: aws.String("StackSetAdministration"),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create administration role: %w", err)
	}
	fmt.Printf("Created administration role: %s\n", adminRoleName)

	// Create and attach inline policy for assuming execution roles
	policyName := "AssumeRole-StackSetExecution"
	permissionsPolicy := getAdminRolePolicy()

	_, err = h.iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(adminRoleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(permissionsPolicy),
	})
	if err != nil {
		return fmt.Errorf("failed to attach permissions policy: %w", err)
	}
	fmt.Printf("Attached permissions policy: %s\n", policyName)

	// Wait for the role to propagate before proceeding
	if err := h.waitForRole(ctx, adminRoleName, 30*time.Second); err != nil {
		return err
	}

	return nil
}

// setupDeployerAccount sets up the administration role and execution role in the deployer account
func (h *awsHandler) setupDeployerAccount(ctx context.Context, deployerAccountID, executionRoleName, adminRoleName string, dryRun bool) error {
	// Get current account ID
	identity, err := h.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}
	currentAccountID := *identity.Account

	if currentAccountID != deployerAccountID {
		return fmt.Errorf("must run this command from deployer account %s (currently in account %s)", deployerAccountID, currentAccountID)
	}

	// Create the administration role
	if err := h.ensureAdminRole(ctx, deployerAccountID, adminRoleName, executionRoleName, dryRun); err != nil {
		return fmt.Errorf("failed to ensure admin role: %w", err)
	}

	fmt.Println()

	// Create the execution role in the deployer account
	return h.setupExecutionRole(ctx, deployerAccountID, deployerAccountID, executionRoleName, adminRoleName, dryRun)
}

// setupTargetAccount creates the StackSet execution role in a target account
func (h *awsHandler) setupTargetAccount(ctx context.Context, deployerAccountID, executionRoleName, adminRoleName string, dryRun bool) error {
	// Get current account ID
	identity, err := h.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}
	targetAccountID := *identity.Account

	if targetAccountID == deployerAccountID {
		return fmt.Errorf("use setup-deployer command for deployer account (currently in account %s)", targetAccountID)
	}

	return h.setupExecutionRole(ctx, deployerAccountID, targetAccountID, executionRoleName, adminRoleName, dryRun)
}

// setupExecutionRole creates the execution role in the specified account
func (h *awsHandler) setupExecutionRole(ctx context.Context, deployerAccountID, targetAccountID, roleName, adminRoleName string, dryRun bool) error {
	trustPolicy := getTrustPolicy(deployerAccountID, adminRoleName)

	if dryRun {
		fmt.Printf("DRY RUN: Would create role in account %s:\n", targetAccountID)
		fmt.Printf("Role Name: %s\n", roleName)
		fmt.Printf("Trust Policy:\n%s\n", prettyJSON(trustPolicy))
		fmt.Printf("Managed Policy: arn:aws:iam::aws:policy/AdministratorAccess\n")
		return nil
	}

	fmt.Printf("Creating execution role in account %s...\n", targetAccountID)

	// Check if role already exists
	_, err := h.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err == nil {
		fmt.Printf("Role %s already exists. Updating trust policy...\n", roleName)
		// Update assume role policy
		_, err = h.iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
			RoleName:       aws.String(roleName),
			PolicyDocument: aws.String(trustPolicy),
		})
		if err != nil {
			return fmt.Errorf("failed to update trust policy: %w", err)
		}
	} else {
		// Create role
		_, err = h.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(trustPolicy),
			Description:              aws.String("Execution role for CloudFormation StackSets from deployer account"),
			Tags: []iamtypes.Tag{
				{
					Key:   aws.String("ManagedBy"),
					Value: aws.String("aws-deployer"),
				},
				{
					Key:   aws.String("Purpose"),
					Value: aws.String("StackSetExecution"),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}
		fmt.Printf("Created role: %s\n", roleName)
	}

	// Attach AdministratorAccess policy
	fmt.Printf("Attaching AdministratorAccess policy...\n")
	_, err = h.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AdministratorAccess"),
	})
	if err != nil {
		// Check if already attached
		if contains(err.Error(), "EntityAlreadyExists") || contains(err.Error(), "already attached") {
			fmt.Printf("AdministratorAccess policy already attached\n")
		} else {
			return fmt.Errorf("failed to attach policy: %w", err)
		}
	} else {
		fmt.Printf("Attached AdministratorAccess policy\n")
	}

	fmt.Printf("\n✓ Setup complete for account %s\n", targetAccountID)
	fmt.Printf("  Role ARN: arn:aws:iam::%s:role/%s\n", targetAccountID, roleName)
	fmt.Printf("  Trusted by: Deployer account %s\n", deployerAccountID)
	return nil
}

// verifySetup tests that the cross-account role works
func (h *awsHandler) verifySetup(ctx context.Context, deployerAccountID, targetAccountID, roleName string) error {
	// Get current account
	identity, err := h.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}
	currentAccount := *identity.Account

	if currentAccount != deployerAccountID {
		return fmt.Errorf("verification must run with deployer account credentials (current: %s, expected: %s)", currentAccount, deployerAccountID)
	}

	// Check if role exists in target account
	fmt.Printf("Verifying role setup...\n")
	fmt.Printf("  Deployer account: %s\n", deployerAccountID)
	fmt.Printf("  Target account: %s\n", targetAccountID)
	fmt.Printf("  Role: %s\n", roleName)

	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", targetAccountID, roleName)

	// Try to assume the role
	_, err = h.stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String("aws-deployer-verification"),
		DurationSeconds: aws.Int32(900), // 15 minutes
	})
	if err != nil {
		return fmt.Errorf("failed to assume role %s: %w\n\nPossible issues:\n  1. Role doesn't exist in target account\n  2. Trust policy not configured correctly\n  3. Deployer account lacks sts:AssumeRole permission", roleArn, err)
	}

	fmt.Printf("\n✓ Verification successful\n")
	fmt.Printf("  Deployer account %s can assume role in target account %s\n", deployerAccountID, targetAccountID)
	return nil
}

// teardown removes the StackSet execution role
func (h *awsHandler) teardown(ctx context.Context, roleName string) error {
	// Get current account ID
	identity, err := h.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}
	targetAccountID := *identity.Account

	fmt.Printf("Removing role from account %s...\n", targetAccountID)

	// Detach AdministratorAccess policy
	fmt.Printf("Detaching AdministratorAccess policy...\n")
	_, err = h.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String("arn:aws:iam::aws:policy/AdministratorAccess"),
	})
	if err != nil {
		if contains(err.Error(), "NoSuchEntity") {
			fmt.Printf("Policy not attached (already removed)\n")
		} else {
			return fmt.Errorf("failed to detach policy: %w", err)
		}
	}

	// Delete role
	fmt.Printf("Deleting role...\n")
	_, err = h.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		if contains(err.Error(), "NoSuchEntity") {
			fmt.Printf("Role not found (already deleted)\n")
			return nil
		}
		return fmt.Errorf("failed to delete role: %w", err)
	}

	fmt.Printf("\n✓ Teardown complete for account %s\n", targetAccountID)
	return nil
}

func prettyJSON(s string) string {
	var obj interface{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return s
	}
	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return s
	}
	return string(pretty)
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
