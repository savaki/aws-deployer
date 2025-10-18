package services

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
)

type DynamoDBService struct {
	client    *dynamodb.Client
	tableName string
	dao       *builddao.DAO
}

func NewDynamoDBService(env string) (*DynamoDBService, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Use the DAO's TableName function to derive the table name from environment
	tableName := builddao.TableName(env)

	client := dynamodb.NewFromConfig(cfg)
	dao := builddao.New(client, tableName)

	return &DynamoDBService{
		client:    client,
		tableName: tableName,
		dao:       dao,
	}, nil
}

// NewDynamoDBServiceWithClient creates a DynamoDBService with a custom client and table name.
// This is useful for testing with local DynamoDB.
func NewDynamoDBServiceWithClient(client *dynamodb.Client, tableName string) *DynamoDBService {
	dao := builddao.New(client, tableName)
	return &DynamoDBService{
		client:    client,
		tableName: tableName,
		dao:       dao,
	}
}

// GetClient returns the underlying DynamoDB client. This is useful for testing.
func (d *DynamoDBService) GetClient() *dynamodb.Client {
	return d.client
}

// GetTableName returns the table name. This is useful for testing.
func (d *DynamoDBService) GetTableName() string {
	return d.tableName
}

// GetBuild retrieves a build record by repo, env, and KSUID
// Returns an error if not found
func (d *DynamoDBService) GetBuild(ctx context.Context, repo, env, ksuid string) (builddao.Record, error) {
	pk := builddao.NewPK(repo, env)
	id := builddao.NewID(pk, ksuid)
	return d.dao.Find(ctx, id)
}

// PutBuild creates a new build record (wraps DAO.Create)
func (d *DynamoDBService) PutBuild(ctx context.Context, input builddao.CreateInput) (builddao.Record, error) {
	return d.dao.Create(ctx, input)
}

// UpdateBuildStatus updates the status of a build (wraps DAO.UpdateStatus)
func (d *DynamoDBService) UpdateBuildStatus(ctx context.Context, input builddao.UpdateInput) (builddao.Record, error) {
	if err := d.dao.UpdateStatus(ctx, input); err != nil {
		return builddao.Record{}, err
	}
	id := builddao.NewID(input.PK, input.SK)
	return d.dao.Find(ctx, id)
}

// QueryBuildsByRepo returns all builds for a given repository and environment
func (d *DynamoDBService) QueryBuildsByRepo(ctx context.Context, repo, env string) ([]builddao.Record, error) {
	return d.dao.QueryByRepoEnv(ctx, repo, env)
}

// QueryLatestBuildsByEnv returns the latest build for each repo in the given environment
// using the "latest" magic records for efficient querying
func (d *DynamoDBService) QueryLatestBuildsByEnv(ctx context.Context, env string) ([]builddao.Record, error) {
	return d.dao.QueryLatestBuilds(ctx, env)
}
