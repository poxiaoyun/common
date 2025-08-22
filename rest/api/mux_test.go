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
		registered []string
		req        string
		matched    bool
		wantMatch  string
		vars       []MatchVar
	}{
		{
			registered: []string{
				"/docs",
				"/docs/",
			},
			req:       "/docs/",
			matched:   true,
			wantMatch: "/docs/",
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
				"/api/v{a}*",
				"/api/{a}/{b}/{c}",
				"/api/{path}*",
			},
			req:       "/api/v2/v/k",
			matched:   true,
			wantMatch: "/api/v{a}*",
			vars: []MatchVar{
				{Name: "a", Value: "2/v/k"},
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
	}
	for _, tt := range tests {
		t.Run(tt.req, func(t *testing.T) {
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
		})
	}
}
