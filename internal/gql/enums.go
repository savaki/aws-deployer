package gql

import "github.com/savaki/aws-deployer/internal/dao/builddao"

// BuildStatus represents the GraphQL BuildStatus enum
type BuildStatus string

const (
	BuildStatusPending    BuildStatus = "PENDING"
	BuildStatusInProgress BuildStatus = "IN_PROGRESS"
	BuildStatusSuccess    BuildStatus = "SUCCESS"
	BuildStatusFailed     BuildStatus = "FAILED"
)

// FromModelBuildStatus converts a builddao.BuildStatus to gql.BuildStatus
func FromModelBuildStatus(status builddao.BuildStatus) BuildStatus {
	return BuildStatus(status)
}

// ToModelBuildStatus converts a gql.BuildStatus to builddao.BuildStatus
func (s BuildStatus) ToModelBuildStatus() builddao.BuildStatus {
	return builddao.BuildStatus(s)
}
