package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/rest/matcher"
)

func TestMuxMediaFallbackKeepsOnlySelectedRouteEffects(t *testing.T) {
	mux := api.NewMux()
	var rejectedCalls, selectedCalls int
	rejected := api.POST("/files/{name}.json").
		ContentType("application/json").
		To(func(w http.ResponseWriter, r *http.Request) { t.Fatal("rejected handler ran") })
	rejected.Filters = api.Filters{api.FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		rejectedCalls++
		next.ServeHTTP(w, r)
	})}
	require.NoError(t, mux.Register(&rejected))
	fallback := api.POST("/files/{path...}").
		To(func(w http.ResponseWriter, r *http.Request) {
			selectedCalls++
			require.Equal(t, api.PathVarList{{Key: "path", Value: "record.json"}}, api.PathVars(r))
			_, _ = io.WriteString(w, "selected")
		})
	require.NoError(t, mux.Register(&fallback))
	request := httptest.NewRequest(http.MethodPost, "/files/record.json", unreadableBody{t: t})
	request.Header.Set("Content-Type", "application/xml")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	require.Equal(t, "selected", response.Body.String())
	require.Zero(t, rejectedCalls)
	require.Equal(t, 1, selectedCalls)
	header := response.Header()
	require.Contains(t, header.Values("Vary"), "Content-Type")
}

func TestMuxRejectsEmptyFinalPathVariableBeforeFallback(t *testing.T) {
	mux := api.NewMux()
	require.NoError(t, mux.Handle(http.MethodGet, "/files/{name}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("empty final variable was accepted")
	})))
	require.NoError(t, mux.Handle(http.MethodGet, "/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, api.PathVarList{{Key: "path", Value: "files/"}}, api.PathVars(r))
		w.WriteHeader(http.StatusNoContent)
	})))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/files/", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestMuxGreedySuffixAllowsEmptyCapture(t *testing.T) {
	mux := api.NewMux()
	var matchedVars api.PathVarList
	for _, pattern := range []string{"/v2", "/v2/{rest...}", "/org/{organization}/{repository}"} {
		require.NoError(t, mux.Handle(http.MethodGet, pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			matchedVars = api.PathVars(r)
			w.WriteHeader(http.StatusNoContent)
		})))
	}
	for _, test := range []struct {
		path   string
		status int
		vars   api.PathVarList
	}{
		{path: "/v2", status: http.StatusNoContent, vars: api.PathVarList{}},
		{path: "/v2/", status: http.StatusNoContent, vars: api.PathVarList{{Key: "rest", Value: ""}}},
		{path: "/v2/x", status: http.StatusNoContent, vars: api.PathVarList{{Key: "rest", Value: "x"}}},
		{path: "/org/team/repo", status: http.StatusNoContent, vars: api.PathVarList{{Key: "organization", Value: "team"}, {Key: "repository", Value: "repo"}}},
		{path: "/org//repo", status: http.StatusNotFound},
		{path: "/org/team/", status: http.StatusNotFound},
	} {
		t.Run(test.path, func(t *testing.T) {
			matchedVars = nil
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			require.Equal(t, test.status, response.Code)
			require.Equal(t, test.vars, matchedVars)
		})
	}
}

func TestMuxSelectedRejectionDoesNotTryFallback(t *testing.T) {
	mux := api.NewMux()
	protected := api.GET("/private").
		Accept("application/json").
		To(func(w http.ResponseWriter, r *http.Request) { t.Fatal("protected handler ran") })
	protected.Filters = api.Filters{api.FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		http.Error(w, "denied", http.StatusUnauthorized)
	})}
	protected.Priority = 1
	require.NoError(t, mux.Register(&protected))
	require.NoError(t, mux.Handle("", "/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("authorization denial triggered route fallback")
	})))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/private", nil))
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestMuxPriorityResolvesOverlappingPaths(t *testing.T) {
	for _, priority := range []int{-1, 0, 1} {
		mux := api.NewMux()
		require.NoError(t, mux.Handle("", "/console/{rest...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "console")
		})))
		repository := api.POST("/{organization}/{kind}/{repository}.git/{rest...}").
			ContentType("application/x-git-upload-pack-request").
			To(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, api.PathVarList{
					{Key: "organization", Value: "console"},
					{Key: "kind", Value: "models"},
					{Key: "repository", Value: "model.git.backup"},
					{Key: "rest", Value: "git-upload-pack"},
				}, api.PathVars(r))
				_, _ = io.WriteString(w, "repository")
			})
		repository.Priority = priority
		require.NoError(t, mux.Register(&repository))
		for _, test := range []struct{ method, contentType string }{
			{http.MethodPost, "application/x-git-upload-pack-request"},
			{http.MethodPost, "application/json"},
			{http.MethodGet, "application/x-git-upload-pack-request"},
		} {
			request := httptest.NewRequest(test.method, "/console/models/model.git.backup.git/git-upload-pack", nil)
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			want := "console"
			if priority > 0 && test.method == http.MethodPost && test.contentType == "application/x-git-upload-pack-request" {
				want = "repository"
			}
			require.Equal(t, want, response.Body.String(), "priority=%d method=%s content-type=%s", priority, test.method, test.contentType)
		}
	}
}

