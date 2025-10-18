package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/auth"
	"github.com/savaki/aws-deployer/internal/di"
	"github.com/savaki/aws-deployer/internal/services"
	"github.com/urfave/cli/v2"
)

//go:embed docroot
var docroot embed.FS

//go:embed graphiql.html
var graphiqlHTML string

type Handler struct {
	dbService     *services.DynamoDBService
	authenticator *auth.Authenticator
	schema        *graphql.Schema
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// loggingMiddleware logs details about each request and response
func loggingMiddleware(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Inject logger into request context
			ctx := logger.WithContext(r.Context())
			r = r.WithContext(ctx)

			// Create a custom response writer to capture status code
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Log incoming request
			zerolog.Ctx(ctx).Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("query", r.URL.RawQuery).
				Str("remote_addr", r.RemoteAddr).
				Str("user_agent", r.UserAgent()).
				Msg("Incoming request")

			// Call the next handler
			next.ServeHTTP(rw, r)

			// Calculate duration
			duration := time.Since(start)

			// Log response
			zerolog.Ctx(ctx).Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status_code", rw.statusCode).
				Dur("duration", duration).
				Msg("Request completed")
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// stripEnvPrefixMiddleware removes the /{env} prefix from request paths
func stripEnvPrefixMiddleware(env string, next http.Handler) http.Handler {
	// If env is empty, return the handler as-is
	if env == "" {
		return next
	}

	prefix := "/" + env
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the environment prefix if present
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)

		// Ensure path starts with /
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		next.ServeHTTP(w, r)
	})
}

func NewHandler(container di.Container) *Handler {
	return &Handler{
		dbService:     di.MustGet[*services.DynamoDBService](container),
		authenticator: di.MustGet[*auth.Authenticator](container),
		schema:        di.MustGet[*graphql.Schema](container),
	}
}

func setupContainer(env, callbackURL string, disableAuth bool) (di.Container, error) {
	return di.New(env,
		di.WithCallbackURL(callbackURL),
		di.WithDisableAuth(disableAuth),
		di.WithProviders(
			di.ProvideLogger,
			di.ProvideSessionKeyService,
			di.ProvideSessionKeys,
			di.ProvideAuthenticator,
			di.ProvideAuthorizer,
			di.ProvideBuildDAO,
			di.ProvideTargetDAO,
			di.ProvideDeploymentDAO,
			di.ProvideGraphQL,
		),
	)
}

// handleGraphQL serves the GraphQL API
func (h *Handler) handleGraphQL() http.Handler {
	// Use the relay handler which provides both GraphQL endpoint and GraphiQL interface
	return &relay.Handler{Schema: h.schema}
}

// handleGraphiQL serves the GraphiQL interface
func (h *Handler) handleGraphiQL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(graphiqlHTML))
}

// jsonResponse writes a JSON response
func (h *Handler) jsonResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"failed to marshal response"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

// errorResponse writes an error JSON response
func (h *Handler) errorResponse(w http.ResponseWriter, statusCode int, message string) {
	h.jsonResponse(w, statusCode, ErrorResponse{Error: message})
}

