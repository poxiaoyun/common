package pattern_test

import (
	"strings"
	"testing"

	"xiaoshiai.cn/common/pattern"
)

var (
	wildcardSink pattern.Wildcard
	matchSink    bool
)

func TestWildcardMatch(t *testing.T) {
	type matchCase struct {
		expression string
		value      string
		match      bool
	}
	groups := []struct {
		name    string
		options pattern.WildcardOptions
		cases   []matchCase
	}{
		{
			name:    "colon separator",
			options: pattern.WildcardOptions{Separator: ':'},
			cases: []matchCase{
				{expression: "", value: "zoo:cats:tom:get", match: false},
				{expression: "zoo:list", value: "", match: false},
				{expression: "zoo:*:**", value: "zoo:get", match: true},
				{expression: "zoo:*:**", value: "zoo:get:", match: false},
				{expression: "zoo:*:**", value: "zoo:get:abc", match: true},
				{expression: "zoo:*:**", value: "zoo:get:abc:def", match: true},
				{expression: "tom:*", value: "tom:get", match: true},
				{expression: "tom:*", value: "tom:", match: false},
				{expression: "tom:*", value: "tom", match: false},
				{expression: "tom:*", value: "tom:get:abc", match: false},
				{expression: "tom:*:*", value: "tom:get", match: false},
				{expression: "tom:*:*", value: "tom:get:abc", match: true},
				{expression: "tom:*:*", value: "tom:get:*", match: true},
				{expression: "tom:*:*", value: "tom:get:*:abc", match: false},
				{expression: "tom:*:foo", value: "tom:get", match: false},
				{expression: "tom:*:foo", value: "tom::foo", match: false},
				{expression: "tom:*:foo", value: "tom:get:foo", match: true},
				{expression: "tom:*:foo", value: "tom:get:foo:bar", match: false},
				{expression: "tom:g*", value: "tom:get", match: true},
				{expression: "tom:*et", value: "tom:get", match: true},
				{expression: "tom:g*t", value: "tom:get", match: true},
				{expression: "tom:g*t", value: "tom:go", match: false},
				{expression: "zoo:**", value: "zoo:cats:tom:remove", match: true},
				{expression: "zoo:**", value: "zoo", match: true},
				{expression: "zoo:**", value: "zoo:cats:tom:remove:abc", match: true},
				{expression: "zoo:**:some-garbage", value: "zoo:cats:tom:remove", match: true},
				{expression: "zoo:**:some-garbage", value: "zoo", match: true},
				{expression: "zoo:list:*:*", value: "zoo:list", match: false},
				{expression: "zoo:list:**", value: "zoo:list", match: true},
				{expression: "zoo:list:*:abc", value: "zoo:list", match: false},
			},
		},
		{
			name:    "alternatives",
			options: pattern.WildcardOptions{Separator: ':'},
			cases: []matchCase{
				{expression: "zoo:cats:*:get,list", value: "zoo:cats:tom:remove", match: false},
				{expression: "zoo:cats:*:get,list", value: "zoo:cats:tom:get", match: true},
				{expression: "zoo:cats:*:get,list", value: "zoo:remove", match: false},
				{expression: "zoo:list,get:**", value: "zoo:get", match: true},
				{expression: "zoo:list,get:**", value: "zoo:kill", match: false},
				{expression: "zoo:{list,get}:**", value: "zoo:get", match: true},
				{expression: "zoo:{list,get}:**", value: "zoo:kill", match: false},
				{expression: "zoo:list,get,*:**", value: "zoo:get", match: true},
			},
		},
		{
			name:    "dot separator",
			options: pattern.WildcardOptions{Separator: '.'},
			cases: []matchCase{
				{expression: "order.created.v1", value: "order.created.v1", match: true},
				{expression: "order.*.v1", value: "order.created.v1", match: true},
				{expression: "order.*.v1", value: "order.eu.created.v1", match: false},
				{expression: "order.**", value: "order", match: true},
				{expression: "order.**", value: "order.created.v1", match: true},
				{expression: "order.**.ignored", value: "order.created.v1", match: true},
				{expression: "order.**.ignored", value: "order", match: true},
				{expression: "order.**.ignored", value: "user.created.v1", match: false},
				{expression: "order..v1", value: "order..v1", match: true},
				{expression: "order.*.v1", value: "order..v1", match: false},
				{expression: "order.**", value: "order..v1", match: false},
			},
		},
	}
	for _, group := range groups {
		t.Run(group.name, func(t *testing.T) {
			for _, test := range group.cases {
				t.Run(test.expression+"/"+test.value, func(t *testing.T) {
					compiled, err := pattern.CompileWildcard(test.expression, group.options)
					if err != nil {
						t.Fatalf("CompileWildcard() error = %v", err)
					}
					if got := compiled.Match(test.value); got != test.match {
						t.Errorf("Match() = %v, want %v", got, test.match)
					}
				})
			}
		})
	}
}

