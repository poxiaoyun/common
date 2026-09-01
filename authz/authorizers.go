package authz

import (
	"context"
	"slices"

	"xiaoshiai.cn/common/authn"
)

// AuthorizerFunc adapts a function to Authorizer.
type AuthorizerFunc func(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error)

// Authorize calls f.
func (f AuthorizerFunc) Authorize(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error) {
	return f(ctx, authentication, operation)
}

// NewAlwaysAllowAuthorizer returns an Authorizer that allows every operation.
func NewAlwaysAllowAuthorizer() AuthorizerFunc {
	return func(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error) {
		return EvaluationResult{Decision: DecisionAllow}, nil
	}
}

// NewAlwaysDenyAuthorizer returns an Authorizer that denies every operation.
func NewAlwaysDenyAuthorizer() AuthorizerFunc {
	return func(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error) {
		return EvaluationResult{Decision: DecisionDeny}, nil
	}
}

// AuthorizerChain evaluates Authorizers in order. Allow, Deny, and errors stop
// evaluation; NoOpinion continues. An all-NoOpinion chain denies.
type AuthorizerChain []Authorizer

// Authorize evaluates the chain according to its ordering and short-circuit
// rules.
func (chain AuthorizerChain) Authorize(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error) {
	for _, authorizer := range chain {
		result, err := authorizer.Authorize(ctx, authentication, operation)
		if err != nil {
			result.Decision = DecisionDeny
			return result, err
		}
		switch result.Decision {
		case DecisionAllow, DecisionDeny:
			return result, nil
		case DecisionNoOpinion:
			continue
		default:
			result.Decision = DecisionDeny
			return result, nil
		}
	}
	return EvaluationResult{Decision: DecisionDeny, Reason: "no decision"}, nil
}

// NewGroupAuthorizer returns an Authorizer that denies members of deny, allows
// members of allow, and otherwise returns NoOpinion.
func NewGroupAuthorizer(allow, deny []string) GroupAuthorizer {
	return GroupAuthorizer{AllowedGroups: allow, DeniedGroups: deny}
}

// GroupAuthorizer authorizes operations by authenticated group membership.
// The first authenticated group in either configured set decides; a group in
// both sets is denied.
type GroupAuthorizer struct {
	AllowedGroups []string
	DeniedGroups  []string
}

// Authorize checks the authenticated Subject's groups.
func (authorizer GroupAuthorizer) Authorize(ctx context.Context, authentication authn.Authentication, operation Operation) (EvaluationResult, error) {
	for _, group := range authentication.Groups {
		if slices.Contains(authorizer.DeniedGroups, group) {
			return EvaluationResult{Decision: DecisionDeny}, nil
		}
		if slices.Contains(authorizer.AllowedGroups, group) {
			return EvaluationResult{Decision: DecisionAllow}, nil
		}
	}
	return EvaluationResult{Decision: DecisionNoOpinion}, nil
}
