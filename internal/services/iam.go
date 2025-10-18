package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type IAMService struct {
	client    *iam.Client
	stsClient *sts.Client
}

type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
}

func NewIAMService() (*IAMService, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &IAMService{
		client:    iam.NewFromConfig(cfg),
		stsClient: sts.NewFromConfig(cfg),
	}, nil
}

const (
	GitHubOIDCProviderURL = "token.actions.githubusercontent.com"
	GitHubOIDCAudience    = "sts.amazonaws.com"
)

// GetAWSAccountID retrieves the AWS account ID
func (s *IAMService) GetAWSAccountID(ctx context.Context) (string, error) {
	result, err := s.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}

	if result.Account == nil {
		return "", fmt.Errorf("account ID is nil")
	}

	return *result.Account, nil
}

// GetOrCreateGitHubOIDCProvider ensures GitHub OIDC provider exists and returns its ARN
func (s *IAMService) GetOrCreateGitHubOIDCProvider(ctx context.Context) (string, error) {
	// Get AWS account ID for constructing ARN
	accountID, err := s.GetAWSAccountID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get AWS account ID: %w", err)
	}

	providerARN := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, GitHubOIDCProviderURL)

	// Check if provider already exists
	_, err = s.client.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
	})

	if err == nil {
		// Provider exists
		return providerARN, nil
	}

	// Check if it's a "not found" error
	var noSuchEntity *types.NoSuchEntityException
	if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
		return "", fmt.Errorf("failed to check OIDC provider: %w", err)
	}

	// Create the OIDC provider
	_, err = s.client.CreateOpenIDConnectProvider(ctx, &iam.CreateOpenIDConnectProviderInput{
		Url: aws.String("https://" + GitHubOIDCProviderURL),
		ClientIDList: []string{
			GitHubOIDCAudience,
		},
		// No thumbprint needed - AWS handles this automatically for GitHub
		ThumbprintList: []string{"6938fd4d98bab03faadb97b34396831e3780aea1"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	_ = noSuchEntity // silence unused variable warning

	return providerARN, nil
}

// CreateAccessKey creates an access key for an IAM user
func (s *IAMService) CreateAccessKey(ctx context.Context, username string) (*AWSCredentials, error) {
	result, err := s.client.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{
		UserName: aws.String(username),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create access key: %w", err)
	}

	if result.AccessKey == nil {
		return nil, fmt.Errorf("access key is nil")
	}

	return &AWSCredentials{
		AccessKeyID:     *result.AccessKey.AccessKeyId,
		SecretAccessKey: *result.AccessKey.SecretAccessKey,
	}, nil
}

// CreateGitHubUser creates a non-console IAM user with S3 permissions for GitHub CI/CD
func (s *IAMService) CreateGitHubUser(ctx context.Context, username, bucket, repo string) (*AWSCredentials, error) {
	// Create the IAM user
	_, err := s.client.CreateUser(ctx, &iam.CreateUserInput{
		UserName: aws.String(username),
	})
	if err != nil {
		// Check if user already exists
		var entityAlreadyExists *types.EntityAlreadyExistsException
		if err == entityAlreadyExists {
			return nil, fmt.Errorf("user %s already exists", username)
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Build the inline policy document
	policyDocument := fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:PutObject"
            ],
            "Resource": "arn:aws:s3:::%s/%s/*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket"
            ],
            "Resource": "arn:aws:s3:::%s",
            "Condition": {
                "StringLike": {
                    "s3:prefix": "%s/*"
                }
            }
        }
    ]
}`, bucket, repo, bucket, repo)

	// Attach the inline policy
	_, err = s.client.PutUserPolicy(ctx, &iam.PutUserPolicyInput{
		UserName:       aws.String(username),
		PolicyName:     aws.String("github"),
		PolicyDocument: aws.String(policyDocument),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach policy to user: %w", err)
	}

	// Create access key for the user
	credentials, err := s.CreateAccessKey(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to create access key: %w", err)
	}

	return credentials, nil
}

// CreateGitHubOIDCRole creates an IAM role for GitHub Actions OIDC authentication
func (s *IAMService) CreateGitHubOIDCRole(ctx context.Context, roleName, owner, repo, bucket string) (string, error) {
	// Ensure OIDC provider exists
	providerARN, err := s.GetOrCreateGitHubOIDCProvider(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get/create OIDC provider: %w", err)
	}

	// Build trust policy for GitHub OIDC
	trustPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "%s"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "%s:aud": "%s"
        },
        "StringLike": {
          "%s:sub": "repo:%s/%s:*"
        }
      }
    }
  ]
}`, providerARN, GitHubOIDCProviderURL, GitHubOIDCAudience, GitHubOIDCProviderURL, owner, repo)

	// Try to get the role first (check if it exists)
	getResult, err := s.client.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	roleExists := err == nil && getResult.Role != nil

	if !roleExists {
		// Create the IAM role
		_, err = s.client.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(trustPolicy),
			Description:              aws.String(fmt.Sprintf("GitHub Actions OIDC role for %s/%s", owner, repo)),
		})
		if err != nil {
			return "", fmt.Errorf("failed to create role: %w", err)
		}
	} else {
		// Role exists, update the trust policy
		_, err = s.client.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
			RoleName:       aws.String(roleName),
			PolicyDocument: aws.String(trustPolicy),
		})
		if err != nil {
			return "", fmt.Errorf("failed to update trust policy: %w", err)
		}
	}

	// Build the S3 permissions policy
	policyDocument := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::%s/%s/*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket"
      ],
      "Resource": "arn:aws:s3:::%s",
      "Condition": {
        "StringLike": {
          "s3:prefix": "%s/*"
        }
      }
    }
  ]
}`, bucket, repo, bucket, repo)

	// Attach/update the inline policy to the role (PutRolePolicy is idempotent)
	_, err = s.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String("github-s3-access"),
		PolicyDocument: aws.String(policyDocument),
	})
	if err != nil {
		return "", fmt.Errorf("failed to attach/update policy to role: %w", err)
	}

	// Get AWS account ID to construct role ARN
	accountID, err := s.GetAWSAccountID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get AWS account ID: %w", err)
	}

	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
	return roleARN, nil
}
