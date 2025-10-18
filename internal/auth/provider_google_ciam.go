package auth

import "fmt"

// GoogleCIAMProvider implements the Provider interface for Google Cloud Identity Platform (CIAM).
type GoogleCIAMProvider struct {
	ProjectID string // Google Cloud project ID
}

// GetIssuerURL returns the Google Identity Platform OIDC issuer URL.
// Note: Google Identity Platform (Firebase Auth) uses securetoken.google.com for token
// verification, but doesn't provide standard OAuth endpoints in the discovery document.
// For OAuth flows, Google Identity Platform uses identitytoolkit.googleapis.com endpoints
// which are not exposed via standard OIDC discovery.
// If you need browser-based OAuth login, consider using regular Google OAuth instead.
func (p *GoogleCIAMProvider) GetIssuerURL() string {
	return fmt.Sprintf("https://securetoken.google.com/%s", p.ProjectID)
}

// GetLogoutURL returns the logout URL for Google CIAM.
// Google Identity Platform doesn't require a provider-level logout endpoint.
// Logout is handled by clearing the local application session.
// We simply return the returnTo URL to redirect the user back to the app.
func (p *GoogleCIAMProvider) GetLogoutURL(clientID, returnTo string) string {
	// Google CIAM doesn't have a centralized logout URL like Auth0
	// The session is managed locally, so we just redirect back
	return returnTo
}

// GetProviderType returns "google-ciam".
func (p *GoogleCIAMProvider) GetProviderType() string {
	return "google-ciam"
}
