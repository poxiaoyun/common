package rbac

import (
	"context"
	"net/http"

	"xiaoshiai.cn/common/base"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

type UserGroup struct {
	store.ObjectMeta `json:",inline"`
	Tenant           string   `json:"tenant,omitempty"`
	Users            []string `json:"users,omitempty"`
}

func (a *ScopedRbacAPI) ListUserGroups(w http.ResponseWriter, r *http.Request) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		return base.GenericListWithWatch(w, r, a.Storage.Scope(scopes...), &store.List[UserGroup]{})
	})
}

func (a *ScopedRbacAPI) GetUserGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroup(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericGet(r, storage, &UserGroup{}, name)
	})
}

func (a *ScopedRbacAPI) CreateUserGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroup(w, r, func(ctx context.Context, storage store.Store, tenant string) (any, error) {
		return base.GenericCreate(r, storage, &UserGroup{Tenant: tenant})
	})
}

func (a *ScopedRbacAPI) UpdateUserGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroup(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericUpdate(r, storage, &UserGroup{}, name)
	})
}

func (a *ScopedRbacAPI) PatchUserGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroup(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericPatch(r, storage, &UserGroup{}, name)
	})
}

func (a *ScopedRbacAPI) DeleteUserGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroup(w, r, func(ctx context.Context, storage store.Store, name string) (any, error) {
		return base.GenericDelete(r, storage, &UserGroup{}, name)
	})
}

func (a *ScopedRbacAPI) AddUserToGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroupUser(w, r, func(ctx context.Context, storage store.Store, tenant, name string) (any, error) {
		return nil, errors.NewNotImplemented("Not implemented")
	})
}

func (a *ScopedRbacAPI) RemoveUserFromGroup(w http.ResponseWriter, r *http.Request) {
	a.onUserGroupUser(w, r, func(ctx context.Context, storage store.Store, tenant, name string) (any, error) {
		return nil, errors.NewNotImplemented("Not implemented")
	})
}

func (a *ScopedRbacAPI) onUserGroupUser(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, storage store.Store, usergroup, user string) (any, error)) {
	a.onUserGroup(w, r, func(ctx context.Context, storage store.Store, usergroup string) (any, error) {
		user := api.Path(r, "user", "")
		if user == "" {
			return nil, errors.NewBadRequest("User name is required")
		}
		return fn(ctx, storage, usergroup, user)
	})
}

func (a *ScopedRbacAPI) onUserGroup(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, storage store.Store, usergroup string) (any, error)) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		userGroup := api.Path(r, "usergroup", "")
		if userGroup == "" {
			return nil, errors.NewBadRequest("UserGroup name is required")
		}
		return fn(ctx, a.Storage.Scope(scopes...), userGroup)
	})
}

func (a *ScopedRbacAPI) userGroupsGroup() api.Group {
	return api.NewGroup("/usergroups").
		Route(
			api.GET("").
				Doc("List usergroups").
				To(a.ListUserGroups).Param(api.PageParams...).Response(store.List[UserGroup]{}),
			api.POST("").
				Doc("Create usergroup").
				To(a.CreateUserGroup).Param(api.BodyParam("usergroup", UserGroup{})).Response(UserGroup{}),
			api.GET("/{usergroup}").
				Doc("Get usergroup").
				To(a.GetUserGroup).Response(UserGroup{}),
			api.PUT("/{usergroup}").
				Doc("Update usergroup").
				To(a.UpdateUserGroup).Param(api.BodyParam("usergroup", UserGroup{})).Response(UserGroup{}),
			api.PATCH("/{usergroup}").
				Doc("Patch usergroup").
				To(a.PatchUserGroup).Param(api.BodyParam("usergroup", UserGroup{})).Response(UserGroup{}),
			api.DELETE("/{usergroup}").
				Doc("Delete usergroup").
				To(a.DeleteUserGroup),
			// usergroup users
			api.POST("/{usergroup}/users/{user}").
				Doc("Add user to usergroup").
				To(a.AddUserToGroup),
			api.DELETE("/{usergroup}/users/{user}").
				Doc("Remove user from usergroup").
				To(a.RemoveUserFromGroup),
		)
}
