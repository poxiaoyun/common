package matcher_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"xiaoshiai.cn/common/rest/matcher"
)

func TestPathPatterns(t *testing.T) {
	for _, test := range []struct {
		pattern, path string
		match         bool
		vars          []matcher.MatchVar
	}{
		{"/v2", "/v2", true, nil},
		{"/v2", "/v2/", false, nil},
		{"/v2/", "/v2", false, nil},
		{"/v2/", "/v20/item", false, nil},
		{"/v2/", "/v2/", true, nil},
		{"/v2/", "/v2/a//b/../c", true, nil},
		{"/v2/{$}", "/v2/", true, nil},
		{"/v2/{$}", "/v2/a", false, nil},
		{"/{$}", "/", true, nil},
		{"/{$}", "/a", false, nil},
		{"/", "/a/b", true, nil},
		{"/v2/{rest...}", "/v2/", true, []matcher.MatchVar{{Name: "rest", Value: ""}}},
		{"/v2/{rest...}", "/v2/a%2Fb/%252F", true, []matcher.MatchVar{{Name: "rest", Value: "a/b/%2F"}}},
		{"/files/{name}", "/files/a%2Fb", true, []matcher.MatchVar{{Name: "name", Value: "a/b"}}},
		{"/files/{name}", "/files/a/b", false, nil},
		{"/{type}", "/models", true, []matcher.MatchVar{{Name: "type", Value: "models"}}},
		{"/files/%61%2fb/{$}", "/files/a%2Fb/", true, nil},
		{"/files/%7Bname%7D", "/files/%7Bname%7D", true, nil},
		{"/proxy/kubernetes{path...}", "/proxy/kubernetes/api/v1/pods", true, []matcher.MatchVar{{Name: "path", Value: "/api/v1/pods"}}},
		{"/{scopes...}/roles/{role}", "/regions/global/roles/admin", true, []matcher.MatchVar{{Name: "scopes", Value: "regions/global"}, {Name: "role", Value: "admin"}}},
		{"/{scopes...}/roles/{role}", "/regions%2Froles%2Ffake/roles/admin", true, []matcher.MatchVar{{Name: "scopes", Value: "regions/roles/fake"}, {Name: "role", Value: "admin"}}},
		{"/{left}2{right}", "/a%2Fb2c", true, []matcher.MatchVar{{Name: "left", Value: "a/b"}, {Name: "right", Value: "c"}}},
		{"/{rest...}/{$}", "/a/b/", true, []matcher.MatchVar{{Name: "rest", Value: "a/b"}}},
		{"/{name:a|b}", "/abc", false, nil},
		{"/{name:a|b}", "/b", true, []matcher.MatchVar{{Name: "name", Value: "b"}}},
		{"/{name...:[a-z/]+}/last", "/a/b/last", true, []matcher.MatchVar{{Name: "name", Value: "a/b"}}},
		{"/assets/prefix-*.css", "/assets/prefix-a/b.css", true, nil},
		{"/api/v{version}/{name...}", "/api/v2/a/b", true, []matcher.MatchVar{{Name: "version", Value: "2"}, {Name: "name", Value: "a/b"}}},
	} {
		t.Run(test.pattern+" "+test.path, func(t *testing.T) {
			root := &matcher.Node[bool]{}
			_, registered, err := root.Register(test.pattern)
			if err != nil {
				t.Fatal(err)
			}
			registered.Value = true
			node, vars := root.Match(test.path, nil)
			if (node != nil) != test.match || !reflect.DeepEqual(vars, test.vars) {
				t.Fatalf("match=%v vars=%v, want match=%v vars=%v", node != nil, vars, test.match, test.vars)
			}
		})
	}
}

func TestInvalidPatterns(t *testing.T) {
	for _, pattern := range []string{"", "/{}", "/{a}/{a}", "/{1name}", "/{id", "/a}", "/{$}/a", "/a{$}", "/{id:[}", "/%xx"} {
		t.Run(pattern, func(t *testing.T) {
			root := &matcher.Node[bool]{}
			if _, _, err := root.Register(pattern); err == nil {
				t.Fatalf("accepted invalid pattern %q", pattern)
			}
		})
	}
}

func TestStandardPathCaptures(t *testing.T) {
	for _, pattern := range []string{"/files/{name}", "/files/{rest...}", "/files/", "/files/{$}", "/{$}", "/"} {
		t.Run(pattern, func(t *testing.T) {
			standard := http.NewServeMux()
			standard.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				w.Header().
					Set("Name", r.PathValue("name"))
				w.Header().
					Set("Rest", r.PathValue("rest"))
				w.WriteHeader(http.StatusNoContent)
			})
			root := &matcher.Node[bool]{}
			if _, _, err := root.Register(pattern); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/", "/files/", "/files/a", "/files/a/b", "/files/a%2fb", "/files/%252F", "/files/%E6%A8%A1%E5%9E%8B", "/other"} {
				response := httptest.NewRecorder()
				standard.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				node, vars := root.Match(path, nil)
				if (node != nil) != (response.Code == http.StatusNoContent) {
					t.Fatalf("%s: matched=%v, ServeMux status=%d", path, node != nil, response.Code)
				}
				for _, capture := range vars {
					if capture.Value != response.Header().
						Get(capture.Name) {
						t.Errorf("%s capture=%v, ServeMux=%v", path, capture, response.Header())
					}
				}
			}
		})
	}
}

