package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestGroupPreservesPathMatchingSemantics(t *testing.T) {
	group := api.NewGroup("/files").
		Route(
			api.GET("").
				To(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "root") }),
			api.GET("/{$}").
				To(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "directory") }),
			api.GET("/{name}").
				Accept("application/json").
				To(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.WriteString(w, api.PathVars(r).
						Get("name"))
				}),
			api.GET("/").
				To(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "fallback") }),
		)
	handler := api.New().
		Group(group).
		Build()
	for _, test := range []struct{ path, accept, want string }{
		{"/files", "", "root"},
		{"/files/", "", "directory"},
		{"/files/a%2Fb", "application/json", "a/b"},
		{"/files/%252F", "application/json", "%2F"},
		{"/files/a%2Fb", "text/plain", "fallback"},
		{"/files/a/b", "application/json", "fallback"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Accept", test.accept)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, test.path)
		require.Equal(t, test.want, response.Body.String(), test.path)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/files-other/item", nil))
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestGroupMediaConditionsIntersectAllAncestors(t *testing.T) {
	group := api.NewGroup("/api").
		ContentType("application/*").
		Accept("application/json", "application/xml").
		SubGroup(api.NewGroup("/nested").
			ContentType("application/json", "text/plain").
			SubGroup(api.NewGroup("/deep").
				Route(api.POST("/resource").
					ContentType("application/json;charset=UTF-8").
					Accept("application/json").
					To(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusNoContent)
					}))))
	mux := api.NewMux()
	routes := group.Build()
	require.Len(t, routes, 1)
	require.NoError(t, mux.Register(&routes[0]))
	// Documentation receives the same effective media conditions as routing.
	require.Equal(t, []string{"application/json; charset=utf-8"}, routes[0].ContentTypes)
	require.Equal(t, []string{"application/json"}, routes[0].Accepts)
	for _, test := range []struct {
		contentType, accept string
		status              int
	}{
		{"application/json;charset=utf-8", "application/json", http.StatusNoContent},
		{"application/json;charset=iso-8859-1", "application/json", http.StatusNotFound},
		{"text/plain;charset=utf-8", "application/json", http.StatusNotFound},
		{"application/json;charset=utf-8", "application/xml", http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/nested/deep/resource", nil)
		request.Header.Set("Content-Type", test.contentType)
		request.Header.Set("Accept", test.accept)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, test.status, response.Code)
	}
}

func TestGroupMediaConflictFailsRegistration(t *testing.T) {
	for _, group := range []api.Group{
		api.NewGroup("/api").
			ContentType("application/json").
			SubGroup(api.NewGroup("/nested").
				ContentType("application/xml").
				SubGroup(api.NewGroup("/deep").
					Route(api.POST("/resource")))),
		api.NewGroup("/api").
			Accept("application/json").
			Route(api.GET("/resource").
				Accept("application/xml")),
		api.NewGroup("/api").
			ContentType("invalid").
			Route(api.POST("/resource")),
	} {
		routes := group.Build()
		require.Len(t, routes, 1)
		mux := api.NewMux()
		require.Error(t, mux.Register(&routes[0]))
	}
}

func TestGroupBuildNormalizesRoutePath(t *testing.T) {
	tests := []struct {
		name  string
		group api.Group
		want  string
	}{
		{
			name: "relative group path",
			group: api.NewGroup("internal").
				Route(api.POST("/authorize")),
			want: "/internal/authorize",
		},
		{
			name: "relative route path",
			group: api.NewGroup("").
				Route(api.POST("internal/authorize")),
			want: "/internal/authorize",
		},
		{
			name: "root path",
			group: api.NewGroup("").
				Route(api.GET("")),
			want: "/",
		},
		{
			name: "nested relative group path",
			group: api.NewGroup("internal").
				SubGroup(api.NewGroup("oauth").
					Route(api.POST("/authorize"))),
			want: "/internal/oauth/authorize",
		},
		{
			name: "absolute path remains unchanged",
			group: api.NewGroup("/internal").
				Route(api.POST("/authorize")),
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

func TestStrictQueryRouteDeclarations(t *testing.T) {
	calls := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.WriteString(w, api.Query(r, "service", "default"))
	}
	server := api.New().
		Group(api.NewGroup("/strict").
			StrictQuery().
			Param(api.QueryParam("service", "Service filter").
				Optional()).
			SubGroup(api.NewGroup("/nested").
				Route(api.GET("").
					Param(api.QueryParam("state", "State filter").
						Optional()).
					Param(api.QueryParam("limit", "Page size").
						Type("integer").
						Optional()).
					Param(api.QueryParam("synchronous", "Wait for completion").
						Type("boolean").
						Optional()).
					Param(api.HeaderParam("Trace", "Request trace")).
					To(handler)))).
		Route(api.GET("/route").
			StrictQuery().
			To(handler)).
		Route(api.GET("/permissive").
			To(handler)).
		Build()
	for _, test := range []struct {
		path   string
		status int
		body   string
	}{
		{"/strict/nested?service=cloud&state=pending", 200, "cloud"},
		{"/strict/nested", 200, "default"},
		{"/strict/nested?serivce=cloud", 400, ""},
		{"/strict/nested?Trace=x", 400, ""},
		{"/strict/nested?limit=0&synchronous=false", 200, "default"},
		{"/strict/nested?limit=", 400, ""},
		{"/strict/nested?limit=wrong", 400, ""},
		{"/strict/nested?limit=9223372036854775808", 400, ""},
		{"/strict/nested?synchronous=maybe", 400, ""},
		{"/strict/nested?service=%ZZ", 400, ""},
		{"/strict/nested?service=cloud;state=pending", 400, ""},
		{"/route", 200, "default"},
		{"/route?service=cloud", 400, ""},
		{"/permissive?service=cloud", 200, "cloud"},
	} {
		t.Run(test.path, func(t *testing.T) {
			before := calls
			w := httptest.NewRecorder()
			server.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
			require.Equal(t, test.status, w.Code, w.Body.String())
			if test.status == 400 {
				require.Equal(t, before, calls, "invalid query reached handler")
				return
			}
			require.Equal(t, before+1, calls)
			require.Equal(t, test.body, w.Body.String())
		})
	}
}
