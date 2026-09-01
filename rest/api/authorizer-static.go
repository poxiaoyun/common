package api

import (
	"context"

	"xiaoshiai.cn/common/authz"
)

// StaticAuthorizationRule matches actions and resource paths. A matching rule
// delegates to Authorizer when set and otherwise allows the request.
type StaticAuthorizationRule struct {
	Authorizer authz.Authorizer
	Actions    []string
	Resources  []string
}

// StaticAuthorizer evaluates a fixed, ordered set of authorization rules.
type StaticAuthorizer struct {
	Rules []StaticAuthorizationRule
}

var _ authz.Authorizer = (*StaticAuthorizer)(nil)

// NewStaticAuthorizer returns an Authorizer backed by a fixed rule set.
func NewStaticAuthorizer(rules []StaticAuthorizationRule) *StaticAuthorizer {
	return &StaticAuthorizer{Rules: rules}
}

// Authorize returns the first matching rule's decision, or NoOpinion
// when the target contains no resources or no rule matches.
func (authorizer *StaticAuthorizer) Authorize(ctx context.Context, authentication Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	if operation.Resource.Type == "" {
		return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, nil
	}
	for _, rule := range authorizer.Rules {
		permission := authz.Permission{Service: operation.Service, Actions: rule.Actions, Resources: rule.Resources}
		if !authz.MatchPermission(permission, operation) {
			continue
		}
		if rule.Authorizer != nil {
			return rule.Authorizer.Authorize(ctx, authentication, operation)
		}
		return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
	}
	return authz.EvaluationResult{Decision: authz.DecisionNoOpinion}, nil
}
