package di

type CallbackURL string
type DisableAuth bool

// Option is a function that configures the dependency injection container.
type Option func(*options)

func WithCallbackURL(url string) Option {
	return func(opts *options) {
		opts.callbackURL = CallbackURL(url)
	}
}

func WithDisableAuth(disable bool) Option {
	return func(opts *options) {
		opts.disableAuth = disable
	}
}

// WithProviders adds constructor functions to the dependency injection container.
// Each provider should be a constructor function that returns one or more values.
// Providers can declare dependencies as function parameters, which will be
// automatically resolved by the container.
//
// Example:
//
//	WithProviders(
//	    func() *Database { return &Database{} },
//	    func(db *Database) *Service { return &Service{DB: db} },
//	)
func WithProviders(providers ...any) Option {
	return func(opts *options) {
		opts.providers = append(opts.providers, providers...)
	}
}

type options struct {
	callbackURL CallbackURL
	providers   []any
	disableAuth bool
}
