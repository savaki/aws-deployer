package gql

import (
	"context"
)

// BuildsByRepo resolves the buildsByRepo query - lists all builds for a specific repo and environment
func (r *Resolver) BuildsByRepo(ctx context.Context, args struct {
	Repo string
	Env  string
}) ([]*BuildResolver, error) {
	records, err := r.build.QueryByRepoEnv(ctx, args.Repo, args.Env)
	if err != nil {
		return nil, err
	}

	// Map records to resolvers with targetDAO, deploymentDAO and context
	resolvers := make([]*BuildResolver, len(records))
	for i, record := range records {
		resolvers[i] = newBuildResolver(record, r.targetDAO, r.deploymentDAO, ctx)
	}

	return resolvers, nil
}
