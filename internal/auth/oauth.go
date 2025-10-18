package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog"
	"github.com/savaki/aws-deployer/internal/authz"
	"golang.org/x/oauth2"
)

const (
	sessionName = "auth-session"
	stateKey    = "state"
	profileKey  = "profile" // stores full profile JSON
	tokenKey    = "access_token"
)

type Authenticator struct {
	oidcProvider  *oidc.Provider
	oauthProvider Provider
	oauth2Config  oauth2.Config
	sessionStore  *sessions.CookieStore
	callbackURL   string
	authorizer    *authz.Authorizer // optional authorization policy enforcement
}

type Profile struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type AuthenticatorInput struct {
	Provider     Provider
	ClientID     string
	ClientSecret string
	CallbackURL  string
	Authorizer   *authz.Authorizer
	SessionKeys  [][]byte
	IsLocalDev   bool // Set to true for local development (disables Secure cookie flag)
}

func NewAuthenticator(ctx context.Context, input AuthenticatorInput) (*Authenticator, error) {
	oauthProvider := input.Provider
	clientID := input.ClientID
	clientSecret := input.ClientSecret
	callbackURL := input.CallbackURL
	authorizer := input.Authorizer
	sessionKeys := input.SessionKeys

	// Get issuer URL from provider
	issuerURL := oauthProvider.GetIssuerURL()
	logger := zerolog.Ctx(ctx)

	// Google CIAM (Firebase/Identity Platform) doesn't expose OAuth endpoints via OIDC discovery
	// When using Google OAuth for login, tokens are issued by accounts.google.com, not securetoken.google.com
	// So we need to use accounts.google.com as the issuer for token verification
	if oauthProvider.GetProviderType() == "google-ciam" {
		logger.Info().Msg("Google CIAM detected - using accounts.google.com for OAuth and token verification")
		issuerURL = "https://accounts.google.com"
	}

	logger.Info().
		Str("provider_type", oauthProvider.GetProviderType()).
		Str("issuer_url", issuerURL).
		Msg("Initializing OIDC provider")

	// Create OIDC provider for token verification
	oidcProvider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		logger.Error().
			Err(err).
			Str("issuer_url", issuerURL).
			Str("provider_type", oauthProvider.GetProviderType()).
			Msg("Failed to create OIDC provider")
		return nil, fmt.Errorf("failed to create OIDC provider for %s: %w", issuerURL, err)
	}

	endpoint := oidcProvider.Endpoint()

	// For Google CIAM, we could use discovered endpoints now since we're using accounts.google.com
	// But we'll keep the explicit configuration for clarity
	if oauthProvider.GetProviderType() == "google-ciam" {
		endpoint = oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		}
	}

	logger.Info().
		Str("auth_url", endpoint.AuthURL).
		Str("token_url", endpoint.TokenURL).
		Msg("OAuth endpoints configured")

	oauth2Config := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  callbackURL,
		Endpoint:     endpoint,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Use provided session keys (supports rotation - multiple valid keys)
	// If no keys provided, generate a fallback key (for local dev)
	if len(sessionKeys) == 0 {
		logger.Warn().Msg("No session keys provided, generating ephemeral fallback key")
		fallbackKey := make([]byte, 32)
		if _, err := rand.Read(fallbackKey); err != nil {
			return nil, fmt.Errorf("failed to generate fallback session key: %w", err)
		}
		sessionKeys = [][]byte{fallbackKey}
	}

	// Create session store with multiple keys (newest first)
	// gorilla/sessions will encrypt with the first key and try decrypting with all keys
	sessionStore := sessions.NewCookieStore(sessionKeys...)

	// Secure flag should only be true for HTTPS (production)
	// For local dev on http://localhost, Secure must be false or cookies won't work
	isSecure := !input.IsLocalDev

	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	}

	logger.Info().
		Int("session_key_count", len(sessionKeys)).
		Str("provider_type", oauthProvider.GetProviderType()).
		Bool("secure_cookies", isSecure).
		Msg("Authenticator initialized")

	return &Authenticator{
		oidcProvider:  oidcProvider,
		oauthProvider: oauthProvider,
		oauth2Config:  oauth2Config,
		sessionStore:  sessionStore,
		callbackURL:   callbackURL,
		authorizer:    authorizer,
	}, nil
}

