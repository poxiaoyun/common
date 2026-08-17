package api

import (
	"context"
	"net/http"
	"slices"
)

// OAuth2RequiredScope maps request attributes to the exact OAuth 2.0 scope
// required by an API operation. An empty result means the operation is not
// mapped for OAuth 2.0 service principals.
type OAuth2RequiredScope func(attributes Attributes) string

// WithPrefix returns a scope mapper that prefixes non-empty results with
// prefix and a dot. Token scopes remain exact strings; this method does not
// introduce wildcard or pattern matching.
func (required OAuth2RequiredScope) WithPrefix(prefix string) OAuth2RequiredScope {
	return func(attributes Attributes) string {
		scope := required(attributes)
		if scope == "" {
			return ""
		}
		return prefix + "." + scope
	}
}

// OAuth2ScopeAuthorizer authorizes OAuth 2.0 service principals using the
// scopes granted to their access token. It returns DecisionNoOpinion for other
// principal types so callers can compose it with other Authorizers.
type OAuth2ScopeAuthorizer struct {
	// RequiredScope maps request attributes to the OAuth 2.0 scope required by
	// the target API operation. An empty result denies service principals.
	RequiredScope OAuth2RequiredScope
}

// IsOAuth2ClientPrincipal reports whether user was authenticated from an OAuth
// 2.0 access token issued to a service client.
func IsOAuth2ClientPrincipal(user UserInfo) bool {
	return slices.Contains(user.Extra[IAMPrincipalTypeExtra], OAuth2ClientPrincipalType)
}

// SetOAuth2InsufficientScopeChallenge adds the RFC 6750 challenge for an
// authenticated OAuth Client that is not authorized for a request.
func SetOAuth2InsufficientScopeChallenge(header http.Header) {
	header.Set("WWW-Authenticate", `Bearer error="insufficient_scope"`)
}

// Authorize implements Authorizer. A service request without a mapped and
// granted scope is denied instead of falling through to user authorization.
func (a OAuth2ScopeAuthorizer) Authorize(_ context.Context, user UserInfo, attributes Attributes) (Decision, string, error) {
	if !IsOAuth2ClientPrincipal(user) {
		return DecisionNoOpinion, "", nil
	}
	required := a.RequiredScope(attributes)
	if required != "" && slices.Contains(user.Extra[OAuth2ScopeExtra], required) {
		return DecisionAllow, "", nil
	}
	return DecisionDeny, "missing required OAuth 2.0 scope", nil
}

var _ Authorizer = OAuth2ScopeAuthorizer{}
