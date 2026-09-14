package authz_test

import (
	"net/netip"
	"testing"
	"time"

	"xiaoshiai.cn/common/authz"
)

func TestComparePolicyValuesPreservesTypesAndInstants(t *testing.T) {
	instant := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		operator      authz.PolicyOperator
		values        []any
		result, known bool
	}{
		{"exact integer", authz.PolicyEqual, []any{int64(9007199254740993), int64(9007199254740993)}, true, true},
		{"distinct integers", authz.PolicyEqual, []any{int64(9007199254740993), int64(9007199254740992)}, false, true},
		{"string not coerced", authz.PolicyNotEqual, []any{"1", int64(1)}, false, false},
		{"bool not coerced", authz.PolicyEqual, []any{true, "true"}, false, false},
		{"null unknown", authz.PolicyNotEqual, []any{nil, "public"}, false, false},
		{"same instant", authz.PolicyEqual, []any{instant, instant.In(time.FixedZone("offset", 3600))}, true, true},
		{"ordered instant", authz.PolicyLessThan, []any{instant, instant.Add(time.Second)}, true, true},
		{"numeric ordering", authz.PolicyGreaterThan, []any{int64(10), int64(2)}, true, true},
		{"set", authz.PolicyIn, []any{"private", "internal", "public"}, false, true},
		{"empty negated set", authz.PolicyNotIn, []any{"private"}, true, true},
		{"empty set with unsupported number", authz.PolicyIn, []any{float64(1)}, false, false},
		{"empty negated set with unsupported number", authz.PolicyNotIn, []any{float64(1)}, false, false},
		{"empty negated set with unsupported object", authz.PolicyNotIn, []any{map[string]any{"value": "private"}}, false, false},
		{"prefix", authz.PolicyStartsWith, []any{"release-1", "release-"}, true, true},
		{"suffix", authz.PolicyEndsWith, []any{"artifact.json", ".json"}, true, true},
		{"network", authz.PolicyIPInCIDR, []any{netip.MustParseAddr("10.0.0.3"), netip.MustParsePrefix("10.0.0.0/24")}, true, true},
		{"reference", authz.PolicyEqual, []any{authz.ResourceReference{Type: "org", ID: "one"}, authz.ResourceReference{Type: "org", ID: "two"}}, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, known := authz.ComparePolicyValues(test.operator, test.values...)
			if known != test.known || known && result != test.result {
				t.Fatalf("comparison = (%v,%v), want (%v,%v)", result, known, test.result, test.known)
			}
		})
	}
}
