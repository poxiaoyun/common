package authz_test

import (
	"encoding/json"
	"math"
	"net/netip"
	"testing"
	"time"

	"xiaoshiai.cn/common/authz"
)

func TestPropertiesJSONPreservesPolicyComparisonSemantics(t *testing.T) {
	instant := time.Date(2026, 9, 14, 12, 0, 0, 123456789, time.FixedZone("offset", 8*60*60))
	properties := authz.Properties{
		"visibility": "public",
		"enabled":    true,
		"revision":   int64(math.MaxInt64),
		"instant":    instant,
		"ip":         netip.MustParseAddr("2001:db8::42"),
		"network":    netip.MustParsePrefix("2001:db8::/32"),
		"owner":      authz.ResourceReference{Type: "organizations", ID: "acme"},
		"null":       nil,
	}
	encoded, err := json.Marshal(properties)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a JSON intermediary using IEEE754 numbers. Decimal-string int64
	// literals must survive it without losing revisions above 2^53.
	var intermediary any
	if err := json.Unmarshal(encoded, &intermediary); err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(intermediary)
	if err != nil {
		t.Fatal(err)
	}
	var decoded authz.Properties
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for key, expected := range properties {
		if key == "null" {
			continue
		}
		match, known := authz.ComparePolicyValues(authz.PolicyEqual, decoded[key], expected)
		if !known || !match {
			t.Errorf("%s changed policy equality: got %T(%v), want %T(%v)", key, decoded[key], decoded[key], expected, expected)
		}
	}
	if value, exists := decoded["null"]; !exists || value != nil {
		t.Fatalf("explicit null was lost: %#v", decoded)
	}
	if _, exists := decoded["missing"]; exists {
		t.Fatal("missing fact appeared during decoding")
	}
	match, known := authz.ComparePolicyValues(authz.PolicyGreaterThan, decoded["revision"], int64(math.MaxInt64-1))
	if !known || !match {
		t.Fatal("integer ordering lost precision")
	}
	match, known = authz.ComparePolicyValues(authz.PolicyIPInCIDR, decoded["ip"], decoded["network"])
	if !known || !match {
		t.Fatal("IP/CIDR types no longer support membership")
	}
}

func TestConstraintJSONPreservesTypedPredicateAndUnknown(t *testing.T) {
	constraint := authz.ResourceConstraint{
		Operator: authz.ConstraintProperties,
		Properties: authz.ResourcePropertyConstraint{
			Expression: authz.GreaterThan(authz.ResourceProperty("apps", "revision"), authz.Literal(int64(9007199254740992))),
			Result:     true,
		},
	}
	encoded, err := json.Marshal(constraint)
	if err != nil {
		t.Fatal(err)
	}
	var decoded authz.ResourceConstraint
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		facts authz.Properties
		want  bool
	}{
		{"above", authz.Properties{"revision": int64(9007199254740993)}, true},
		{"equal", authz.Properties{"revision": int64(9007199254740992)}, false},
		{"wrong type", authz.Properties{"revision": "9007199254740993"}, false},
		{"missing", nil, false},
		{"null", authz.Properties{"revision": nil}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := decoded.Properties.Match(authz.Resource{Properties: test.facts}); got != test.want {
				t.Fatalf("predicate = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPropertiesJSONRejectsAmbiguousAndMalformedFacts(t *testing.T) {
	for _, input := range []string{
		`{"value":9007199254740993}`,
		`{"value":{"type":"int64","value":9007199254740993}}`,
		`{"value":{"type":"int64","value":"9223372036854775808"}}`,
		`{"value":{"type":"bool","value":"true"}}`,
		`{"value":{"type":"string","value":null}}`,
		`{"value":{"type":"ip","value":"invalid"}}`,
		`{"value":{"type":"cidr","value":"10.0.0.1"}}`,
		`{"value":{"type":"resourceReference","value":{"type":"repositories"}}}`,
		`{"value":{"type":"float64","value":1}}`,
		`{"value":{"type":"string","value":"public","override":true}}`,
	} {
		t.Run(input, func(t *testing.T) {
			var properties authz.Properties
			if err := json.Unmarshal([]byte(input), &properties); err != nil {
				return
			}
			t.Fatalf("accepted ambiguous facts: %#v", properties)
		})
	}
	for _, value := range []any{float64(1), int(1), []string{"public"}, netip.Addr{}, authz.ResourceReference{Type: "repositories"}} {
		if _, err := json.Marshal(authz.Properties{"value": value}); err != nil {
			continue
		}
		t.Errorf("encoded unsupported fact %T(%v)", value, value)
	}
}

func TestPolicyValueJSONRejectsInvalidUnion(t *testing.T) {
	for _, input := range []string{
		`{"source":"literal","literal":null}`,
		`{"source":"literal","literal":{"type":"string","value":"admin"},"builtin":null}`,
		`{"source":"builtin","builtin":"subject.id","literal":null}`,
		`{"source":"builtin","builtin":"subject.id","literal":{"type":"string","value":"admin"}}`,
		`{"source":"property","property":{"service":"apps","namespace":"resource","name":"owner"},"builtin":"subject.id"}`,
		`{"source":"unknown"}`,
		`{"source":"literal","literal":{"type":"string","value":"admin"},"fallback":"allow"}`,
	} {
		t.Run(input, func(t *testing.T) {
			var value authz.PolicyValue
			if err := json.Unmarshal([]byte(input), &value); err != nil {
				return
			}
			t.Fatalf("accepted invalid policy operand: %#v", value)
		})
	}
}

func TestConstraintJSONRejectsUnknownFieldsAndMissingTruthSet(t *testing.T) {
	for _, input := range []string{
		`{"operator":"all","deny":true}`,
		`{"operator":"properties","properties":{"expression":{"operator":"exists","values":[{"source":"property","property":{"service":"apps","namespace":"resource","name":"visibility"}}]}}}`,
		`{"operator":"properties","properties":{"expression":{"operator":"exists","values":[{"source":"property","property":{"service":"apps","namespace":"resource","name":"visibility"}}]},"result":null}}`,
		`{"operator":"properties","properties":{"expression":{"operator":"exists","unknown":true,"values":[{"source":"property","property":{"service":"apps","namespace":"resource","name":"visibility"}}]},"result":true}}`,
		`{"operator":"related","related":{"relationship":{"service":"iam","name":"member"},"objectProperty":{"service":"apps","namespace":"resource","name":"owner"}}}`,
		`{"operator":"related","related":{"relationship":{"service":"iam","name":"member"},"objectProperty":{"service":"apps","namespace":"resource","name":"owner"},"result":null}}`,
	} {
		t.Run(input, func(t *testing.T) {
			var constraint authz.ResourceConstraint
			if err := json.Unmarshal([]byte(input), &constraint); err != nil {
				return
			}
			t.Fatalf("accepted ambiguous constraint: %#v", constraint)
		})
	}
	var policy authz.Policy
	input := `{"version":"v1","root":{"operator":"all","ignoreDeny":true}}`
	if err := json.Unmarshal([]byte(input), &policy); err != nil {
		return
	}
	t.Fatalf("accepted unknown policy expression fields: %#v", policy)
}
