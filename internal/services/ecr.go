package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type ECRService struct {
	client   *ecr.Client
	stsClient *sts.Client
	orgClient *organizations.Client
	region   string
}

func NewECRService(ctx context.Context, region string) (*ECRService, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &ECRService{
		client:   ecr.NewFromConfig(cfg),
		stsClient: sts.NewFromConfig(cfg),
		orgClient: organizations.NewFromConfig(cfg),
		region:   region,
	}, nil
}

type RepositoryInfo struct {
	Name string
	ARN  string
	URI  string
}

// CreateRepository creates an ECR repository with scan-on-push and tag immutability enabled
func (s *ECRService) CreateRepository(ctx context.Context, repositoryName string) (*RepositoryInfo, error) {
	input := &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repositoryName),
		ImageTagMutability: types.ImageTagMutabilityImmutable,
		ImageScanningConfiguration: &types.ImageScanningConfiguration{
			ScanOnPush: true,
		},
		Tags: []types.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("aws-deployer"),
			},
		},
	}

	output, err := s.client.CreateRepository(ctx, input)
	if err != nil {
		// Check if repository already exists - this is idempotent
		if strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
			// Repository exists, describe it to get details
			describeOutput, describeErr := s.client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
				RepositoryNames: []string{repositoryName},
			})
			if describeErr != nil {
				return nil, fmt.Errorf("repository exists but failed to describe: %w", describeErr)
			}
			if len(describeOutput.Repositories) == 0 {
				return nil, fmt.Errorf("repository exists but not found in describe")
			}
			repo := describeOutput.Repositories[0]
			return &RepositoryInfo{
				Name: aws.ToString(repo.RepositoryName),
				ARN:  aws.ToString(repo.RepositoryArn),
				URI:  aws.ToString(repo.RepositoryUri),
			}, nil
		}
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	return &RepositoryInfo{
		Name: aws.ToString(output.Repository.RepositoryName),
		ARN:  aws.ToString(output.Repository.RepositoryArn),
		URI:  aws.ToString(output.Repository.RepositoryUri),
	}, nil
}

// GetOrganizationID retrieves the AWS Organization ID if the account belongs to one
func (s *ECRService) GetOrganizationID(ctx context.Context) (string, error) {
	output, err := s.orgClient.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		// Not in an organization or no permissions
		if strings.Contains(err.Error(), "AWSOrganizationsNotInUseException") ||
			strings.Contains(err.Error(), "AccessDeniedException") {
			return "", nil
		}
		return "", fmt.Errorf("failed to describe organization: %w", err)
	}

	return aws.ToString(output.Organization.Id), nil
}

// SetRepositoryPolicy sets an organization-wide read policy on the repository
func (s *ECRService) SetRepositoryPolicy(ctx context.Context, repositoryName, organizationID string) error {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Sid":    "OrganizationAccess",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": "*",
				},
				"Action": []string{
					"ecr:GetDownloadUrlForLayer",
					"ecr:BatchGetImage",
					"ecr:BatchCheckLayerAvailability",
					"ecr:DescribeRepositories",
					"ecr:GetRepositoryPolicy",
					"ecr:ListImages",
				},
				"Condition": map[string]interface{}{
					"StringEquals": map[string]interface{}{
						"aws:PrincipalOrgID": organizationID,
					},
				},
			},
		},
	}

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	_, err = s.client.SetRepositoryPolicy(ctx, &ecr.SetRepositoryPolicyInput{
		RepositoryName: aws.String(repositoryName),
		PolicyText:     aws.String(string(policyJSON)),
	})
	if err != nil {
		return fmt.Errorf("failed to set repository policy: %w", err)
	}

	return nil
}


// GetAccountID retrieves the AWS account ID
func (s *ECRService) GetAccountID(ctx context.Context) (string, error) {
	output, err := s.stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}
	return aws.ToString(output.Account), nil
}
