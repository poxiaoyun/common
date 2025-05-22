package rbac

import (
	"testing"

	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

func TestAuthorityMatch(t *testing.T) {
	tests := []struct {
		name      string
		scopes    []store.Scope
		authority []Authority
		attr      api.Attributes
		want      bool
	}{
		{
			authority: []Authority{{Actions: []string{"get", "list"}, Resources: []string{"**"}}},
			attr: api.Attributes{
				Action:    "get",
				Resources: []api.AttrbuteResource{{Resource: "namespaces", Name: "default"}},
			},
			want: true,
		},
		{
			authority: []Authority{{Actions: []string{"get", "list"}, Resources: []string{"applications:**"}}},
			attr: api.Attributes{
				Action:    "list",
				Resources: []api.AttrbuteResource{{Resource: "applications"}},
			},
			want: true,
		},
		{
			authority: []Authority{{Actions: []string{"*"}, Resources: []string{"applications:**"}}},
			attr: api.Attributes{
				Action:    "get",
				Resources: []api.AttrbuteResource{{Resource: "applications", Name: "default"}},
			},
			want: true,
		},
		{
			authority: []Authority{
				{Actions: []string{"*"}, Resources: []string{"applications:**"}},
			},
			attr: api.Attributes{
				Action: "get",
				Resources: []api.AttrbuteResource{
					{Resource: "applications", Name: "default"},
					{Resource: "resources", Name: "pods"},
				},
			},
			want: true,
		},
		{
			authority: []Authority{{Actions: []string{"*"}, Resources: []string{"applications:**"}}},
			attr: api.Attributes{
				Action:    "list",
				Resources: []api.AttrbuteResource{{Resource: "applications"}},
			},
			want: true,
		},
		{
			scopes: []store.Scope{
				{Resource: "tenants", Name: "default"},
			},
			authority: []Authority{
				{Actions: []string{"*"}, Resources: []string{"applications:**"}},
			},
			attr: api.Attributes{
				Action: "list",
				Resources: []api.AttrbuteResource{
					{Resource: "tenants", Name: "default"},
					{Resource: "applications"},
				},
			},
			want: true,
		},
		{
			scopes: []store.Scope{
				{Resource: "tenants", Name: "abc"},
			},
			authority: []Authority{
				{Actions: []string{"*"}, Resources: []string{"applications:**"}},
			},
			attr: api.Attributes{
				Action: "list",
				Resources: []api.AttrbuteResource{
					{Resource: "tenants", Name: "default"},
					{Resource: "applications"},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act, expr := tt.attr.Action, api.ResourcesToWildcard(tt.attr.Resources)
			if got := ScopedAuthorityMatch(tt.scopes, tt.authority, act, expr); got != tt.want {
				t.Errorf("AuthorityMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
