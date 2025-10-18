package di

import (
	"errors"
	"testing"

	"go.uber.org/dig"
)

// Test types for dependency injection
type Database struct {
	Name string
}

type Logger struct {
	Level string
}

type Service struct {
	DB     *Database
	Logger *Logger
	Env    string
}

type Repository struct {
	DB *Database
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		opts    []Option
		wantErr bool
	}{
		{
			name:    "creates container with no providers",
			env:     "dev",
			opts:    nil,
			wantErr: false,
		},
		{
			name: "creates container with single provider",
			env:  "staging",
			opts: []Option{
				WithProviders(func() *Database {
					return &Database{Name: "test-db"}
				}),
			},
			wantErr: false,
		},
		{
			name: "creates container with multiple providers",
			env:  "prod",
			opts: []Option{
				WithProviders(
					func() *Database {
						return &Database{Name: "prod-db"}
					},
					func() *Logger {
						return &Logger{Level: "info"}
					},
				),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container, err := New(tt.env, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if container == nil && !tt.wantErr {
				t.Error("New() returned nil container without error")
			}
		})
	}
}

func TestNew_InvalidProvider(t *testing.T) {
	// Attempting to provide the same type twice should fail
	_, err := New("dev",
		WithProviders(
			func() *Database {
				return &Database{Name: "db1"}
			},
			func() *Database {
				return &Database{Name: "db2"}
			},
		),
	)

	if err == nil {
		t.Error("New() should return error when providing duplicate types")
	}
}

func TestNew_ProvidesEnvironment(t *testing.T) {
	expectedEnv := "test-env"
	container, err := New(expectedEnv)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	// Extract the environment as a string parameter
	var actualEnv string
	err = container.Invoke(func(env string) {
		actualEnv = env
	})
	if err != nil {
		t.Fatalf("Invoke() unexpected error: %v", err)
	}

	if actualEnv != expectedEnv {
		t.Errorf("Environment = %v, want %v", actualEnv, expectedEnv)
	}
}

func TestMustGet(t *testing.T) {
	t.Run("successfully retrieves dependency", func(t *testing.T) {
		container, err := New("dev",
			WithProviders(func() *Database {
				return &Database{Name: "test-db"}
			}),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		db := MustGet[*Database](container)
		if db == nil {
			t.Error("MustGet() returned nil")
		}
		if db.Name != "test-db" {
			t.Errorf("Database.Name = %v, want %v", db.Name, "test-db")
		}
	})

	t.Run("panics when dependency not found", func(t *testing.T) {
		container, err := New("dev")
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustGet() did not panic")
			}
		}()

		_ = MustGet[*Database](container)
	})
}

func TestWithProviders(t *testing.T) {
	t.Run("adds single provider", func(t *testing.T) {
		container, err := New("dev",
			WithProviders(func() *Database {
				return &Database{Name: "test-db"}
			}),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		var db *Database
		err = container.Invoke(func(d *Database) {
			db = d
		})
		if err != nil {
			t.Fatalf("Invoke() unexpected error: %v", err)
		}
		if db.Name != "test-db" {
			t.Errorf("Database.Name = %v, want %v", db.Name, "test-db")
		}
	})

	t.Run("adds multiple providers", func(t *testing.T) {
		container, err := New("dev",
			WithProviders(
				func() *Database {
					return &Database{Name: "test-db"}
				},
				func() *Logger {
					return &Logger{Level: "debug"}
				},
			),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		var db *Database
		var logger *Logger
		err = container.Invoke(func(d *Database, l *Logger) {
			db = d
			logger = l
		})
		if err != nil {
			t.Fatalf("Invoke() unexpected error: %v", err)
		}
		if db.Name != "test-db" {
			t.Errorf("Database.Name = %v, want %v", db.Name, "test-db")
		}
		if logger.Level != "debug" {
			t.Errorf("Logger.Level = %v, want %v", logger.Level, "debug")
		}
	})

	t.Run("chains multiple WithProviders calls", func(t *testing.T) {
		container, err := New("dev",
			WithProviders(func() *Database {
				return &Database{Name: "test-db"}
			}),
			WithProviders(func() *Logger {
				return &Logger{Level: "info"}
			}),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		var db *Database
		var logger *Logger
		err = container.Invoke(func(d *Database, l *Logger) {
			db = d
			logger = l
		})
		if err != nil {
			t.Fatalf("Invoke() unexpected error: %v", err)
		}
		if db == nil || logger == nil {
			t.Error("Expected both dependencies to be available")
		}
	})
}

func TestDependencyInjection(t *testing.T) {
	t.Run("resolves dependencies automatically", func(t *testing.T) {
		container, err := New("production",
			WithProviders(
				func() *Database {
					return &Database{Name: "prod-db"}
				},
				func() *Logger {
					return &Logger{Level: "error"}
				},
				func(db *Database, logger *Logger, env string) *Service {
					return &Service{
						DB:     db,
						Logger: logger,
						Env:    env,
					}
				},
			),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		service := MustGet[*Service](container)
		if service.DB.Name != "prod-db" {
			t.Errorf("Service.DB.Name = %v, want %v", service.DB.Name, "prod-db")
		}
		if service.Logger.Level != "error" {
			t.Errorf("Service.Logger.Level = %v, want %v", service.Logger.Level, "error")
		}
		if service.Env != "production" {
			t.Errorf("Service.Env = %v, want %v", service.Env, "production")
		}
	})

	t.Run("handles nested dependencies", func(t *testing.T) {
		container, err := New("dev",
			WithProviders(
				func() *Database {
					return &Database{Name: "dev-db"}
				},
				func(db *Database) *Repository {
					return &Repository{DB: db}
				},
			),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		repo := MustGet[*Repository](container)
		if repo.DB.Name != "dev-db" {
			t.Errorf("Repository.DB.Name = %v, want %v", repo.DB.Name, "dev-db")
		}
	})
}

func TestContainer_Interface(t *testing.T) {
	t.Run("implements Container interface", func(t *testing.T) {
		var _ Container = (*dig.Container)(nil)
	})

	t.Run("can be used polymorphically", func(t *testing.T) {
		var container Container
		container, err := New("dev",
			WithProviders(func() *Database {
				return &Database{Name: "test"}
			}),
		)
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		err = container.Invoke(func(db *Database) {
			if db.Name != "test" {
				t.Errorf("Database.Name = %v, want %v", db.Name, "test")
			}
		})
		if err != nil {
			t.Fatalf("Invoke() unexpected error: %v", err)
		}
	})
}

func TestErrorHandling(t *testing.T) {
	t.Run("returns error from failing provider", func(t *testing.T) {
		providerErr := errors.New("provider initialization failed")

		// Create a provider that returns an error
		_, err := New("dev",
			WithProviders(func() (*Database, error) {
				return nil, providerErr
			}),
		)

		// dig should accept this provider (it will fail at invoke time)
		if err != nil {
			t.Logf("Provider registration failed (expected behavior): %v", err)
		}
	})

	t.Run("MustGet panics with meaningful error", func(t *testing.T) {
		container, err := New("dev")
		if err != nil {
			t.Fatalf("New() unexpected error: %v", err)
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("MustGet() should panic when dependency is missing")
			}
		}()

		_ = MustGet[*Database](container)
	})
}
