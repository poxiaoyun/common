package wildcard

import "testing"

func TestWildcardMatchSections(t *testing.T) {
	tests := []struct {
		expr  string
		perm  string
		match bool
	}{
		{expr: "", perm: "zoo:cats:tom:get", match: false},
		{expr: "zoo:list", perm: "", match: false},

		{expr: "zoo:*:**", perm: "zoo:get", match: true},
		{expr: "zoo:*:**", perm: "zoo:get:", match: true},
		{expr: "zoo:*:**", perm: "zoo:get:abc", match: true},
		{expr: "zoo:*:**", perm: "zoo:get:abc:def", match: true},

		{expr: "tom:*", perm: "tom:get", match: true},
		{expr: "tom:*", perm: "tom:", match: false},
		{expr: "tom:*", perm: "tom", match: false},
		{expr: "tom:*", perm: "tom:get:abc", match: false},

		{expr: "tom:*:*", perm: "tom:get", match: false},
		{expr: "tom:*:*", perm: "tom:get:abc", match: true},
		{expr: "tom:*:*", perm: "tom:get:*", match: true},
		{expr: "tom:*:*", perm: "tom:get:*:abc", match: false},

		{expr: "tom:*:foo", perm: "tom:get", match: false},
		{expr: "tom:*:foo", perm: "tom::foo", match: true},
		{expr: "tom:*:foo", perm: "tom:get:foo", match: true},
		{expr: "tom:*:foo", perm: "tom:get:foo:bar", match: false},

		{expr: "zoo:cats:*:get,list", perm: "zoo:cats:tom:remove", match: false},
		{expr: "zoo:cats:*:get,list", perm: "zoo:cats:tom:get", match: true},
		{expr: "zoo:cats:*:get,list", perm: "zoo:remove", match: false},

		{expr: "zoo:**", perm: "zoo:cats:tom:remove", match: true},
		{expr: "zoo:**", perm: "zoo", match: true},
		{expr: "zoo:**", perm: "zoo:cats:tom:remove:abc", match: true},
		{expr: "zoo:**:some-garbage", perm: "zoo:cats:tom:remove", match: true},

		{expr: "zoo:list:*:*", perm: "zoo:list", match: false},
		{expr: "zoo:list:**", perm: "zoo:list", match: true},
		{expr: "zoo:list:*:abc", perm: "zoo:list", match: false},
		{expr: "zoo:list,get:**", perm: "zoo:get", match: true},
		{expr: "zoo:list,get:**", perm: "zoo:kill", match: false},
		{expr: "zoo:list,get,*:**", perm: "zoo:get", match: true},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			if got := Match(tt.expr, tt.perm); got != tt.match {
				t.Errorf("WildcardMatchSections() = %v, want %v", got, tt.match)
			}
		})
	}
}

func BenchmarkMatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Match("zoo:*:**:list,get", "zoo:cats:tom:get")
	}
}
