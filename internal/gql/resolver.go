package gql

import (
	_ "embed"

	"github.com/graph-gophers/graphql-go"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
	"github.com/savaki/aws-deployer/internal/orchestrator"
	"github.com/savaki/aws-deployer/internal/services"
	"go.uber.org/dig"
)

//go:embed schema.graphqls
var schemaString string

type Config struct {
	dig.In

	Build         *builddao.DAO
	TargetDAO     *targetdao.DAO
	DeploymentDAO *deploymentdao.DAO
	DbService     *services.DynamoDBService
	Orchestrator  *orchestrator.Orchestrator
	AppConfig     *services.Config
}

// Resolver is the root GraphQL resolver
type Resolver struct {
	build         *builddao.DAO
	targetDAO     *targetdao.DAO
	deploymentDAO *deploymentdao.DAO
	dbService     *services.DynamoDBService
	orchestrator  *orchestrator.Orchestrator
	appConfig     *services.Config
}

// NewResolver creates a new root resolver with the required dependencies
func NewResolver(config Config) *Resolver {
	return &Resolver{
		build:         config.Build,
		targetDAO:     config.TargetDAO,
		deploymentDAO: config.DeploymentDAO,
		dbService:     config.DbService,
		orchestrator:  config.Orchestrator,
		appConfig:     config.AppConfig,
	}
}

// NewSchema creates a new GraphQL schema with the root resolver
func NewSchema(resolver *Resolver) (*graphql.Schema, error) {
	schema := graphql.MustParseSchema(schemaString, resolver)
	return schema, nil
}

// Ok returns "ok" for health checks
func (r *Resolver) Ok() string {
	return "ok"
}
