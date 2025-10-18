package auth

// Provider defines the interface for OAuth/OIDC providers.
// Different providers (Auth0, Google CIAM, etc.) implement this interface
// to provide provider-specific configuration and behavior.
type Provider interface {
	// GetIssuerURL returns the OIDC issuer URL for this provider.
	// This is used to discover the provider's OAuth2 endpoints.
	GetIssuerURL() string

	// GetLogoutURL returns the provider-specific logout URL.
	// clientID: OAuth client identifier
	// returnTo: URL to redirect to after logout
	GetLogoutURL(clientID, returnTo string) string

	// GetProviderType returns the provider type identifier (e.g., "auth0", "google-ciam").
	GetProviderType() string
}
