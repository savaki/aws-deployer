package auth

// NewNoOpAuthenticator creates an Authenticator that bypasses all authentication.
// This should ONLY be used for local development.
// It returns an Authenticator with nil oidcProvider as a marker that it's in NoOp mode.
func NewNoOpAuthenticator() *Authenticator {
	return &Authenticator{
		oidcProvider:  nil, // nil marker indicates NoOp mode
		oauthProvider: nil,
		sessionStore:  nil,
		callbackURL:   "",
		authorizer:    nil,
	}
}

// IsNoOp returns true if this is a NoOp authenticator
func (a *Authenticator) IsNoOp() bool {
	return a.oidcProvider == nil && a.oauthProvider == nil
}
