package di

import (
	"os"

	"github.com/rs/zerolog"
)

// ProvideLogger creates a new zerolog.Logger configured for the runtime environment.
// In Lambda (when AWS_LAMBDA_RUNTIME_API is set), it uses JSON format.
// In terminal/CLI, it uses console format with pretty printing.
func ProvideLogger() zerolog.Logger {
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		// Running in Lambda - use JSON format
		return zerolog.New(os.Stdout).
			Level(zerolog.InfoLevel).
			With().
			Timestamp().
			Logger()
	}

	// Running in terminal - use console format with colors
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
		Level(zerolog.InfoLevel).
		With().
		Timestamp().
		Logger()
}
