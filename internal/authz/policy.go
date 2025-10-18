package authz

import (
	"fmt"
	"strings"
)

// Profile represents user information needed for authorization.
// This mirrors the auth.Profile struct but keeps packages decoupled.
type Profile struct {
	Sub   string
	Name  string
	Email string
}

// Policy defines an authorization rule that can allow or deny access.
type Policy interface {
	// Authorize returns nil if the user is authorized, or an error if denied.
	Authorize(profile Profile) error
	// Name returns a human-readable name for this policy.
	Name() string
}

// GoogleEmailPolicy restricts access to a specific email when using Google authentication.
// Behavior varies by OAuth provider:
// - Auth0: Only applies to federated Google login (sub starts with "google-oauth2|")
// - Google CIAM: Applies to all users (all users are Google users)
type GoogleEmailPolicy struct {
	AllowedEmail string
	ProviderType string // "auth0" or "google-ciam"
}

// Name returns the policy name.
func (p *GoogleEmailPolicy) Name() string {
	return "GoogleEmailRestriction"
}

// Authorize checks if the user is authorized based on Google email policy.
func (p *GoogleEmailPolicy) Authorize(profile Profile) error {
	switch p.ProviderType {
	case "auth0":
		// For Auth0: only apply policy to Google federated logins
		// Auth0 Google logins have sub format: "google-oauth2|123456"
		if strings.HasPrefix(profile.Sub, "google-oauth2|") {
			// This is a Google sign-in via Auth0, enforce email restriction
			if profile.Email != p.AllowedEmail {
				return fmt.Errorf("access denied: email %s is not authorized for Google authentication", profile.Email)
			}
		}
		// For non-Google Auth0 providers, allow access
		return nil

	case "google-ciam":
		// For Google CIAM: all users are Google users, always check email
		if profile.Email != p.AllowedEmail {
			return fmt.Errorf("access denied: email %s is not authorized", profile.Email)
		}
		return nil

	default:
		// Unknown provider type, deny access to be safe
		return fmt.Errorf("unknown provider type: %s", p.ProviderType)
	}
}

// Authorizer manages a collection of authorization policies.
type Authorizer struct {
	policies []Policy
	enabled  bool
}

// NewAuthorizer creates a new authorizer with the given policies.
func NewAuthorizer(enabled bool, policies ...Policy) *Authorizer {
	return &Authorizer{
		policies: policies,
		enabled:  enabled,
	}
}

// Authorize runs all policies and returns an error if any policy denies access.
func (a *Authorizer) Authorize(profile Profile) error {
	if !a.enabled {
		return nil
	}

	for _, policy := range a.policies {
		if err := policy.Authorize(profile); err != nil {
			return fmt.Errorf("authorization policy %s failed: %w", policy.Name(), err)
		}
	}
	return nil
}

// NewGoogleEmailAuthorizer creates a preconfigured authorizer for Google email restrictions.
// This is a convenience function for the common use case.
// providerType should be "auth0" or "google-ciam" to determine policy behavior.
func NewGoogleEmailAuthorizer(enabled bool, allowedEmail string, providerType string) *Authorizer {
	return NewAuthorizer(enabled, &GoogleEmailPolicy{
		AllowedEmail: allowedEmail,
		ProviderType: providerType,
	})
}
