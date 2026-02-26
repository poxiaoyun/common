package matcher

import (
	"reflect"
	"regexp"
	"testing"
)

func TestCompileSection(t *testing.T) {
	tests := []struct {
		name    string
		want    []Section
		wantErr bool
	}{
		{
			name: "/assets/prefix-*.css",
			want: []Section{
				{{Pattern: "/assets"}},
				{
					{Pattern: "/prefix-", Greedy: true},
					{Pattern: ".css"},
				},
			},
		},
		{
			name: "/zoo/tom",
			want: []Section{
				{{Pattern: "/zoo"}},
				{{Pattern: "/tom"}},
			},
		},
		{
			name: "/v1/proxy*",
			want: []Section{
				{{Pattern: "/v1"}},
				{{Pattern: "/proxy", Greedy: true}},
			},
		},

		{
			name: "/api/v{version}/{name}*",
			want: []Section{
				{{Pattern: "/api"}},
				{{Pattern: "/v"}, {Pattern: "{version}", VarName: "version"}},
				{{Pattern: "/"}, {Pattern: "{name}", VarName: "name", Greedy: true}},
			},
		},
		{
			name: "/{repository:(?:[a-zA-Z0-9]+(?:[._-][a-zA-Z0-9]+)*/?)+}*/manifests/{reference}",
			want: []Section{
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
			got, err := CompilePattern(tt.name)
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

func TestSection_score(t *testing.T) {
	tests := []struct {
		a  string
		b  string
		eq int // 1: a 优先级更高, -1: b 优先级更高, 0: 相同
	}{
		{a: "a", b: "{a}", eq: 1},                // 常量 > 变量
		{a: "api", b: "{a}", eq: 1},              // 常量 > 变量
		{a: "v{a}*", b: "{a}", eq: 1},            // 有常量前缀的贪婪 > 纯变量（因为有常量 "v"）
		{a: "/", b: "/{a}", eq: 1},               // 根路径 > 变量
		{a: "/", b: "/a", eq: 1},                 // 根路径特殊处理，优先级更高
		{a: "{a}*", b: "{a}*:action", eq: -1},    // 更少常量 < 更多常量
		{a: "/{a}*", b: "/{a}*/foo/{b}", eq: -1}, // 更少常量 < 更多常量
	}
	for _, tt := range tests {
		t.Run(tt.a, func(t *testing.T) {
			seca, _ := Compile(tt.a)
			secb, _ := Compile(tt.b)

			// 使用优化后的评分机制
			// compareSectionOptimized 返回：< 0 表示 a 优先级更高，> 0 表示 b 优先级更高
			cmp := compareSectionOptimized(seca, secb)

			// 转换为测试期望的格式
			var result int
			if cmp < 0 {
				result = 1 // a 优先级更高
			} else if cmp > 0 {
				result = -1 // b 优先级更高
			} else {
				result = 0 // 相同
			}

			if result != tt.eq {
				scorea := seca.detailedScore()
				scoreb := secb.detailedScore()
				t.Errorf("compareSectionOptimized() result = %v, want %v\n  a=%s: %+v\n  b=%s: %+v",
					result, tt.eq, tt.a, scorea, tt.b, scoreb)
			}
		})
	}
}

func TestCompileError_Error(t *testing.T) {
	tests := []struct {
		name   string
		fields CompileError
		want   string
	}{
		{
			fields: CompileError{
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
				t.Errorf("CompileError.Error() = %v, want %v", got, tt.want)
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
			root := &Node[string]{}
			for _, route := range sc.routes {
				_, node, err := root.Register(route)
				if err != nil {
					b.Fatal(err)
				}
				node.Value = route
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
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
	root := &Node[string]{}
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
