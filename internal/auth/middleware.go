package auth

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
)

// RequireAuth creates middleware that ensures the user is authenticated.
// If redirectOnFail is true (for document/HTML routes), it redirects to /login on auth failure.
// If redirectOnFail is false (for API routes), it returns a 403 JSON response on auth failure.
func (a *Authenticator) RequireAuth(redirectOnFail bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := zerolog.Ctx(r.Context())

			// Check if this is a NoOp authenticator (auth disabled)
			if a.IsNoOp() {
				logger.Debug().
					Str("path", r.URL.Path).
					Msg("⚠️  Authentication BYPASSED (NoOp mode)")
				next.ServeHTTP(w, r)
				return
			}

			session, err := a.sessionStore.Get(r, sessionName)
			if err != nil {
				// securecookie errors are expected when:
				// - Cookie encrypted with old/rotated keys
				// - Cookie tampered with
				// - No valid session exists
				// Log at debug level, not error
				logger.Debug().
					Str("path", r.URL.Path).
					Str("error", err.Error()).
					Msg("Invalid or expired session cookie")
				a.handleAuthFailure(w, r, redirectOnFail, "Session expired or invalid")
				return
			}

			// Check if profile exists in session
			profileJSON, ok := session.Values[profileKey].(string)
			if !ok || profileJSON == "" {
				logger.Debug().Str("path", r.URL.Path).Msg("No profile in session")
				a.handleAuthFailure(w, r, redirectOnFail, "Unauthorized")
				return
			}

			// Parse profile to extract email (for logging and future use)
			var profile Profile
			if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
				logger.Error().Err(err).Msg("Failed to parse profile from session")
				a.handleAuthFailure(w, r, redirectOnFail, "Invalid session data")
				return
			}

			logger.Debug().
				Str("path", r.URL.Path).
				Str("email", profile.Email).
				Str("sub", profile.Sub).
				Msg("Authenticated request")

			// User is authenticated, continue
			next.ServeHTTP(w, r)
		})
	}
}

// handleAuthFailure handles authentication failures based on the request type
func (a *Authenticator) handleAuthFailure(w http.ResponseWriter, r *http.Request, redirectOnFail bool, message string) {
	logger := zerolog.Ctx(r.Context())

	if redirectOnFail {
		// For document/HTML routes: redirect to login
		logger.Info().
			Str("path", r.URL.Path).
			Str("reason", message).
			Msg("Redirecting to login")
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	} else {
		// For API routes: return 403 JSON
		logger.Warn().
			Str("path", r.URL.Path).
			Str("reason", message).
			Msg("API authentication failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": message,
		})
	}
}
