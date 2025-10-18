package auth

import (
	"fmt"
	"net/url"
)

// Auth0Provider implements the Provider interface for Auth0.
type Auth0Provider struct {
	Domain string // Auth0 domain (e.g., "your-tenant.us.auth0.com")
}

// GetIssuerURL returns the Auth0 OIDC issuer URL.
func (p *Auth0Provider) GetIssuerURL() string {
	return fmt.Sprintf("https://%s/", p.Domain)
}

// GetLogoutURL returns the Auth0-specific logout URL.
// Auth0 requires calling their /v2/logout endpoint to fully log out the user
// from Auth0's session, not just the application session.
func (p *Auth0Provider) GetLogoutURL(clientID, returnTo string) string {
	logoutURL := fmt.Sprintf("https://%s/v2/logout", p.Domain)
	params := url.Values{}
	params.Add("client_id", clientID)
	params.Add("returnTo", returnTo)
	return fmt.Sprintf("%s?%s", logoutURL, params.Encode())
}

// GetProviderType returns "auth0".
func (p *Auth0Provider) GetProviderType() string {
	return "auth0"
}
