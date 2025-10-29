// Package di provides a lightweight wrapper around uber's dig dependency injection framework.
// It simplifies container setup and provides type-safe dependency retrieval with generics.
package di

import (
	"github.com/savaki/aws-deployer/internal/services"
	"go.uber.org/dig"
)

// Container defines a dependency injection container based on uber's dig.
// This interface allows for easy testing and mocking of the DI container.
type Container interface {
	// Invoke executes a function, injecting its dependencies from the container.
	Invoke(function any, opts ...dig.InvokeOption) error

	// Provide registers a constructor function in the container.
	Provide(constructor any, opts ...dig.ProvideOption) error

	// Scope creates a scoped sub-container with its own set of values.
	Scope(name string, opts ...dig.ScopeOption) *dig.Scope
}

// MustGet returns an instance constructed via dependency injection or panics.
// This is a convenience function for retrieving a dependency from the container
// when you're certain it exists. If the dependency cannot be resolved, it will panic.
//
// Example:
//
//	db := MustGet[*Database](container)
func MustGet[T any](container Container) (want T) {
	callback := func(got T) {
		want = got
	}
	if err := container.Invoke(callback); err != nil {
		panic(err)
	}
	return want
}

// New creates a new dependency injection container for the given environment.
// The environment string is automatically registered as a string dependency
// that can be injected as a regular string parameter.
//
// Example:
//
//	container, err := New("production",
//	    WithProviders(
//	        func() *Database { return &Database{} },
//	        func(db *Database, env string) *Service { return &Service{DB: db, Env: env} },
//	    ),
//	)
func New(env string, opts ...Option) (Container, error) {
	// Build options
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	// Create dig container
	container := dig.New()
	if err := container.Provide(func() string { return env }); err != nil {
		return nil, err
	}
	if err := container.Provide(func() CallbackURL { return o.callbackURL }); err != nil {
		return nil, err
	}
	if err := container.Provide(func() DisableAuth { return DisableAuth(o.disableAuth) }); err != nil {
		return nil, err
	}

	// Register all provided constructors
	for _, provider := range core {
		if err := container.Provide(provider); err != nil {
			return nil, err
		}
	}

	// Register all provided constructors
	for _, provider := range o.providers {
		if err := container.Provide(provider); err != nil {
			return nil, err
		}
	}

	return container, nil
}

var core = []any{
	ProvideAWSConfig,
	ProvideContext,
	ProvideSSMClient,
	ProvideParameterStore,
	ProvideAppConfig,
	ProvideDynamoDB,
	ProvideStepFunctions,
	ProvideOrchestrator,
	ProvideSignerClient,
	ProvideS3Client,
	services.NewDynamoDBService,
	services.NewSecretsManagerService,
	services.NewSignatureVerifier,
	services.NewContainerMetadataParser,
}