func TestMuxPriorityPrecedesMethodAndMediaWithinPath(t *testing.T) {
	for _, priority := range []int{0, 1} {
		mux := api.NewMux()
		post := mediaRoute("post").
			ContentType("application/json")
		require.NoError(t, mux.Register(&post))
		anyMethod := api.Any("/media").
			ContentType("application/*").
			To(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "any")
			})
		anyMethod.Priority = priority
		require.NoError(t, mux.Register(&anyMethod))
		request := httptest.NewRequest(http.MethodPost, "/media", nil)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		want := "post"
		if priority > 0 {
			want = "any"
		}
		require.Equal(t, want, response.Body.String())
	}
}

func TestMuxRetainsSelectedVariablesDuringBacktracking(t *testing.T) {
	mux := api.NewMux()
	repository := api.GET("/{a}/{b}/{c}/{repository}.git").
		To(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, api.PathVarList{
				{Key: "a", Value: "one"}, {Key: "b", Value: "two"},
				{Key: "c", Value: "three"}, {Key: "repository", Value: "model"},
			}, api.PathVars(r))
			w.WriteHeader(http.StatusNoContent)
		})
	repository.Priority = 1
	require.NoError(t, mux.Register(&repository))
	require.NoError(t, mux.Handle(http.MethodGet, "/{a}/{b}/{c}/{file}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("lower priority handler ran")
	})))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/one/two/three/model.git", nil))
	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestMuxMethodAndMediaFailures(t *testing.T) {
	mux := api.NewMux()
	for _, route := range []api.Route{
		api.POST("/resource").
			ContentType("application/json"),
		api.PUT("/resource").
			ContentType("application/xml"),
	} {
		require.NoError(t, mux.Register(&route))
	}
	for _, test := range []struct {
		method, contentType string
		status              int
		allow               string
	}{
		{http.MethodPost, "text/plain", http.StatusNotFound, ""},
		{http.MethodPatch, "application/json", http.StatusMethodNotAllowed, "POST"},
		{http.MethodDelete, "application/xml", http.StatusMethodNotAllowed, "PUT"},
		{http.MethodPatch, "", http.StatusNotFound, ""},
		{http.MethodOptions, "", http.StatusOK, "OPTIONS, POST, PUT"},
	} {
		t.Run(test.method+test.contentType, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/resource", nil)
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			require.Equal(t, test.status, response.Code)
			header := response.Header()
			require.Equal(t, test.allow, header.Get("Allow"))
		})
	}
	var customStatusCalled bool
	mux.SetMethodNotAllowed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customStatusCalled = true
		header := w.Header()
		require.Equal(t, "POST", header.Get("Allow"))
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	request := httptest.NewRequest(http.MethodDelete, "/resource", nil)
	request.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(httptest.NewRecorder(), request)
	require.True(t, customStatusCalled)
}

func TestMuxExplicitMethodPrecedesAny(t *testing.T) {
	mux := api.NewMux()
	require.NoError(t, mux.Handle("", "/media", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "any")
	})))
	route := mediaRoute("post").
		ContentType("application/json")
	require.NoError(t, mux.Register(&route))
	for _, test := range []struct{ method, contentType, want string }{
		{http.MethodPost, "application/json", "post"},
		{http.MethodPost, "application/xml", "any"},
		{http.MethodOptions, "", "any"},
	} {
		request := httptest.NewRequest(test.method, "/media", nil)
		request.Header.Set("Content-Type", test.contentType)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, test.want, response.Body.String())
	}
}

