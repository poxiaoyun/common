package api

import (
	"context"
)

const (
	SystemAdminGroup = "system:admin"
	SystemBanedGroup = "system:baned"
)

// Authorize implements Authorizer.
// it containers list of predefined authorities not in rtbac
// example:
/*
	node := []CustomAuthorizeNode{
		{
			Actions:  []string{"get", "list"},
			Resource: []string{"avatars", "user-search"},
		},
		{
			Actions:  []string{"* "},
			Resource: []string{"current"},
		},
		{
			Actions:  []string{"create"},
			Resource: []string{"tenants:*:users"},
		},
	}
*/
func NewCustomAuthorizer(nodes []CustomAuthorizeNode) Authorizer {
	return AuthorizerFunc(func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
		return customAuthorizerCheck(ctx, nodes, user, a)
	})
}

type CustomAuthorizeNode struct {
	// Authorizer is a custom function to authorize on this node
	// default is Allow
	Authorizer Authorizer
	Actions    []string
	Resource   []string // resource is a list of resource wildcard expression
}

func customAuthorizerCheck(ctx context.Context, checks []CustomAuthorizeNode, user UserInfo, attr Attributes) (authorized Decision, reason string, err error) {
	if len(attr.Resources) == 0 {
		return DecisionNoOpinion, "", nil
	}
	for _, allow := range checks {
		if !(Authority{Actions: allow.Actions, Resources: allow.Resource}).MatchAttributes(attr) {
			continue
		}
		if allow.Authorizer != nil {
			return allow.Authorizer.Authorize(ctx, user, attr)
		}
		return DecisionAllow, "", nil
	}
	return DecisionNoOpinion, "", nil
}
