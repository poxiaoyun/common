package api

import (
	"net/http"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/rest/matcher"
)

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
				"/{org}/{repo}*",
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
				"/a/{a}/b/{b}*",
				"/a/{a}/b/{b}*/index",
				"/a/{a}/b/{b}*/manifests/{c}",
			},
			req:       "/a/core/b/foo/bar/manifests/v1",
			matched:   true,
			wantMatch: "/a/{a}/b/{b}*/manifests/{c}",
			vars: []MatchVar{
				{Name: "a", Value: "core"},
				{Name: "b", Value: "foo/bar"},
				{Name: "c", Value: "v1"},
			},
		},
		{
			registered: []string{
				"/api/{a}",
				"/api/v{a}*",
				"/api/v1",
				"/apis",
				"/api/{a}/{b}/{c}",
				"/api/{path}*",
			},
			req:       "/api/v1/g/v/k",
			matched:   true,
			wantMatch: "/api/v{a}*",
			vars: []MatchVar{
				{Name: "a", Value: "1/g/v/k"},
			},
		},
		{
			registered: []string{
				"/v1/service-proxy/{realpath}*",
				"/v1/{group}/{version}/{resource}",
			},
			req:       "/v1/service-proxy/js/t2.js",
			matched:   true,
			wantMatch: "/v1/service-proxy/{realpath}*",
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
				"/api/v2/{a}*",
				"/api/{a}/{b}/{c}",
				"/api/{path}*",
			},
			req:       "/api/v2/v/k",
			matched:   true,
			wantMatch: "/api/v2/{a}*",
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
				"/api/{name}/{path}*:action",
				"/api/{name}/{path}*",
			},
			req:       "/api/dog/wang/1:action",
			matched:   true,
			wantMatch: "/api/{name}/{path}*:action",
			vars: []MatchVar{
				{Name: "name", Value: "dog"},
				{Name: "path", Value: "wang/1"},
			},
		},
		{
			registered: []string{
				"/api/{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}*/manifests/{reference}",
				"/api/{repository}*/blobs/{digest:[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}}",
			},
			req:       "/api/lib/a/b/c/manifests/v1",
			matched:   true,
			wantMatch: "/api/{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}*/manifests/{reference}",
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
				"/api/{scopes}*/roles",
				"/api/{scopes}*/members/{member}/roles/{role}",
				"/api/{scopes}*/members",
				"/api/{scopes}*/members/{member}",
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
				"/api/{scopes}*/members/abc",
				"/api/{scopes}*/members/{member}/roles/{role}",
				"/api/{scopes}*/members/{member}",
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
				"/api/{path}*",
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
				"/api/{path}*",
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
				"/api/{path}*",
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
				"/api/{path}*",
			},
			req:       "/api/a/b/c",
			matched:   true,
			wantMatch: "/api/{path}*",
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
				"/files/{path}*",
				"/files/{path}*/download",
			},
			req:       "/files/a/b/c/download",
			matched:   true,
			wantMatch: "/files/{path}*/download",
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
			m := NewMux()
			for _, v := range tt.registered {
				if err := m.Handle("", v, http.NotFoundHandler()); err != nil {
					t.Error(err)
				}
			}
			node, vars := m.GlobalTree.Match(tt.req, DefaultMatchCandidateFunc)
			matched := (node != nil && node.Value != nil)
			if matched != tt.matched {
				t.Errorf("matcher.Match() matched = %v, want %v", matched, tt.matched)
			}
			if !reflect.DeepEqual(vars, tt.vars) {
				t.Errorf("matcher.Match() vars = %v, want %v", vars, tt.vars)
			}
			if tt.wantMatch != "" && node != nil && node.Pattern != tt.wantMatch {
				t.Errorf("matcher.Match() pattern = %v, want %v", node.Pattern, tt.wantMatch)
			}
		})
	}
}