func TestMuxKeepsRegisteredRouteFiltersAndHostPatterns(t *testing.T) {
	mux := api.NewMux()
	route := api.GET("/items/{id:[0-9]+}").
		Host("first.example", "second.example").
		To(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "item")
		})
	require.NoError(t, mux.Register(&route))
	var filterCalls int
	route.Filters = append(route.Filters, api.FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		filterCalls++
		next.ServeHTTP(w, r)
	}))
	require.NoError(t, mux.Handle("", "/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "global")
	})))
	for _, host := range []string{"first.example", "second.example"} {
		valid := httptest.NewRecorder()
		mux.ServeHTTP(valid, httptest.NewRequest(http.MethodGet, "http://"+host+"/items/42", nil))
		require.Equal(t, "item", valid.Body.String())
		invalid := httptest.NewRecorder()
		mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "http://"+host+"/items/name", nil))
		require.Equal(t, http.StatusNotFound, invalid.Code)
	}
	require.Equal(t, 2, filterCalls)
}

func TestMuxVaryIncludesRejectedMediaCandidates(t *testing.T) {
	mux := api.NewMux()
	route := api.GET("/media").
		Accept("application/json")
	require.NoError(t, mux.Register(&route))
	request := httptest.NewRequest(http.MethodGet, "/media", nil)
	request.Header.Set("Accept", "text/plain")
	response := httptest.NewRecorder()
	header := response.Header()
	header.Set("Vary", "Accept-Encoding")
	mux.ServeHTTP(response, request)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, "Accept-Encoding,Accept", strings.Join(header.Values("Vary"), ","))
}

type unreadableBody struct{ t *testing.T }

func (body unreadableBody) Read([]byte) (int, error) {
	body.t.Fatal("route selection read the body")
	return 0, io.EOF
}

type MatchVar = matcher.MatchVar

