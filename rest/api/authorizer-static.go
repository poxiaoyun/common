package api

import "context"

// StaticAuthorizationRule matches actions and resource paths. A matching rule
// delegates to Authorizer when set and otherwise allows the request.
type StaticAuthorizationRule struct {
	Authorizer Authorizer
	Actions    []string
	Resources  []string
}

// StaticAuthorizer evaluates a fixed, ordered set of authorization rules.
type StaticAuthorizer struct {
	Rules []StaticAuthorizationRule
}

var _ Authorizer = (*StaticAuthorizer)(nil)

// NewStaticAuthorizer returns an Authorizer backed by a fixed rule set.
func NewStaticAuthorizer(rules []StaticAuthorizationRule) *StaticAuthorizer {
	return &StaticAuthorizer{Rules: rules}
}

// Authorize returns the first matching rule's decision, or DecisionNoOpinion
// when the target contains no resources or no rule matches.
func (authorizer *StaticAuthorizer) Authorize(ctx context.Context, authentication AuthenticationInfo, attributes Attributes) (Decision, string, error) {
	if len(attributes.Resources) == 0 {
		return DecisionNoOpinion, "", nil
	}
	for _, rule := range authorizer.Rules {
		if !(Authority{Actions: rule.Actions, Resources: rule.Resources}).MatchAttributes(attributes) {
			continue
		}
		if rule.Authorizer != nil {
			return rule.Authorizer.Authorize(ctx, authentication, attributes)
		}
		return DecisionAllow, "", nil
	}
	return DecisionNoOpinion, "", nil
}
