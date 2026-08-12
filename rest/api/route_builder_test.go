package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupBuildNormalizesRoutePath(t *testing.T) {
	tests := []struct {
		name  string
		group Group
		want  string
	}{
		{
			name: "relative group path",
			group: NewGroup("internal").
				Route(POST("/authorize")),
			want: "/internal/authorize",
		},
		{
			name: "relative route path",
			group: NewGroup("").
				Route(POST("internal/authorize")),
			want: "/internal/authorize",
		},
		{
			name: "root path",
			group: NewGroup("").
				Route(GET("")),
			want: "/",
		},
		{
			name: "nested relative group path",
			group: NewGroup("internal").
				SubGroup(NewGroup("oauth").Route(POST("/authorize"))),
			want: "/internal/oauth/authorize",
		},
		{
			name: "absolute path remains unchanged",
			group: NewGroup("/internal").
				Route(POST("/authorize")),
			want: "/internal/authorize",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := tt.group.Build()
			require.Len(t, routes, 1)
			require.Equal(t, tt.want, routes[0].Path)
		})
	}
}