func TestWildcardDoesNotInterpretValueAsExpression(t *testing.T) {
	options := pattern.WildcardOptions{Separator: ':'}
	tests := []struct {
		expression string
		value      string
	}{
		{expression: "tenant:alice:read", value: "tenant:*:read"},
		{expression: "tenant:alice:read", value: "tenant:**:read"},
		{expression: "tenant:alice:read", value: "tenant:alice,bob:read"},
		{expression: "tenant:alice:read", value: "tenant:{alice}:read"},
		{expression: "tenant:*:read", value: "tenant::read"},
		{expression: "tenant:*:read", value: "tenant:alice:admin:read"},
		{expression: "tenant:alice:read", value: "tenant：alice：read"},
		{expression: "tenant:alice:read", value: "tenant%3Aalice%3Aread"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			compiled, err := pattern.CompileWildcard(test.expression, options)
			if err != nil {
				t.Fatalf("CompileWildcard() error = %v", err)
			}
			if compiled.Match(test.value) {
				t.Fatal("Match() = true, want false")
			}
		})
	}
}

func FuzzWildcardMatch(f *testing.F) {
	f.Add("tenant:alice:read", "tenant:**:read", byte(':'))
	f.Add("tenant:alice:read", "tenant:{alice,bob}:read", byte(':'))
	f.Add("order.foo*", "order.foobar", byte('.'))
	f.Add("order..v1", "order..v1", byte('.'))
	f.Add("", "\x00:*:{value}", byte(':'))

	f.Fuzz(func(t *testing.T, expression, value string, separator byte) {
		compiled, err := pattern.CompileWildcard(expression, pattern.WildcardOptions{Separator: separator})
		if err != nil {
			return
		}
		matched := compiled.Match(value)
		if !strings.ContainsAny(expression, "*,{}") && matched != (expression == value) {
			t.Fatalf("literal expression %q matching %q = %v", expression, value, matched)
		}
	})
}

func TestCompileWildcardOptions(t *testing.T) {
	tests := []struct {
		name    string
		options pattern.WildcardOptions
	}{
		{
			name:    "empty separator",
			options: pattern.WildcardOptions{},
		},
		{
			name:    "comma separator",
			options: pattern.WildcardOptions{Separator: ','},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := pattern.CompileWildcard("order.created", test.options); err == nil {
				t.Fatal("CompileWildcard() error = nil")
			}
		})
	}
}

func TestCompileWildcardAllowsNonFinalDoubleStar(t *testing.T) {
	compiled, err := pattern.CompileWildcard(
		"zoo:**:some-garbage",
		pattern.WildcardOptions{Separator: ':'},
	)
	if err != nil {
		t.Fatalf("CompileWildcard() error = %v", err)
	}
	if !compiled.Match("zoo:cats:tom:remove") {
		t.Fatal("Match() = false, want true")
	}
}

func TestWildcardDoesNotAllocate(t *testing.T) {
	options := pattern.WildcardOptions{Separator: ':'}
	compiled, err := pattern.CompileWildcard("zoo:*:**:ignored", options)
	if err != nil {
		t.Fatalf("CompileWildcard() error = %v", err)
	}
	matchAllocations := testing.AllocsPerRun(1000, func() {
		matchSink = compiled.Match("zoo:cats:tom:get")
	})
	if matchAllocations != 0 {
		t.Fatalf("Match() allocations = %v, want 0", matchAllocations)
	}

	compileAllocations := testing.AllocsPerRun(1000, func() {
		var compileErr error
		wildcardSink, compileErr = pattern.CompileWildcard("zoo:*:**:ignored", options)
		if compileErr != nil {
			panic(compileErr)
		}
	})
	if compileAllocations != 0 {
		t.Fatalf("CompileWildcard() allocations = %v, want 0", compileAllocations)
	}
}

func BenchmarkWildcardMatch(b *testing.B) {
	compiled, err := pattern.CompileWildcard("zoo:*:**:list,get", pattern.WildcardOptions{Separator: ':'})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		matchSink = compiled.Match("zoo:cats:tom:get")
	}
}

func BenchmarkCompileWildcard(b *testing.B) {
	options := pattern.WildcardOptions{Separator: ':'}
	b.ReportAllocs()
	for b.Loop() {
		var err error
		wildcardSink, err = pattern.CompileWildcard("zoo:*:**:list,get", options)
		if err != nil {
			b.Fatal(err)
		}
	}
}
