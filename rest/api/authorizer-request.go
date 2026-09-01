package api

import (
	"context"
	"net/http"
	"regexp"
	"slices"

	"xiaoshiai.cn/common/authz"
)

// RequestAuthorizerFunc adapts a function to RequestAuthorizer.
type RequestAuthorizerFunc func(r *http.Request) (authz.EvaluationResult, error)

// AuthorizeRequest calls f.
func (f RequestAuthorizerFunc) AuthorizeRequest(r *http.Request) (authz.EvaluationResult, error) {
	return f(r)
}

// NewWhitelistAuthorizer creates an operation authorizer that allows paths
// matching any supplied regular expression and otherwise returns NoOpinion.
func NewWhitelistAuthorizer(pattern ...string) authz.AuthorizerFunc {
	compiledPatterns := make([]*regexp.Regexp, 0, len(pattern))
	for _, pattern := range pattern {
		compiledPatterns = append(compiledPatterns, regexp.MustCompile(pattern))
	}
	return func(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
		matched := slices.ContainsFunc(compiledPatterns, func(r *regexp.Regexp) bool {
			return r.MatchString(operation.Context[authorizationContextHTTPPath])
		})
		if matched {
			return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
		}
		return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, nil
	}
}
