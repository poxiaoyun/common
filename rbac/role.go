package rbac

import (
	"context"
	"net/http"

	"xiaoshiai.cn/common/base"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type Role struct {
	store.ObjectMeta `json:",inline"`
	Hidden           bool        `json:"hidden,omitempty"` // hidden role will not be listed
	Authorities      []Authority `json:"authorities,omitempty"`
}

type Authority = api.Authority

type UserRole struct {
	store.ObjectMeta `json:",inline"`
	Roles            []string `json:"roles,omitempty"`
}

func (a *ScopedRbacAPI) ListRoles(w http.ResponseWriter, r *http.Request) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		return base.GenericListWithWatch(w, r, a.Storage.Scope(scopes...), &store.List[Role]{})
	})
}

func (a *ScopedRbacAPI) CreateRole(w http.ResponseWriter, r *http.Request) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		return base.GenericCreate(r, a.Storage.Scope(scopes...), &Role{})
	})
}

func (a *ScopedRbacAPI) GetRole(w http.ResponseWriter, r *http.Request) {
	a.OnRole(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericGet(r, storage, &Role{}, name)
	})
}

func (a *ScopedRbacAPI) UpdateRole(w http.ResponseWriter, r *http.Request) {
	a.OnRole(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericUpdate(r, storage, &Role{}, name)
	})
}

func (a *ScopedRbacAPI) PatchRole(w http.ResponseWriter, r *http.Request) {
	a.OnRole(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericPatch(r, storage, &Role{}, name)
	})
}

func (a *ScopedRbacAPI) DeleteRole(w http.ResponseWriter, r *http.Request) {
	a.OnRole(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericDelete(r, storage, &Role{}, name)
	})
}

func (a *ScopedRbacAPI) OnRole(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, storage store.Store, role string) (any, error)) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		role := api.Path(r, "role", "")
		if role == "" {
			return nil, errors.NewBadRequest("Role name is required")
		}
		return fn(ctx, a.Storage.Scope(scopes...), role)
	})
}

func (a *ScopedRbacAPI) rolesGroup() api.Group {
	return api.
		NewGroup("/roles").
		Route(
			api.GET("").To(a.ListRoles).Doc("List roles").
				Param(api.PageParams...).Response(store.List[Role]{}),
			api.POST("").To(a.CreateRole).Doc("Create role").
				Param(api.BodyParam("role", Role{})).Response(Role{}),
			api.GET("/{role}").Doc("Get role").
				To(a.GetRole).Response(Role{}),
			api.PUT("/{role}").Doc("Update role").
				To(a.UpdateRole).Param(api.BodyParam("role", Role{})).Response(Role{}),
			api.PATCH("/{role}").Doc("Patch role").
				To(a.PatchRole).Param(api.BodyParam("role", Role{})).Response(Role{}),
			api.DELETE("/{role}").Doc("Delete role").
				To(a.DeleteRole),
		)
}