// generateState creates a random state value for CSRF protection
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// HandleLogin redirects to the OAuth provider for authentication
func (a *Authenticator) HandleLogin(w http.ResponseWriter, r *http.Request) {
	logger := zerolog.Ctx(r.Context())

	// NoOp mode - redirect to home
	if a.IsNoOp() {
		logger.Info().Msg("Login not required in NoOp auth mode, redirecting to home")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	state, err := generateState()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to generate state")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get or create new session (ignore decrypt errors - we're creating fresh state)
	session, _ := a.sessionStore.Get(r, sessionName)
	// Note: We intentionally ignore the error here because:
	// - If cookie is invalid/expired, Get() returns a new empty session
	// - We're about to overwrite it with fresh state anyway
	// - This is the entry point for authentication, so no valid session is expected

	session.Values[stateKey] = state
	if err := session.Save(r, w); err != nil {
		logger.Error().Err(err).Msg("Failed to save session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Log session details for debugging
	logger.Info().
		Str("session_name", sessionName).
		Str("state", state).
		Bool("session_is_new", session.IsNew).
		Msg("Session saved with state in HandleLogin")

	// Redirect to OAuth provider
	authURL := a.oauth2Config.AuthCodeURL(state)
	logger.Info().
		Str("auth_url", authURL).
		Str("provider", a.oauthProvider.GetProviderType()).
		Msg("Redirecting to OAuth provider for login")
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback handles the OAuth2 callback from the OAuth provider
func (a *Authenticator) HandleCallback(w http.ResponseWriter, r *http.Request) {
	logger := zerolog.Ctx(r.Context())

	// NoOp mode - redirect to home
	if a.IsNoOp() {
		logger.Info().Msg("OAuth callback not used in NoOp auth mode, redirecting to home")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Log received cookies for debugging
	cookieHeader := r.Header.Get("Cookie")
	hasCookie := false
	if cookie, err := r.Cookie(sessionName); err == nil {
		hasCookie = true
		logger.Info().
			Str("cookie_name", cookie.Name).
			Int("cookie_value_length", len(cookie.Value)).
			Str("cookie_path", cookie.Path).
			Str("cookie_domain", cookie.Domain).
			Bool("cookie_secure", cookie.Secure).
			Msg("Session cookie received in callback")
	} else {
		logger.Warn().
			Str("cookie_header", cookieHeader).
			Msg("No session cookie found in callback request")
	}

	// Get session (ignore decrypt errors - we need to check state that we just set in HandleLogin)
	session, err := a.sessionStore.Get(r, sessionName)
	if err != nil {
		logger.Warn().
			Str("error", err.Error()).
			Bool("had_cookie", hasCookie).
			Str("expected_session_name", sessionName).
			Msg("Session cookie error in callback, redirecting to login")
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		return
	}

	// Log successful session retrieval
	logger.Info().
		Bool("session_is_new", session.IsNew).
		Int("session_values_count", len(session.Values)).
		Msg("Session retrieved successfully in callback")

	// Verify state
	storedState, ok := session.Values[stateKey].(string)
	if !ok || storedState == "" {
		logger.Error().
			Bool("state_exists", ok).
			Int("session_values_count", len(session.Values)).
			Msg("State not found in session")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	receivedState := r.URL.Query().Get("state")
	if receivedState != storedState {
		logger.Error().
			Str("received_state", receivedState[:10]+"...").
			Str("stored_state", storedState[:10]+"...").
			Msg("State mismatch")
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	logger.Info().Msg("State verified successfully")

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		logger.Error().Msg("Code not found in callback")
		http.Error(w, "Code not found", http.StatusBadRequest)
		return
	}

	token, err := a.oauth2Config.Exchange(r.Context(), code)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to exchange code for token")
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	// Extract ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		logger.Error().Msg("No id_token in token response")
		http.Error(w, "No id_token", http.StatusInternalServerError)
		return
	}

	preview := rawIDToken
	if len(preview) > 50 {
		preview = preview[:50] + "..."
	}
	logger.Info().
		Int("id_token_length", len(rawIDToken)).
		Str("id_token_preview", preview).
		Msg("ID token received")

	// Verify ID token
	verifier := a.oidcProvider.Verifier(&oidc.Config{ClientID: a.oauth2Config.ClientID})
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		logger.Error().
			Err(err).
			Str("expected_issuer", a.oidcProvider.Endpoint().AuthURL).
			Str("client_id", a.oauth2Config.ClientID).
			Msg("Failed to verify ID token")
		http.Error(w, "Failed to verify token", http.StatusInternalServerError)
		return
	}

	logger.Info().
		Str("issuer", idToken.Issuer).
		Str("subject", idToken.Subject).
		Msg("ID token verified successfully")

	// Extract profile
	var profile Profile
	if err := idToken.Claims(&profile); err != nil {
		logger.Error().Err(err).Msg("Failed to extract claims")
		http.Error(w, "Failed to extract profile", http.StatusInternalServerError)
		return
	}

	// Authorize user (if authorizer is configured)
	if a.authorizer != nil {
		authzProfile := authz.Profile{
			Sub:   profile.Sub,
			Name:  profile.Name,
			Email: profile.Email,
		}
		if err := a.authorizer.Authorize(authzProfile); err != nil {
			logger.Warn().
				Str("sub", profile.Sub).
				Str("email", profile.Email).
				Err(err).
				Msg("User authorization failed")
			http.Error(w, fmt.Sprintf("Access denied: %v", err), http.StatusForbidden)
			return
		}
	}

	// Store full profile as JSON in session
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal profile")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	session.Values[profileKey] = string(profileJSON)
	session.Values[tokenKey] = token.AccessToken
	delete(session.Values, stateKey) // Clean up state

	if err := session.Save(r, w); err != nil {
		logger.Error().Err(err).Msg("Failed to save session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	logger.Info().Str("sub", profile.Sub).Msg("User authenticated successfully")

	// Redirect to home
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// HandleLogout performs logout (clears session and redirects to provider logout if supported)
func (a *Authenticator) HandleLogout(w http.ResponseWriter, r *http.Request) {
	logger := zerolog.Ctx(r.Context())

	// NoOp mode - redirect to home
	if a.IsNoOp() {
		logger.Info().Msg("Logout not required in NoOp auth mode, redirecting to home")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	// Get or create session (ignore decrypt errors - we're clearing it anyway)
	session, _ := a.sessionStore.Get(r, sessionName)

	// Clear session
	session.Options.MaxAge = -1
	if err := session.Save(r, w); err != nil {
		logger.Error().Err(err).Msg("Failed to clear session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get the return URL (scheme + host from callback URL)
	callbackURL, err := url.Parse(a.callbackURL)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse callback URL")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	returnTo := fmt.Sprintf("%s://%s", callbackURL.Scheme, callbackURL.Host)

	// Get provider-specific logout URL
	logoutURL := a.oauthProvider.GetLogoutURL(a.oauth2Config.ClientID, returnTo)

	logger.Info().
		Str("logout_url", logoutURL).
		Str("provider", a.oauthProvider.GetProviderType()).
		Msg("Logging out user")

	// Redirect to provider logout (or return URL for providers without logout endpoint)
	http.Redirect(w, r, logoutURL, http.StatusTemporaryRedirect)
}
