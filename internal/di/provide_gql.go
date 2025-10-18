package di

import (
	"fmt"

	"github.com/graph-gophers/graphql-go"
	"github.com/savaki/aws-deployer/internal/gql"
)

func ProvideGraphQL(config gql.Config) (*graphql.Schema, error) {
	resolver := gql.NewResolver(config)
	schema, err := gql.NewSchema(resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL schema: %w", err)
	}
	return schema, nil
}
