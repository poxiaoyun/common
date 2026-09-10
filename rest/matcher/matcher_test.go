package matcher_test

import (
	"reflect"
	"regexp"
	"slices"
	"testing"

	"xiaoshiai.cn/common/rest/matcher"
)

func TestMatchCandidateReceivesCompleteVariables(t *testing.T) {
	root := &matcher.Node[string]{}
	for _, pattern := range []string{"/org/{org}/repos/{name}.git", "/{path}*"} {
		_, node, err := root.Register(pattern)
		if err != nil {
			t.Fatal(err)
		}
		node.Value = pattern
	}
	var seen [][]matcher.MatchVar
	node, vars := root.Match("/org/team/repos/model.git", func(pattern string, vars []matcher.MatchVar) bool {
		seen = append(seen, slices.Clone(vars))
		return pattern == "/{path}*"
	})
	want := [][]matcher.MatchVar{
		{{Name: "org", Value: "team"}, {Name: "name", Value: "model"}},
		{{Name: "path", Value: "org/team/repos/model.git"}},
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("candidate variables = %v, want %v", seen, want)
	}
	if node == nil || node.Value != "/{path}*" || !reflect.DeepEqual(vars, want[1]) {
		t.Fatalf("fallback result = %v, %v", node, vars)
	}
}

func TestMatchAnchorsFinalLiteralSuffix(t *testing.T) {
	for _, test := range []struct {
		pattern, path string
		want          []matcher.MatchVar
	}{
		{pattern: "/{repository}.git", path: "/model.git.backup.git", want: []matcher.MatchVar{{Name: "repository", Value: "model.git.backup"}}},
		{pattern: "/{repository}.git/{rest}*", path: "/model.git.backup.git/info/refs", want: []matcher.MatchVar{{Name: "repository", Value: "model.git.backup"}, {Name: "rest", Value: "info/refs"}}},
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

func TestCompileSection(t *testing.T) {
	tests := []struct {
		name    string
		want    []matcher.Section
		wantErr bool
	}{
		{
			name: "/assets/prefix-*.css",
			want: []matcher.Section{
				{{Pattern: "/assets"}},
				{
					{Pattern: "/prefix-", Greedy: true},
					{Pattern: ".css"},
				},
			},
		},
		{
			name: "/zoo/tom",
			want: []matcher.Section{
				{{Pattern: "/zoo"}},
				{{Pattern: "/tom"}},
			},
		},
		{
			name: "/v1/proxy*",
			want: []matcher.Section{
				{{Pattern: "/v1"}},
				{{Pattern: "/proxy", Greedy: true}},
			},
		},

		{
			name: "/api/v{version}/{name}*",
			want: []matcher.Section{
				{{Pattern: "/api"}},
				{{Pattern: "/v"}, {Pattern: "{version}", VarName: "version"}},
				{{Pattern: "/"}, {Pattern: "{name}", VarName: "name", Greedy: true}},
			},
		},
		{
			name: "/{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}*/manifests/{reference}",
			want: []matcher.Section{
				{
					{Pattern: "/"},
					{
						Pattern: "{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}", VarName: "repository", Greedy: true,
						Validate: regexp.MustCompile(`^(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+$`),
					},
					{Pattern: "/manifests"},
					{Pattern: "/"},
					{Pattern: "{reference}", VarName: "reference"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matcher.CompilePattern(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compile() = %v, want %v", got, tt.want)
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
				"/api/{path}*",
				"/api/v1/{path}*",
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
				"/a/{a}/b/{b}*",
				"/a/{a}/b/{b}*/index",
				"/a/{a}/b/{b}*/manifests/{c}",
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
				"/api/{path}*",
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