// handleStatic serves static files from the embedded docroot filesystem
func (h *Handler) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Get the file path, defaulting to index.html for root
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	// Read file from embedded filesystem
	content, err := docroot.ReadFile(filepath.Join("docroot", path))
	if err != nil {
		// If file not found, try index.html (for SPA routing)
		content, err = docroot.ReadFile("docroot/index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path = "index.html"
	}

	// Detect content type based on file extension
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Set content type header
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// setupRouter configures all HTTP routes
func (h *Handler) setupRouter() http.Handler {
	mux := http.NewServeMux()

	// Auth routes (no authentication required)
	mux.HandleFunc("GET /login", h.authenticator.HandleLogin)
	mux.HandleFunc("GET /logout", h.authenticator.HandleLogout)
	mux.HandleFunc("GET /oauth/callback", h.authenticator.HandleCallback)

	// GraphQL endpoints (authentication required - API mode: 403 on failure)
	// GET /graphql serves the GraphiQL interface
	// POST /graphql handles GraphQL queries
	requireAuthAPI := h.authenticator.RequireAuth(false) // false = return 403 for API calls
	mux.Handle("GET /graphql", requireAuthAPI(http.HandlerFunc(h.handleGraphiQL)))
	mux.Handle("POST /graphql", requireAuthAPI(h.handleGraphQL()))

	// Serve static files for all other routes (authentication required - redirect mode)
	requireAuth := h.authenticator.RequireAuth(true) // true = redirect to /login for documents
	mux.Handle("/", requireAuth(http.HandlerFunc(h.handleStatic)))

	return mux
}

// buildCallbackURL constructs the OAuth callback URL based on environment
func buildCallbackURL(env string, customDomain string, apiGatewayID string, port string) string {
	// For local development
	if port != "" {
		return fmt.Sprintf("http://localhost:%s/oauth/callback", port)
	}

	// For Lambda: check if custom domain is set
	if customDomain != "" {
		return fmt.Sprintf("https://%s/oauth/callback", customDomain)
	}

	// Default to API Gateway URL with environment prefix
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	if apiGatewayID != "" && env != "" {
		return fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s/oauth/callback", apiGatewayID, region, env)
	}

	// Fallback (should not happen in production)
	return "http://localhost:8080/oauth/callback"
}

// serveAction starts a local HTTP server for testing
func serveAction(c *cli.Context) error {
	port := c.String("port")
	addr := fmt.Sprintf(":%s", port)
	env := c.String("env")
	disableAuth := c.Bool("disable-auth")

	// Build callback URL for local dev (no custom domain or API Gateway ID in local mode)
	callbackURL := buildCallbackURL(env, "", "", port)

	// Initialize DI container
	container, err := setupContainer(env, callbackURL, disableAuth)
	if err != nil {
		return fmt.Errorf("failed to setup DI container: %w", err)
	}

	// Get logger from container
	logger := di.MustGet[zerolog.Logger](container)

	if disableAuth {
		logger.Warn().Msg("⚠️  Authentication is DISABLED - this should only be used for development")
	}

	handler := NewHandler(container)

	// Setup HTTP router
	router := handler.setupRouter()

	// Apply middleware stack: strip env prefix -> logging
	if env != "" {
		logger.Info().
			Str("addr", addr).
			Str("env", env).
			Str("callback_url", callbackURL).
			Bool("disable_auth", disableAuth).
			Msg("Starting HTTP server with env prefix stripping")
	} else {
		logger.Info().
			Str("addr", addr).
			Str("callback_url", callbackURL).
			Bool("disable_auth", disableAuth).
			Msg("Starting HTTP server")
	}

	httpHandler := loggingMiddleware(logger)(stripEnvPrefixMiddleware(env, router))

	server := &http.Server{
		Addr:    addr,
		Handler: httpHandler,
	}

	return server.ListenAndServe()
}

// listBuildsAction lists builds for a repository and environment
func listBuildsAction(c *cli.Context) error {
	// Create minimal DI container with just DynamoDB service
	container, err := di.New("",
		di.WithProviders(),
	)
	if err != nil {
		return fmt.Errorf("failed to create DI container: %w", err)
	}

	dbService := di.MustGet[*services.DynamoDBService](container)

	repo := c.String("repo")
	env := c.String("env")
	builds, err := dbService.QueryBuildsByRepo(context.Background(), repo, env)
	if err != nil {
		return fmt.Errorf("failed to query builds: %w", err)
	}

	jsonData, err := json.MarshalIndent(builds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal builds: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}

// getBuildAction gets a specific build
func getBuildAction(c *cli.Context) error {
	// Create minimal DI container with just DynamoDB service
	container, err := di.New("",
		di.WithProviders(),
	)
	if err != nil {
		return fmt.Errorf("failed to create DI container: %w", err)
	}

	dbService := di.MustGet[*services.DynamoDBService](container)

	repo := c.String("repo")
	env := c.String("env")
	ksuid := c.String("ksuid")

	build, err := dbService.GetBuild(context.Background(), repo, env, ksuid)
	if err != nil {
		return fmt.Errorf("failed to get build: %w", err)
	}

	jsonData, err := json.MarshalIndent(build, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal build: %w", err)
	}

	fmt.Println(string(jsonData))
	return nil
}

func main() {
	logger := di.ProvideLogger().With().Str("lambda", "server").Logger()

	// Check if running in Lambda environment
	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		// Get ENV from environment variable
		env := os.Getenv("ENV")
		if env == "" {
			env = os.Getenv("ENVIRONMENT")
		}
		if env == "" {
			logger.Error().Msg("ENV or ENVIRONMENT variable is required")
			os.Exit(1)
		}

		// Check if auth should be disabled (for development only)
		disableAuth := os.Getenv("DISABLE_AUTH") == "true"

		// Load config from Parameter Store to get custom domain and API Gateway ID
		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to load AWS config")
			os.Exit(1)
		}

		var paramStore services.ParameterStore
		if os.Getenv("DISABLE_SSM") == "true" {
			paramStore = services.NewEnvParameterStore(env)
		} else {
			ssmClient := di.ProvideSSMClient(cfg)
			paramStore = services.NewSSMParameterStore(ssmClient, env)
		}

		appConfig, err := paramStore.GetConfig(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to load configuration")
			os.Exit(1)
		}

		// Build callback URL (custom domain takes precedence)
		callbackURL := buildCallbackURL(env, appConfig.CustomDomain, appConfig.APIGatewayID, "")

		logger.Info().
			Str("env", env).
			Str("callback_url", callbackURL).
			Bool("disable_auth", disableAuth).
			Msg("Initializing Lambda handler")

		if disableAuth {
			logger.Warn().Msg("⚠️  Authentication is DISABLED - this should only be used for development")
		}

		container, err := setupContainer(env, callbackURL, disableAuth)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to setup DI container")
			os.Exit(1)
		}

		handler := NewHandler(container)

		// Setup HTTP router
		router := handler.setupRouter()

		// Apply middleware stack: strip env prefix -> logging
		httpHandler := loggingMiddleware(logger)(stripEnvPrefixMiddleware(env, router))

		// Use AWS Lambda HTTP adapter for API Gateway V2
		lambda.Start(httpadapter.NewV2(httpHandler).ProxyWithContext)
		return
	}

	// CLI mode for local testing
	app := &cli.App{
		Name:  "server",
		Usage: "API Gateway server management console",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "env",
				Usage:   "Environment name (for stripping path prefix)",
				EnvVars: []string{"ENV", "ENVIRONMENT"},
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Start local HTTP server for testing",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "port",
						Usage: "Port to listen on",
						Value: "8080",
					},
					&cli.BoolFlag{
						Name:    "disable-auth",
						Usage:   "Disable authentication (for local development only)",
						EnvVars: []string{"DISABLE_AUTH"},
					},
					&cli.BoolFlag{
						Name:    "disable-ssm",
						Usage:   "Disable AWS Systems Manager Parameter Store (use environment variables)",
						EnvVars: []string{"DISABLE_SSM"},
					},
				},
				Action: serveAction,
			},
			{
				Name:  "list-builds",
				Usage: "List builds for a repository and environment",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "repo",
						Usage:    "Repository name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "env",
						Usage:    "Environment name",
						Required: true,
					},
				},
				Action: listBuildsAction,
			},
			{
				Name:  "get-build",
				Usage: "Get a specific build",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "repo",
						Usage:    "Repository name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "env",
						Usage:    "Environment name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "ksuid",
						Usage:    "Build KSUID",
						Required: true,
					},
				},
				Action: getBuildAction,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error().Err(err).Msg("Application error")
		os.Exit(1)
	}
}
