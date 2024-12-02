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
			Children: []CustomAuthorizeNode{
				{
					Actions:  []string{"*"},
					Resource: []string{"**"},
				},
			},
		},
		{
			Actions:  []string{"create"},
			Resource: []string{"tenants"},
			Children: []CustomAuthorizeNode{
				{
					Actions:    []string{"get", "list"},
					Resource:   []string{"applications"},
					Authorizer: custom.CheckSome,
				},
			},
		},
	}
*/
func NewCustomAuthorizer(nodes []CustomAuthorizeNode) Authorizer {
	return AuthorizerFunc(func(ctx context.Context, user UserInfo, a Attributes) (authorized Decision, reason string, err error) {
		return checkRecursive(ctx, nodes, a.Resources, user, a)
	})
}

type CustomAuthorizeNode struct {
	// Authorizer is a custom function to authorize on this node
	// default is Allow
	Authorizer Authorizer
	Actions    []string
	Resource   []string
	Children   []CustomAuthorizeNode
}

func checkRecursive(ctx context.Context, allows []CustomAuthorizeNode, resources []AttrbuteResource, user UserInfo, origin Attributes) (authorized Decision, reason string, err error) {
	if len(resources) == 0 {
		return DecisionNoOpinion, "", nil
	}
	thisresource, resources := resources[0], resources[1:]
	for _, allow := range allows {
		ok, matchall := matchResource(allow.Resource, thisresource.Resource)
		if !ok {
			continue
		}
		// start check when we have matched end of allow
		if len(allow.Children) == 0 {
			if !matchAction(allow.Actions, origin.Action) {
				continue
			}
			if len(resources) != 0 && matchall {
				continue
			}
			if allow.Authorizer != nil {
				return allow.Authorizer.Authorize(ctx, user, origin)
			}
			return DecisionAllow, "", nil
		}
		decision, msg, err := checkRecursive(ctx, allow.Children, resources, user, origin)
		if err != nil {
			return decision, msg, err
		}
		if decision == DecisionAllow {
			// check this level
			if allow.Authorizer != nil {
				return allow.Authorizer.Authorize(ctx, user, origin)
			}
			return decision, msg, err
		}
	}
	return DecisionNoOpinion, "", nil
}

func matchResource(allows []string, resource string) (match bool, matchall bool) {
	for _, allow := range allows {
		if allow == "**" {
			return true, true
		}
		if allow == "*" {
			return true, false
		}
		if resource == allow {
			return true, false
		}
	}
	return false, false
}

func matchAction(allows []string, action string) bool {
	for _, allow := range allows {
		if allow == "*" {
			return true
		}
		if action == allow {
			return true
		}
	}
	return false
}
