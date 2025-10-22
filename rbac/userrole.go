package rbac

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/base"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/events"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

const LastAdminCheck = false

type ScopedUserRole struct {
	User              string             `json:"user"`
	Role              string             `json:"role"`
	Roles             []string           `json:"roles,omitempty"`
	CreationTimestamp meta.Time          `json:"creationTimestamp"`
	DeleteTimestamp   *meta.Time        `json:"deleteTimestamp"`
	UserInfo          *authn.UserProfile `json:"userInfo"`
}

func (a *ScopedRbacAPI) ListUserRole(w http.ResponseWriter, r *http.Request) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		userRoleList, err := base.GenericList(r, a.Storage.Scope(scopes...), &store.List[UserRole]{})
		if err != nil {
			return nil, err
		}
		userroles := make([]ScopedUserRole, 0, len(userRoleList.Items))
		for _, rb := range userRoleList.Items {
			tuser := ScopedUserRole{
				User:              rb.Name,
				Role:              FirstOrEmpty(rb.Roles),
				Roles:             rb.Roles,
				CreationTimestamp: rb.CreationTimestamp,
				DeleteTimestamp:   rb.DeletionTimestamp,
			}
			if a.UserProvider != nil {
				userInfo, err := a.UserProvider.GetUserProfile(ctx, rb.Name)
				if err != nil {
					log.FromContext(ctx).Error(err, "Failed to get user profile")
				} else {
					tuser.UserInfo = &authn.UserProfile{User: userInfo.User}
				}
			}
			userroles = append(userroles, tuser)
		}
		return store.List[ScopedUserRole]{
			Items: userroles,
			Total: userRoleList.Total,
			Page:  userRoleList.Page,
			Size:  userRoleList.Size,
		}, nil
	})
}

func FirstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

func (a *ScopedRbacAPI) SetUserRole(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, storage store.Store, user string) (any, error) {
		setRole := &ScopedUserRole{}
		if err := api.Body(r, setRole); err != nil {
			return nil, err
		}
		if setRole.Role == "" {
			return nil, errors.NewBadRequest("Role is required")
		}
		hasExists := true
		exists := &UserRole{}
		if err := storage.Get(ctx, user, exists); err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
			hasExists = false
		}
		// from 'admin' to other role
		if LastAdminCheck && slices.Contains(exists.Roles, RoleAdmin) && setRole.Role != RoleAdmin {
			// check must have at least one 'admin' role user
			roleList := &store.List[UserRole]{}
			if err := storage.List(ctx, roleList); err != nil {
				return nil, err
			}
			adminCount := 0
			for _, role := range roleList.Items {
				if role.Name == user || !slices.Contains(role.Roles, RoleAdmin) {
					continue
				}
				adminCount++
			}
			if adminCount == 0 {
				return nil, errors.NewBadRequest(fmt.Sprintf("user %s is the last admin user", user))
			}
		}
		if err := AddUserToScope(ctx, storage, nil, user, setRole.Role); err != nil {
			return nil, err
		}
		if !hasExists {
			scopes := make([]store.Scope, 0, len(a.ScopePathVarNames))
			for _, val := range a.ScopePathVarNames {
				value := api.Path(r, val.PathVarName, "")
				if value == "" {
					return nil, errors.NewBadRequest(val.PathVarName + " is required")
				}
				scopes = append(scopes, store.Scope{Resource: val.Resource, Name: value})
			}
			exists = &UserRole{
				ObjectMeta: store.ObjectMeta{
					Name:     user,
					Scopes:   scopes,
					Resource: "userroles",
				},
			}
		}
		a.Recorder.EventNoAggregate(ctx, exists, events.UserPermissionsRefresh, "Role Changed")
		return nil, nil
	})
}

func SetUserRole(ctx context.Context, storage store.Store, user, role string) error {
	roleObj := &UserRole{ObjectMeta: store.ObjectMeta{Name: user}}
	updatefunc := func() error {
		roleObj.Roles = []string{role}
		return nil
	}
	return store.CreateOrUpdate(ctx, storage, roleObj, updatefunc)
}

func AddUserToScope(ctx context.Context, storage store.Store, scopes []store.Scope, user, role string) error {
	return SetUserRole(ctx, storage.Scope(scopes...), user, role)
}

func (a *ScopedRbacAPI) RemoveUserRole(w http.ResponseWriter, r *http.Request) {
	a.OnUser(w, r, func(ctx context.Context, storage store.Store, user string) (any, error) {
		exists := &UserRole{}
		if err := storage.Get(ctx, user, exists); err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
		}
		// check the scope must have at least one 'admin' role user
		if LastAdminCheck && slices.Contains(exists.Roles, RoleAdmin) {
			roleList := &store.List[UserRole]{}
			if err := storage.List(ctx, roleList); err != nil {
				return nil, err
			}
			adminCount := 0
			for _, role := range roleList.Items {
				if role.Name == user || !slices.Contains(role.Roles, RoleAdmin) {
					continue
				}
				adminCount++
			}
			if adminCount == 0 {
				return nil, errors.NewBadRequest(fmt.Sprintf("user %s is the last admin user", user))
			}
		}
		role := &UserRole{}
		if err := storage.Get(ctx, user, role); err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
		}
		if err := store.IgnoreNotFound(storage.Delete(ctx, role)); err != nil {
			return nil, err
		}
		a.Recorder.EventNoAggregate(ctx, role, events.UserPermissionsRefresh, "User Role Changed")
		return nil, nil
	})
}

func (a *ScopedRbacAPI) OnUser(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, storage store.Store, user string) (any, error)) {
	api.OnScope(w, r, a.ScopePathVarNames, func(ctx context.Context, scopes []store.Scope) (any, error) {
		user := api.Path(r, "user", "")
		if user == "" {
			return nil, errors.NewBadRequest("User name is required")
		}
		return fn(ctx, a.Storage.Scope(scopes...), user)
	})
}

func (a *ScopedRbacAPI) UserRolesGroup() api.Group {
	return a.CustomUserRolesGroup("/users")
}

func (a *ScopedRbacAPI) CustomUserRolesGroup(prefix string) api.Group {
	return api.
		NewGroup(prefix).
		Route(
			api.GET("").
				Doc("List users").
				To(a.ListUserRole),
			api.PUT("/{user}").
				Doc("Add user to scope").
				To(a.SetUserRole).
				Param(
					api.PathParam("user", "User name"),
					api.BodyParam("role", ScopedUserRole{}),
				),
			api.DELETE("/{user}").
				Doc("Delete user from scope").
				To(a.RemoveUserRole),
		)
}
