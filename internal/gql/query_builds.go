package gql

import (
	"context"
)

// Builds resolves the builds query - lists recent builds for a given environment
// Note: This currently returns all builds for all repos in the environment.
// TODO: Once we have a way to list all repos, we can query each one individually.
func (r *Resolver) Builds(ctx context.Context, args struct{ Env string }) ([]*BuildResolver, error) {
	records, err := r.build.QueryLatestBuilds(ctx, args.Env)
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
