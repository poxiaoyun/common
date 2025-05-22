package rbac

import (
	"context"
	"net/http"

	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

type UserRoleView struct {
	UserRole     `json:",inline"`
	Role         string `json:"role"`
	Tenant       string `json:"tenant"`
	Organization string `json:"organization"`
}

func (a *API) CurrentRoles(w http.ResponseWriter, r *http.Request) {
	api.OnCurrentUser(w, r, func(ctx context.Context, username string) (any, error) {
		list := store.List[UserRole]{}
		options := []store.ListOption{
			store.WithSubScopes(),
			store.WithFieldRequirementsFromSet(map[string]string{"name": username}),
		}
		if err := a.Storage.List(ctx, &list, options...); err != nil {
			return nil, err
		}
		items := make([]UserRoleView, 0, len(list.Items))
		for _, item := range list.Items {
			roleview := UserRoleView{UserRole: item}
			if len(item.Roles) != 0 {
				roleview.Role = item.Roles[0]
			}
			for _, scope := range item.Scopes {
				if scope.Resource == "tenants" {
					roleview.Tenant = scope.Name
				}
				if scope.Resource == "organizations" {
					roleview.Organization = scope.Name
				}
			}
			items = append(items, roleview)
		}
		return items, nil
	})
}

func (a *API) currentGroup() api.Group {
	return api.
		NewGroup("/current").
		Route(
			api.GET("/roles").
				To(a.CurrentRoles).
				Response(
					[]UserRole{}, "all roles",
				),
		)
}