func Test_matcher_Match(t *testing.T) {
	tests := []struct {
		name       string
		registered []string
		req        string
		matched    bool
		wantMatch  string
		vars       []MatchVar
	}{
		{
			name: "trailing_slash_match",
			registered: []string{
				"/docs",
				"/docs/",
			},
			req:       "/docs/",
			matched:   true,
			wantMatch: "/docs/",
		},
		{
			name: "no_trailing_slash_match",
			registered: []string{
				"/docs",
				"/docs/",
			},
			req:       "/docs",
			matched:   true,
			wantMatch: "/docs",
		},
		{
			name: "console_with_trailing_slash",
			registered: []string{
				"/console",
				"/console/",
			},
			req:       "/console/",
			matched:   true,
			wantMatch: "/console/",
		},
		{
			name: "console_without_trailing_slash",
			registered: []string{
				"/console",
				"/console/",
			},
			req:       "/console",
			matched:   true,
			wantMatch: "/console",
		},
		{
			registered: []string{
				"/{group}/{version}/{resource}",
				"/{group}/{version}/namespaces/{namespace}",
			},
			req:     "/core/v1/namespaces",
			matched: true,
			vars: []MatchVar{
				{Name: "group", Value: "core"},
				{Name: "version", Value: "v1"},
				{Name: "resource", Value: "namespaces"},
			},
		},
		{
			registered: []string{
				"/front/*",
				"/{org}/{repo...}",
			},
			req:     "/front/@iconify-json/logos-c3b8b8cf.js",
			matched: true,
		},
		{
			registered: []string{
				"/api/s",
			},
			req:     "/api",
			matched: false,
		},
		{
			registered: []string{
				"/a/{a}/b/{b...}",
				"/a/{a}/b/{b...}/index",
				"/a/{a}/b/{b...}/manifests/{c}",
			},
			req:       "/a/core/b/foo/bar/manifests/v1",
			matched:   true,
			wantMatch: "/a/{a}/b/{b...}/manifests/{c}",
			vars: []MatchVar{
				{Name: "a", Value: "core"},
				{Name: "b", Value: "foo/bar"},
				{Name: "c", Value: "v1"},
			},
		},
		{
			registered: []string{
				"/api/{a}",
				"/api/v{a...}",
				"/api/v1",
				"/apis",
				"/api/{a}/{b}/{c}",
				"/api/{path...}",
			},
			req:       "/api/v1/g/v/k",
			matched:   true,
			wantMatch: "/api/v{a...}",
			vars: []MatchVar{
				{Name: "a", Value: "1/g/v/k"},
			},
		},
		{
			registered: []string{
				"/v1/service-proxy/{realpath...}",
				"/v1/{group}/{version}/{resource}",
			},
			req:       "/v1/service-proxy/js/t2.js",
			matched:   true,
			wantMatch: "/v1/service-proxy/{realpath...}",
			vars: []MatchVar{
				{Name: "realpath", Value: "js/t2.js"},
			},
		},
		{
			registered: []string{
				"/v1/{group}/{version}/{resource}/{name}",
				"/v1/{group}/{version}/configmap/{name}",
			},
			req:       "/v1/core/v1/configmap/abc",
			matched:   true,
			wantMatch: "/v1/{group}/{version}/configmap/{name}",
			vars: []MatchVar{
				{Name: "group", Value: "core"},
				{Name: "version", Value: "v1"},
				{Name: "name", Value: "abc"},
			},
		},
		{
			registered: []string{
				"/api/v2/{a...}",
				"/api/{a}/{b}/{c}",
				"/api/{path...}",
			},
			req:       "/api/v2/v/k",
			matched:   true,
			wantMatch: "/api/v2/{a...}",
			vars: []MatchVar{
				{Name: "a", Value: "v/k"},
			},
		},
		{
			registered: []string{
				"/api/dog:wang",
			},
			req:     "/api/dog",
			matched: false,
		},
		{
			registered: []string{
				"/api/{dog:[a-z]+}",
			},
			req:     "/api/HI",
			matched: false,
		},
		{
			registered: []string{"/api"},
			req:        "",
			matched:    false,
		},
		{
			registered: []string{
				"/api/{name}/{path...}:action",
				"/api/{name}/{path...}",
			},
			req:       "/api/dog/wang/1:action",
			matched:   true,
			wantMatch: "/api/{name}/{path...}:action",
			vars: []MatchVar{
				{Name: "name", Value: "dog"},
				{Name: "path", Value: "wang/1"},
			},
		},
		{
			registered: []string{
				"/api/{repository...:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}/manifests/{reference}",
				"/api/{repository...}/blobs/{digest:[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}}",
			},
			req:       "/api/lib/a/b/c/manifests/v1",
			matched:   true,
			wantMatch: "/api/{repository...:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}/manifests/{reference}",
			vars: []MatchVar{
				{Name: "repository", Value: "lib/a/b/c"},
				{Name: "reference", Value: "v1"},
			},
		},
		{
			registered: []string{
				"/api/tenants/{tenant}/organizations",
			},
			req:     "/api/tenants//organizations",
			matched: false,
		},
		{
			registered: []string{
				"/api/organizations/{org}/roles",
				"/api/{scopes...}/roles",
				"/api/{scopes...}/members/{member}/roles/{role}",
				"/api/{scopes...}/members",
				"/api/{scopes...}/members/{member}",
			},
			req:     "/api/regions/global/members/john/roles/admin",
			matched: true,
			vars: []MatchVar{
				{Name: "scopes", Value: "regions/global"},
				{Name: "member", Value: "john"},
				{Name: "role", Value: "admin"},
			},
		},
		{
			registered: []string{
				"/api/{scopes...}/members/abc",
				"/api/{scopes...}/members/{member}/roles/{role}",
				"/api/{scopes...}/members/{member}",
			},
			req:     "/api/regions/global/members/john/roles/admin",
			matched: true,
			vars: []MatchVar{
				{Name: "scopes", Value: "regions/global"},
				{Name: "member", Value: "john"},
				{Name: "role", Value: "admin"},
			},
		},
		{
			registered: []string{
				"/",
				"/{service}",
			},
			req:       "/",
			matched:   true,
			wantMatch: "/",
		},
		{
			name: "static_vs_dynamic",
			registered: []string{
				"/v1/nodes",
				"/v1/{resource}",
			},
			req:       "/v1/nodes",
			matched:   true,
			wantMatch: "/v1/nodes",
		},
		{
			name: "static_vs_dynamic_2",
			registered: []string{
				"/v1/nodes",
				"/v1/{resource}",
			},
			req:       "/v1/pods",
			matched:   true,
			wantMatch: "/v1/{resource}",
			vars: []MatchVar{
				{Name: "resource", Value: "pods"},
			},
		},
		{
			name: "regex_vs_plain_variable",
			registered: []string{
				"/api/{id:[0-9]+}",
				"/api/{id}",
			},
			req:       "/api/123",
			matched:   true,
			wantMatch: "/api/{id:[0-9]+}",
			vars: []MatchVar{
				{Name: "id", Value: "123"},
			},
		},
		{
			name: "regex_vs_plain_variable_2",
			registered: []string{
				"/api/{id:[0-9]+}",
				"/api/{id}",
			},
			req:       "/api/abc",
			matched:   true,
			wantMatch: "/api/{id}",
			vars: []MatchVar{
				{Name: "id", Value: "abc"},
			},
		},
		{
			name: "multiple_priority_levels",
			registered: []string{
				"/api/users",
				"/api/{id:[0-9]+}",
				"/api/{id}",
				"/api/{path...}",
			},
			req:       "/api/users",
			matched:   true,
			wantMatch: "/api/users",
		},
		{
			name: "multiple_priority_levels_2",
			registered: []string{
				"/api/users",
				"/api/{id:[0-9]+}",
				"/api/{id}",
				"/api/{path...}",
			},
			req:       "/api/123",
			matched:   true,
			wantMatch: "/api/{id:[0-9]+}",
			vars: []MatchVar{
				{Name: "id", Value: "123"},
			},
		},
		{
			name: "multiple_priority_levels_3",
			registered: []string{
				"/api/users",
				"/api/{id:[0-9]+}",
				"/api/{id}",
				"/api/{path...}",
			},
			req:       "/api/abc",
			matched:   true,
			wantMatch: "/api/{id}",
			vars: []MatchVar{
				{Name: "id", Value: "abc"},
			},
		},
		{
			name: "multiple_priority_levels_4",
			registered: []string{
				"/api/users",
				"/api/{id:[0-9]+}",
				"/api/{id}",
				"/api/{path...}",
			},
			req:       "/api/a/b/c",
			matched:   true,
			wantMatch: "/api/{path...}",
			vars: []MatchVar{
				{Name: "path", Value: "a/b/c"},
			},
		},
		{
			name: "complex_priority",
			registered: []string{
				"/api/v1/users",
				"/api/v1/{resource}",
				"/api/{version}/users",
				"/api/{version}/{resource}",
			},
			req:       "/api/v1/users",
			matched:   true,
			wantMatch: "/api/v1/users",
		},
		{
			name: "complex_priority_2",
			registered: []string{
				"/api/v1/users",
				"/api/v1/{resource}",
				"/api/{version}/users",
				"/api/{version}/{resource}",
			},
			req:       "/api/v1/pods",
			matched:   true,
			wantMatch: "/api/v1/{resource}",
			vars: []MatchVar{
				{Name: "resource", Value: "pods"},
			},
		},
		{
			name: "complex_priority_3",
			registered: []string{
				"/api/v1/users",
				"/api/v1/{resource}",
				"/api/{version}/users",
				"/api/{version}/{resource}",
			},
			req:       "/api/v2/users",
			matched:   true,
			wantMatch: "/api/{version}/users",
			vars: []MatchVar{
				{Name: "version", Value: "v2"},
			},
		},
		{
			name: "complex_priority_4",
			registered: []string{
				"/api/v1/users",
				"/api/v1/{resource}",
				"/api/{version}/users",
				"/api/{version}/{resource}",
			},
			req:       "/api/v2/pods",
			matched:   true,
			wantMatch: "/api/{version}/{resource}",
			vars: []MatchVar{
				{Name: "version", Value: "v2"},
				{Name: "resource", Value: "pods"},
			},
		},
		{
			name: "greedy_with_suffix",
			registered: []string{
				"/files/{path...}",
				"/files/{path...}/download",
			},
			req:       "/files/a/b/c/download",
			matched:   true,
			wantMatch: "/files/{path...}/download",
			vars: []MatchVar{
				{Name: "path", Value: "a/b/c"},
			},
		},
		{
			name: "variable_with_slash",
			registered: []string{
				"/api/{group}/{version}",
			},
			req:       "/api/core/v1",
			matched:   true,
			wantMatch: "/api/{group}/{version}",
			vars: []MatchVar{
				{Name: "group", Value: "core"},
				{Name: "version", Value: "v1"},
			},
		},
	}
	for _, tt := range tests {
		name := tt.req
		if tt.name != "" {
			name = tt.name
		}
		t.Run(name, func(t *testing.T) {
			m := api.NewMux()
			var vars []MatchVar
			var pattern string
			matched := false
			for _, v := range tt.registered {
				if err := m.Handle("", v, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					matched, pattern = true, v
					for _, variable := range api.PathVars(r) {
						vars = append(vars, MatchVar{Name: variable.Key, Value: variable.Value})
					}
				})); err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.URL.Path = tt.req
			m.ServeHTTP(httptest.NewRecorder(), request)
			if matched != tt.matched {
				t.Errorf("matcher.Match() matched = %v, want %v", matched, tt.matched)
			}
			if !reflect.DeepEqual(vars, tt.vars) {
				t.Errorf("matcher.Match() vars = %v, want %v", vars, tt.vars)
			}
			if tt.wantMatch != "" && matched && pattern != tt.wantMatch {
				t.Errorf("matcher.Match() pattern = %v, want %v", pattern, tt.wantMatch)
			}
		})
	}
}
