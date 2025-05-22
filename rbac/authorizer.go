package rbac

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/fields"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
	"xiaoshiai.cn/common/wildcard"
)

var _ api.Authorizer = &RBACAuthorizer{}

func NewRBACAuthorizer(storage store.Store) *RBACAuthorizer {
	return &RBACAuthorizer{Storage: storage}
}

type RBACAuthorizer struct {
	Storage store.Store
}

// Authorize implements api.Authorizer.
func (r *RBACAuthorizer) Authorize(ctx context.Context, user api.UserInfo, attr api.Attributes) (authorized api.Decision, reason string, err error) {
	ok, err := HasAutorityOnce(ctx, r.Storage, user.Name, attr)
	if err != nil || !ok {
		return api.DecisionDeny, "", err
	}
	return api.DecisionAllow, "", nil
}

// IsResourceInOrIsScope check the resources's scope <= scopes
// tenants/default , tenants/default -> true
// tenants/default , tenants/other -> false
// tenants/default/organizations/default , tenants/default -> true
// tenants/default/organizations/default , tenants/default/organizations/default -> true
func IsResourceInOrIsScope(resources []api.AttrbuteResource, scopes []store.Scope) bool {
	for i, scope := range scopes {
		if i >= len(resources) {
			return false
		}
		if scope.Resource != resources[i].Resource || scope.Name != resources[i].Name {
			return false
		}
	}
	return true
}

func IsSameScopes(a []api.AttrbuteResource, b []store.Scope) bool {
	if len(a) != len(b) {
		return false
	}
	for i, scope := range a {
		if scope.Resource != b[i].Resource || scope.Name != b[i].Name {
			return false
		}
	}
	return true
}

func HasAutorityOnce(ctx context.Context, storage store.Store, username string, attr api.Attributes) (bool, error) {
	list := &store.List[UserRole]{}

	options := []store.ListOption{
		store.WithSubScopes(),
		store.WithFieldRequirementsFromSet(fields.Set{"name": username}),
	}
	if err := storage.List(ctx, list, options...); err != nil {
		return false, err
	}
	act, expr := attr.Action, api.ResourcesToWildcard(attr.Resources)
	for _, userrole := range list.Items {
		// the resources is not covered by the userrole's scope, it must not have the authority
		if !IsResourceInOrIsScope(attr.Resources, userrole.Scopes) {
			continue
		}
		// user has role under this scope, and user only want to access this resource
		// eg.

		// tenant user has access to get tenant info
		// attr = {"resources": [{"resource": "tenants", "name": "default"}]}
		// userrole.Scopes = [{"resource": "tenants", "name": "default"}]
		//  -> user has access to get tenant info

		// eg. organization user has access to "get tenant" -> "get organization" implicitly
		if len(attr.Resources) > 0 && attr.Action == "get" && IsSameScopes(attr.Resources, userrole.Scopes) {
			return true, nil
		}
		for _, rolename := range userrole.Roles {
			scopedrole := &Role{}
			userrolescoped := storage.Scope(userrole.Scopes...)
			if err := userrolescoped.Get(ctx, rolename, scopedrole); err != nil {
				if !errors.IsNotFound(err) {
					return false, err
				}
				continue
			}
			// scope as authority
			if ScopedAuthorityMatch(userrole.Scopes, scopedrole.Authorities, act, expr) {
				return true, nil
			}
		}
	}
	return false, nil
}

func ScopedAuthorityMatch(scopes []store.Scope, authorities []Authority, act, expr string) bool {
	prefix := ""
	for _, scope := range scopes {
		prefix += scope.Resource + ":" + scope.Name + ":"
	}
	for _, item := range authorities {
		if !slices.ContainsFunc(item.Actions, func(item string) bool { return item == "*" || item == act }) {
			return false
		}
		if slices.ContainsFunc(item.Resources, func(item string) bool { return wildcard.Match(prefix+item, expr) }) {
			return true
		}
	}
	return false
}
