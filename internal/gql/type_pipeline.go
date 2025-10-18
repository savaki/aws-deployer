package gql

import (
	"github.com/savaki/aws-deployer/internal/dao/targetdao"
)

// TargetResolver resolves the Target GraphQL type
type TargetResolver struct {
	target targetdao.Target
}

// newTargetResolver creates a new TargetResolver
func newTargetResolver(target targetdao.Target) *TargetResolver {
	return &TargetResolver{
		target: target,
	}
}

// AccountIds resolves the accountIds field
func (r *TargetResolver) AccountIds() []string {
	return r.target.AccountIDs
}

// Regions resolves the regions field
func (r *TargetResolver) Regions() []string {
	return r.target.Regions
}

// DeploymentTargetsResolver resolves the DeploymentTargets GraphQL type
type DeploymentTargetsResolver struct {
	record *targetdao.Record
}

// newDeploymentTargetsResolver creates a new DeploymentTargetsResolver
func newDeploymentTargetsResolver(record *targetdao.Record) *DeploymentTargetsResolver {
	return &DeploymentTargetsResolver{
		record: record,
	}
}

// Repo resolves the repo field
func (r *DeploymentTargetsResolver) Repo() string {
	return r.record.PK.String()
}

// Env resolves the env field
func (r *DeploymentTargetsResolver) Env() string {
	return r.record.SK
}

// Targets resolves the targets field
func (r *DeploymentTargetsResolver) Targets() []*TargetResolver {
	resolvers := make([]*TargetResolver, len(r.record.Targets))
	for i, target := range r.record.Targets {
		resolvers[i] = newTargetResolver(target)
	}
	return resolvers
}

// DownstreamEnvs resolves the downstreamEnvs field
func (r *DeploymentTargetsResolver) DownstreamEnvs() []string {
	if r.record.DownstreamEnv == nil {
		return []string{}
	}
	return r.record.DownstreamEnv
}

// PipelineConfigResolver resolves the PipelineConfig GraphQL type
type PipelineConfigResolver struct {
	repo         string
	initialEnv   string
	environments []*targetdao.Record
}

// newPipelineConfigResolver creates a new PipelineConfigResolver
func newPipelineConfigResolver(repo, initialEnv string, environments []*targetdao.Record) *PipelineConfigResolver {
	return &PipelineConfigResolver{
		repo:         repo,
		initialEnv:   initialEnv,
		environments: environments,
	}
}

// Repo resolves the repo field
func (r *PipelineConfigResolver) Repo() string {
	return r.repo
}

// InitialEnv resolves the initialEnv field
func (r *PipelineConfigResolver) InitialEnv() string {
	return r.initialEnv
}

// Environments resolves the environments field
func (r *PipelineConfigResolver) Environments() []*DeploymentTargetsResolver {
	resolvers := make([]*DeploymentTargetsResolver, len(r.environments))
	for i, env := range r.environments {
		resolvers[i] = newDeploymentTargetsResolver(env)
	}
	return resolvers
}
