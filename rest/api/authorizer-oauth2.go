package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"xiaoshiai.cn/common/authz"
)

// ScopeMatcher reports whether one granted OAuth 2.0 scope authorizes request
// attributes.
type ScopeMatcher func(scope string, operation authz.Operation) bool

// ScopeActionMatcher reports whether an aggregate action from a granted scope
// authorizes the request attributes. Exact actions are matched before this
// adapter is called.
type ScopeActionMatcher func(grantedAction string, operation authz.Operation) bool

// ScopeResourceMatcher reports whether a resource from a granted scope names
// the logical target represented by the request attributes.
type ScopeResourceMatcher func(grantedResource string, operation authz.Operation) bool

// OAuth2ScopeAuthorizer authorizes access-token requests using their granted
// scopes. It returns NoOpinion for other authentication modes so
// callers can compose it with alternative Authorizers.
type OAuth2ScopeAuthorizer struct {
	// MatchScope reports whether one granted scope authorizes the target API
	// operation. Nil uses the action:resource convention.
	MatchScope ScopeMatcher
}

// Authorize decides access-token requests and returns an RFC 6750 challenge
// error when the required scope is missing.
func (authorizer OAuth2ScopeAuthorizer) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	if authentication.Token == nil {
		return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, nil
	}

	match := authorizer.MatchScope
	if match == nil {
		match = DefaultOAuth2ScopeMatcher
	}
	if slices.ContainsFunc(authentication.Token.Scopes, func(scope string) bool {
		return match(scope, operation)
	}) {
		return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
	}

	reason := "missing required OAuth 2.0 scope"
	return authz.EvaluationResult{Decision: authz.DecisionDeny, Reason: reason}, NewForbiddenChallengeError(
		`Bearer error="insufficient_scope"`,
		reason,
	)
}

// NewOAuth2ScopeMatcher composes aggregate-action and logical-resource
// matching while preserving exact matching for arbitrary actions.
func NewOAuth2ScopeMatcher(matchAction ScopeActionMatcher, matchResource ScopeResourceMatcher) ScopeMatcher {
	if matchAction == nil {
		matchAction = DefaultOAuth2ScopeActionMatcher
	}
	if matchResource == nil {
		matchResource = DefaultOAuth2ScopeResourceMatcher
	}
	return func(scope string, operation authz.Operation) bool {
		return matchOAuth2Scope(scope, operation, matchAction, matchResource)
	}
}

// DefaultOAuth2ScopeMatcher matches an action:resource scope against request
// attributes using the default action and final-resource conventions.
func DefaultOAuth2ScopeMatcher(scope string, operation authz.Operation) bool {
	return matchOAuth2Scope(
		scope,
		operation,
		DefaultOAuth2ScopeActionMatcher,
		DefaultOAuth2ScopeResourceMatcher,
	)
}

// DefaultOAuth2ScopeActionMatcher lets read cover get and list and lets write
// cover every other non-empty request action.
func DefaultOAuth2ScopeActionMatcher(grantedAction string, operation authz.Operation) bool {
	switch grantedAction {
	case "read":
		return operation.Action == "get" || operation.Action == "list"
	case "write":
		return operation.Action != "" && operation.Action != "get" && operation.Action != "list"
	default:
		return false
	}
}

// SafeMethodOAuth2ScopeActionMatcher lets read cover GET and HEAD and lets
// write cover every other non-empty HTTP method. Requests without a method use
// the default action convention.
func SafeMethodOAuth2ScopeActionMatcher(grantedAction string, operation authz.Operation) bool {
	method := operation.Context[authorizationContextHTTPMethod]
	if method == "" {
		return DefaultOAuth2ScopeActionMatcher(grantedAction, operation)
	}
	if method == http.MethodGet || method == http.MethodHead {
		return grantedAction == "read"
	}
	return grantedAction == "write"
}

// DefaultOAuth2ScopeResourceMatcher matches only the final request resource.
func DefaultOAuth2ScopeResourceMatcher(grantedResource string, operation authz.Operation) bool {
	return grantedResource == operation.Resource.Type
}

func matchOAuth2Scope(
	scope string,
	operation authz.Operation,
	matchAction ScopeActionMatcher,
	matchResource ScopeResourceMatcher,
) bool {
	grantedAction, grantedResource, ok := strings.Cut(scope, ":")
	if !ok || grantedAction == "" || grantedResource == "" {
		return false
	}
	if grantedAction != operation.Action && !matchAction(grantedAction, operation) {
		return false
	}
	return matchResource(grantedResource, operation)
}

var _ authz.Authorizer = OAuth2ScopeAuthorizer{}
