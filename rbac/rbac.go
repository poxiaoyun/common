package rbac

import (
	"context"

	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/events"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

// SubjectGetter resolves canonical subject display data from its stable
// reference.
type SubjectGetter interface {
	// GetSubject returns the subject identified by reference.
	GetSubject(ctx context.Context, reference authn.SubjectReference) (authn.Subject, error)
}

func NewAPI(storage store.Store, subjects SubjectGetter, events events.Recorder) *API {
	return &API{Storage: storage, SubjectGetter: subjects, Recorder: events}
}

type API struct {
	Storage       store.Store
	SubjectGetter SubjectGetter
	Recorder      events.Recorder
}

func NewRBACAPIGroup(storage store.Store, subjects SubjectGetter, recorder events.Recorder, scopePathVarNames ...api.ScopeVar) api.Group {
	return ScopedRbacAPI{
		Storage:           storage,
		SubjectGetter:     subjects,
		Recorder:          recorder,
		ScopePathVarNames: scopePathVarNames,
	}.Group()
}

type ScopedRbacAPI struct {
	Storage           store.Store
	ScopePathVarNames []api.ScopeVar
	SubjectGetter     SubjectGetter
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
		SubjectGetter: a.SubjectGetter,
		Recorder:      a.Recorder,
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

			NewRBACAPIGroup(a.Storage, a.SubjectGetter, a.Recorder,
				api.ScopeVar{Resource: "tenants", PathVarName: "tenant"}),
			NewRBACAPIGroup(a.Storage, a.SubjectGetter, a.Recorder,
				api.ScopeVar{Resource: "tenants", PathVarName: "tenant"},
				api.ScopeVar{Resource: "organizations", PathVarName: "organization"}),
		)
}