func TestMatchCandidateReceivesCompleteVariables(t *testing.T) {
	root := &matcher.Node[string]{}
	for _, pattern := range []string{"/org/{org}/repos/{name}.git", "/{path...}"} {
		_, node, err := root.Register(pattern)
		if err != nil {
			t.Fatal(err)
		}
		node.Value = pattern
	}
	var seen [][]matcher.MatchVar
	node, vars := root.Match("/org/team/repos/model.git", func(pattern string, vars []matcher.MatchVar) bool {
		seen = append(seen, slices.Clone(vars))
		return pattern == "/{path...}"
	})
	want := [][]matcher.MatchVar{
		{{Name: "org", Value: "team"}, {Name: "name", Value: "model"}},
		{{Name: "path", Value: "org/team/repos/model.git"}},
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("candidate variables = %v, want %v", seen, want)
	}
	if node == nil || node.Value != "/{path...}" || !reflect.DeepEqual(vars, want[1]) {
		t.Fatalf("fallback result = %v, %v", node, vars)
	}
}

func TestMatchAnchorsFinalLiteralSuffix(t *testing.T) {
	for _, test := range []struct {
		pattern, path string
		want          []matcher.MatchVar
	}{
		{pattern: "/{repository}.git", path: "/model.git.backup.git", want: []matcher.MatchVar{{Name: "repository", Value: "model.git.backup"}}},
		{pattern: "/{repository}.git/{rest...}", path: "/model.git.backup.git/info/refs", want: []matcher.MatchVar{{Name: "repository", Value: "model.git.backup"}, {Name: "rest", Value: "info/refs"}}},
		{pattern: "/{repository}.git", path: "/model.git.backup"},
		{pattern: "/{repository}.git", path: "/.git"},
		{pattern: "/{repository:[a-z.]+}.git", path: "/model.git.backup.git", want: []matcher.MatchVar{{Name: "repository", Value: "model.git.backup"}}},
		{pattern: "/{repository:[a-z]+}.git", path: "/model.git.backup.git"},
		{pattern: "/{name}.{extension}", path: "/model.git.backup", want: []matcher.MatchVar{{Name: "name", Value: "model"}, {Name: "extension", Value: "git.backup"}}},
	} {
		t.Run(test.pattern+test.path, func(t *testing.T) {
			root := &matcher.Node[bool]{}
			_, registered, err := root.Register(test.pattern)
			if err != nil {
				t.Fatal(err)
			}
			registered.Value = true
			node, vars := root.Match(test.path, nil)
			if (node != nil) != (test.want != nil) || !reflect.DeepEqual(vars, test.want) {
				t.Fatalf("match = %v, %v; want captures %v", node, vars, test.want)
			}
		})
	}
}

func TestCompileError_Error(t *testing.T) {
	tests := []struct {
		name   string
		fields matcher.CompileError
		want   string
	}{
		{
			fields: matcher.CompileError{
				Pattern:  "pre{name}suf",
				Position: 1,
				Str:      "pre",
				Message:  "invalid character",
			},
			want: "invalid [pre] in [pre{name}suf] at position 1: invalid character",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.fields
			if got := e.Error(); got != tt.want {
				t.Errorf("matcher.CompileError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

// BenchmarkMatch 测试完整的路由匹配性能
func BenchmarkMatch(b *testing.B) {
	scenarios := []struct {
		name   string
		routes []string
		path   string
	}{
		{
			name:   "static_simple",
			routes: []string{"/api/users"},
			path:   "/api/users",
		},
		{
			name:   "static_complex",
			routes: []string{"/api/v1/users/list"},
			path:   "/api/v1/users/list",
		},
		{
			name:   "dynamic_simple",
			routes: []string{"/api/{id}"},
			path:   "/api/123",
		},
		{
			name:   "dynamic_complex",
			routes: []string{"/api/{group}/{version}/{resource}"},
			path:   "/api/core/v1/pods",
		},
		{
			name: "multiple_routes",
			routes: []string{
				"/api/users",
				"/api/{id}",
				"/api/v1/users",
				"/api/v1/{resource}",
			},
			path: "/api/v1/pods",
		},
		{
			name: "greedy_match",
			routes: []string{
				"/api/{path...}",
				"/api/v1/{path...}",
			},
			path: "/api/v1/users/123/posts/456",
		},
		{
			name: "regex_validation",
			routes: []string{
				"/api/{id:[0-9]+}",
				"/api/{name:[a-z]+}",
			},
			path: "/api/123",
		},
		{
			name: "complex_pattern",
			routes: []string{
				"/a/{a}/b/{b...}",
				"/a/{a}/b/{b...}/index",
				"/a/{a}/b/{b...}/manifests/{c}",
			},
			path: "/a/core/b/foo/bar/manifests/v1",
		},
		{
			name: "root_vs_variable",
			routes: []string{
				"/",
				"/{service}",
			},
			path: "/",
		},
		{
			name: "priority_test",
			routes: []string{
				"/api/{path...}",
				"/api/{id}",
				"/api/users",
			},
			path: "/api/users",
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			// 构建路由树
			root := &matcher.Node[string]{}
			for _, route := range sc.routes {
				_, node, err := root.Register(route)
				if err != nil {
					b.Fatal(err)
				}
				node.Value = route
			}

			b.ReportAllocs()

			for b.Loop() {
				node, _ := root.Match(sc.path, nil)
				if node == nil {
					b.Fatal("no match")
				}
			}
		})
	}
}

// BenchmarkMatchConcurrent 测试并发匹配性能
func BenchmarkMatchConcurrent(b *testing.B) {
	root := &matcher.Node[string]{}
	routes := []string{
		"/api/users",
		"/api/{id}",
		"/api/v1/{resource}",
		"/api/v1/{group}/{version}",
	}

	for _, route := range routes {
		_, node, _ := root.Register(route)
		node.Value = route
	}

	paths := []string{
		"/api/users",
		"/api/123",
		"/api/v1/pods",
		"/api/v1/core/v1",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			path := paths[i%len(paths)]
			root.Match(path, nil)
			i++
		}
	})
}
