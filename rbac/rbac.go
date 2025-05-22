package rbac

import (
	"context"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/events"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

type UserInfoGetter interface {
	GetUserProfile(ctx context.Context, username string) (authn.UserProfile, error)
}

func NewAPI(storage store.Store, users UserInfoGetter, events events.Recorder) *API {
	return &API{Storage: storage, UserProvider: users, Recorder: events}
}

type API struct {
	Storage      store.Store
	UserProvider UserInfoGetter
	Recorder     events.Recorder
}

func NewRBACAPIGroup(storage store.Store, users UserInfoGetter, recorder events.Recorder, scopePathVarNames ...api.ScopeVar) api.Group {
	return ScopedRbacAPI{
		Storage:           storage,
		UserProvider:      users,
		Recorder:          recorder,
		ScopePathVarNames: scopePathVarNames,
	}.Group()
}

type ScopedRbacAPI struct {
	Storage           store.Store
	ScopePathVarNames []api.ScopeVar
	UserProvider      UserInfoGetter
	Recorder          events.Recorder
}

func (a ScopedRbacAPI) Group() api.Group {
	prefix := ""
	for _, val := range a.ScopePathVarNames {
		prefix += "/" + val.Resource + "/{" + val.PathVarName + "}"
	}
	return api.
		NewGroup(prefix).
		SubGroup(
			a.rolesGroup(),
			a.userGroupsGroup(),
			a.UserRolesGroup(),
		)
}

func (a *API) Group() api.Group {
	adminscopedapi := &ScopedRbacAPI{
		UserProvider: a.UserProvider,
		Recorder:     a.Recorder,
	}
	return api.
		NewGroup("").
		Tag("RBAC").
		SubGroup(
			a.currentGroup(),
			// global rbac only has roles
			adminscopedapi.rolesGroup(),
			// admin userroles
			adminscopedapi.CustomUserRolesGroup("/userroles"),

			NewRBACAPIGroup(a.Storage, a.UserProvider, a.Recorder,
				api.ScopeVar{Resource: "tenants", PathVarName: "tenant"}),
			NewRBACAPIGroup(a.Storage, a.UserProvider, a.Recorder,
				api.ScopeVar{Resource: "tenants", PathVarName: "tenant"},
				api.ScopeVar{Resource: "organizations", PathVarName: "organization"}),
		)
}
