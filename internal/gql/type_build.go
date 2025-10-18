package gql

import (
	"context"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/savaki/aws-deployer/internal/dao/builddao"
	"github.com/savaki/aws-deployer/internal/dao/deploymentdao"
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
)

// BuildResolver resolves the Build GraphQL type
type BuildResolver struct {
	build         builddao.Record
	targetDAO     *targetdao.DAO
	deploymentDAO *deploymentdao.DAO
	ctx           context.Context
}

// newBuildResolver creates a new BuildResolver
func newBuildResolver(build builddao.Record, targetDAO *targetdao.DAO, deploymentDAO *deploymentdao.DAO, ctx context.Context) *BuildResolver {
	return &BuildResolver{
		build:         build,
		targetDAO:     targetDAO,
		deploymentDAO: deploymentDAO,
		ctx:           ctx,
	}
}

// ID resolves the id field (BuildID format: {repo}/{env}:{ksuid})
func (r *BuildResolver) ID() graphql.ID {
	return graphql.ID(r.build.GetID())
}

// Repo resolves the repo field
func (r *BuildResolver) Repo() string {
	return r.build.Repo
}

// Env resolves the env field
func (r *BuildResolver) Env() string {
	return r.build.Env
}

// BuildNumber resolves the buildNumber field
func (r *BuildResolver) BuildNumber() string {
	return r.build.BuildNumber
}

// Branch resolves the branch field
func (r *BuildResolver) Branch() string {
	return r.build.Branch
}

// Version resolves the version field
func (r *BuildResolver) Version() string {
	return r.build.Version
}

// CommitHash resolves the commitHash field
func (r *BuildResolver) CommitHash() string {
	return r.build.CommitHash
}

// Status resolves the status field
func (r *BuildResolver) Status() BuildStatus {
	return FromModelBuildStatus(r.build.Status)
}

// StackName resolves the stackName field
func (r *BuildResolver) StackName() string {
	return r.build.StackName
}

// ExecutionArn resolves the executionArn field
func (r *BuildResolver) ExecutionArn() *string {
	return r.build.ExecutionArn
}

// StartTime resolves the startTime field
func (r *BuildResolver) StartTime() DateTime {
	return NewDateTimeFromUnix(r.build.CreatedAt)
}

// EndTime resolves the endTime field
func (r *BuildResolver) EndTime() *DateTime {
	return NewDateTimePtrFromUnix(r.build.FinishedAt)
}

// ErrorMsg resolves the errorMsg field
func (r *BuildResolver) ErrorMsg() *string {
	return r.build.ErrorMsg
}

// DownstreamEnvs resolves the downstreamEnvs field by looking up target configuration
func (r *BuildResolver) DownstreamEnvs() ([]string, error) {
	// Get targets with fallback to default
	targets, err := r.targetDAO.GetWithDefault(r.ctx, r.build.Repo, r.build.Env)
	if err != nil {
		// On error, return empty array rather than failing the whole query
		return []string{}, nil
	}

	// If no targets configured, return empty array
	if targets == nil {
		return []string{}, nil
	}

	// Return downstream environments (may be empty)
	if targets.DownstreamEnv == nil {
		return []string{}, nil
	}

	return targets.DownstreamEnv, nil
}

// DeploymentErrors resolves the deploymentErrors field by fetching failed deployments
func (r *BuildResolver) DeploymentErrors() ([]*DeploymentErrorResolver, error) {
	// Query all deployments for this build
	deployments, err := r.deploymentDAO.QueryByBuild(r.ctx, r.build.Env, r.build.Repo, r.build.SK)
	if err != nil {
		// On error, return empty array rather than failing the whole query
		return []*DeploymentErrorResolver{}, nil
	}

	// Filter to only failed deployments and limit to first 3
	var resolvers []*DeploymentErrorResolver
	for _, deployment := range deployments {
		if deployment.Status == deploymentdao.StatusFailed {
			resolvers = append(resolvers, newDeploymentErrorResolver(deployment))
			if len(resolvers) >= 3 {
				break
			}
		}
	}

	return resolvers, nil
}

// DeploymentErrorResolver resolves the DeploymentError GraphQL type
type DeploymentErrorResolver struct {
	deployment deploymentdao.Record
}

// newDeploymentErrorResolver creates a new DeploymentErrorResolver
func newDeploymentErrorResolver(deployment deploymentdao.Record) *DeploymentErrorResolver {
	return &DeploymentErrorResolver{
		deployment: deployment,
	}
}

// AccountId resolves the accountId field
func (r *DeploymentErrorResolver) AccountId() string {
	account, _, _ := deploymentdao.ParseSK(r.deployment.SK)
	return account
}

// Region resolves the region field
func (r *DeploymentErrorResolver) Region() string {
	_, region, _ := deploymentdao.ParseSK(r.deployment.SK)
	return region
}

// StatusReason resolves the statusReason field
func (r *DeploymentErrorResolver) StatusReason() *string {
	if r.deployment.StatusReason == "" {
		return nil
	}
	return &r.deployment.StatusReason
}

// StackEvents resolves the stackEvents field
func (r *DeploymentErrorResolver) StackEvents() []string {
	if r.deployment.StackEvents == nil {
		return []string{}
	}
	return r.deployment.StackEvents
}
