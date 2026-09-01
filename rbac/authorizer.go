package rbac

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

var _ authz.Authorizer = &RBACAuthorizer{}

func NewRBACAuthorizer(storage store.Store) *RBACAuthorizer {
	return &RBACAuthorizer{Storage: storage}
}

type RBACAuthorizer struct {
	Storage store.Store
}

// Authorize implements authz.Authorizer.
func (r *RBACAuthorizer) Authorize(ctx context.Context, authentication authn.Authentication, operation authz.Operation) (authz.EvaluationResult, error) {
	ok, err := HasAuthority(ctx, r.Storage, authentication.ID, operation)
	if err != nil || !ok {
		return authz.EvaluationResult{Decision: authz.DecisionDeny}, err
	}
	return authz.EvaluationResult{Decision: authz.DecisionAllow}, nil
}

// IsResourceInOrIsScope check the resources's scope <= scopes
// tenants/default , tenants/default -> true
// tenants/default , tenants/other -> false
// tenants/default/organizations/default , tenants/default -> true
// tenants/default/organizations/default , tenants/default/organizations/default -> true
func IsResourceInOrIsScope(resource authz.Resource, scopes []store.Scope) bool {
	for i, scope := range scopes {
		if i < len(resource.Scope) {
			if scope.Resource != resource.Scope[i].Type || scope.Name != resource.Scope[i].ID {
				return false
			}
			continue
		}
		if i > len(resource.Scope) {
			return false
		}
		if scope.Resource != resource.Type || scope.Name != resource.ID {
			return false
		}
	}
	return true
}

func IsSameScopes(a authz.Scope, b []store.Scope) bool {
	if len(a) != len(b) {
		return false
	}
	for i, scope := range a {
		if scope.Type != b[i].Resource || scope.ID != b[i].Name {
			return false
		}
	}
	return true
}

// HasAuthority reports whether the stable Subject ID has one applicable role
// permission for attributes.
func HasAuthority(ctx context.Context, storage store.Store, subjectID string, operation authz.Operation) (bool, error) {
	list := &store.List[UserRole]{}

	options := []store.ListOption{
		store.WithSubScopes(),
		store.WithFieldRequirementsFromSet(fields.Set{"name": subjectID}),
	}
	if err := storage.List(ctx, list, options...); err != nil {
		return false, err
	}
	for _, userrole := range list.Items {
		// the resources is not covered by the userrole's scope, it must not have the authority
		if !IsResourceInOrIsScope(operation.Resource, userrole.Scopes) {
			continue
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
			if ScopedPermissionMatch(userrole.Scopes, scopedrole.Authorities, operation) {
				return true, nil
			}
		}
	}
	return false, nil
}

// ScopedPermissionMatch reports whether a permission applies to attributes
// below the supplied role-binding scope.
func ScopedPermissionMatch(scopes []store.Scope, permissions []authz.Permission, operation authz.Operation) bool {
	prefix := ""
	for _, scope := range scopes {
		prefix += scope.Resource + ":" + scope.Name + ":"
	}
	for _, permission := range permissions {
		scoped := permission
		scoped.Resources = make([]string, len(permission.Resources))
		for index, resource := range permission.Resources {
			scoped.Resources[index] = prefix + resource
		}
		if authz.MatchPermission(scoped, operation) {
			return true
		}
	}
	return false
}
