package api

import (
	"context"
	"slices"
)

// RequiredScopeResolver resolves the exact OAuth 2.0 scope required by request
// attributes. An empty result means no access token may call the operation.
type RequiredScopeResolver func(attributes Attributes) string

// OAuth2ScopeAuthorizer authorizes access-token requests using their granted
// scopes. It returns DecisionNoOpinion for other authentication modes so
// callers can compose it with alternative Authorizers.
type OAuth2ScopeAuthorizer struct {
	// ResolveRequiredScope resolves the OAuth 2.0 scope required by the target
	// API operation. An empty result denies access-token requests.
	ResolveRequiredScope RequiredScopeResolver
}

// Authorize decides access-token requests and returns an RFC 6750 challenge
// error when the required scope is missing.
func (authorizer OAuth2ScopeAuthorizer) Authorize(ctx context.Context, authentication AuthenticationInfo, attributes Attributes) (Decision, string, error) {
	if authentication.Access == nil {
		return DecisionNoOpinion, "", nil
	}

	required := ""
	if authorizer.ResolveRequiredScope != nil {
		required = authorizer.ResolveRequiredScope(attributes)
	}
	if required != "" && slices.Contains(authentication.Access.Scopes, required) {
		return DecisionAllow, "", nil
	}

	reason := "missing required OAuth 2.0 scope"
	return DecisionDeny, reason, NewForbiddenChallengeError(
		`Bearer error="insufficient_scope"`,
		reason,
	)
}

var _ Authorizer = OAuth2ScopeAuthorizer{}
