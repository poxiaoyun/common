package authz_test

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"xiaoshiai.cn/common/authz"
)

func TestPolicyBuildersDescribeResourceAndRelationshipFacts(t *testing.T) {
	moha := authz.ServiceID("moha")
	iam := authz.ServiceID("iam")
	policy := authz.NewPolicy(authz.Any(
		authz.Equal(authz.ResourceProperty(moha, "visibility"), authz.Literal("public")),
		authz.All(
			authz.Equal(authz.ResourceProperty(moha, "visibility"), authz.Literal("internal")),
			authz.Related(
				authz.RelationshipReference{Service: iam, Name: "organization.member"},
				authz.ResourceProperty(moha, "organization"),
			),
		),
	))

	if policy.Version != authz.PolicyVersionV1 || policy.Root.Operator != authz.PolicyAny {
		t.Fatalf("policy = %#v", policy)
	}
	related := policy.Root.Expressions[1].Expressions[1]
	if related.Operator != authz.PolicyRelated || related.Relationship.Service != iam {
		t.Fatalf("related expression = %#v", related)
	}
	if related.Values[0].Property.Namespace != authz.PolicyAttributeResource {
		t.Fatalf("related object = %#v", related.Values[0])
	}
}

func TestEmptyPolicyCompositionsRemainExplicit(t *testing.T) {
	all := authz.All()
	if all.Operator != authz.PolicyAll || len(all.Expressions) != 0 {
		t.Fatalf("All() = %#v", all)
	}

	any := authz.Any()
	if any.Operator != authz.PolicyAny || len(any.Expressions) != 0 {
		t.Fatalf("Any() = %#v", any)
	}
}

func TestPolicyValidateAcceptsCompleteExpressionTree(t *testing.T) {
	policy := authz.NewPolicy(authz.Any(
		authz.Equal(authz.ResourceProperty("moha", "visibility"), authz.Literal("public")),
		authz.Not(authz.Exists(authz.RequestProperty("moha", "blocked"))),
		authz.Related(
			authz.RelationshipReference{Service: "iam", Name: "organization.member"},
			authz.ResourceProperty("moha", "organization"),
		),
	))
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyValidateAcceptsEveryLiteralType(t *testing.T) {
	literals := []any{
		true,
		"public",
		int64(42),
		time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC),
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParsePrefix("192.0.2.0/24"),
		authz.ResourceReference{Type: "iam.organization", ID: "organization-1"},
	}
	for _, literal := range literals {
		if err := authz.NewPolicy(authz.Equal(authz.Literal(literal), authz.Literal(literal))).Validate(); err != nil {
			t.Fatalf("literal %T: %v", literal, err)
		}
	}
}

func TestPolicyValidateRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		name   string
		policy authz.Policy
		want   string
	}{
		{
			name:   "version",
			policy: authz.Policy{Version: "future", Root: authz.All()},
			want:   "unsupported policy version",
		},
		{
			name: "unknown operator",
			policy: authz.Policy{Version: authz.PolicyVersionV1, Root: authz.PolicyExpression{
				Operator: "execute",
			}},
			want: "unsupported operator",
		},
		{
			name: "not arity",
			policy: authz.Policy{Version: authz.PolicyVersionV1, Root: authz.PolicyExpression{
				Operator: authz.PolicyNot,
			}},
			want: "exactly one child expression",
		},
		{
			name: "comparison arity",
			policy: authz.Policy{Version: authz.PolicyVersionV1, Root: authz.PolicyExpression{
				Operator: authz.PolicyEqual,
				Values:   []authz.PolicyValue{authz.Literal("public")},
			}},
			want: "exactly two values",
		},
		{
			name: "relationship reference",
			policy: authz.Policy{Version: authz.PolicyVersionV1, Root: authz.PolicyExpression{
				Operator: authz.PolicyRelated,
				Values:   []authz.PolicyValue{authz.ResourceProperty("moha", "organization")},
			}},
			want: "complete relationship reference",
		},
		{
			name: "nested invalid value",
			policy: authz.NewPolicy(authz.All(authz.Equal(
				authz.PolicyValue{Source: authz.PolicyValueBuiltin, Builtin: "unknown"},
				authz.Literal("public"),
			))),
			want: "supported builtin",
		},
		{
			name: "property reference",
			policy: authz.NewPolicy(authz.Exists(authz.PolicyValue{
				Source: authz.PolicyValueProperty,
				Property: authz.PolicyAttributeReference{
					Service: "moha",
					Name:    "visibility",
				},
			})),
			want: "complete property reference",
		},
		{
			name:   "nil literal",
			policy: authz.NewPolicy(authz.Exists(authz.Literal(nil))),
			want:   "non-nil literal",
		},
		{
			name:   "unsupported literal",
			policy: authz.NewPolicy(authz.Exists(authz.Literal(float64(1)))),
			want:   "unsupported type",
		},
		{
			name:   "exists literal",
			policy: authz.NewPolicy(authz.Exists(authz.Literal("value"))),
			want:   "requires a property value",
		},
		{
			name: "nonliteral set value",
			policy: authz.NewPolicy(authz.In(
				authz.ResourceProperty("moha", "visibility"),
				authz.RequestProperty("moha", "visibility"),
			)),
			want: "must be a literal",
		},
		{
			name: "heterogeneous set",
			policy: authz.NewPolicy(authz.In(
				authz.ResourceProperty("moha", "visibility"),
				authz.Literal("public"),
				authz.Literal(int64(1)),
			)),
			want: "set values must have one type",
		},
		{
			name:   "non-string prefix",
			policy: authz.NewPolicy(authz.StartsWith(authz.Literal(int64(1)), authz.Literal("1"))),
			want:   "requires string",
		},
		{
			name: "invalid CIDR operand",
			policy: authz.NewPolicy(authz.IPInCIDR(
				authz.Literal(netip.MustParseAddr("192.0.2.1")),
				authz.Literal("192.0.2.0/24"),
			)),
			want: "requires cidr",
		},
		{
			name: "invalid relationship object",
			policy: authz.NewPolicy(authz.Related(
				authz.RelationshipReference{Service: "iam", Name: "organization.member"},
				authz.Literal("organization-1"),
			)),
			want: "requires a resource reference object",
		},
		{
			name: "incomplete resource reference literal",
			policy: authz.NewPolicy(authz.Exists(authz.Literal(authz.ResourceReference{
				Type: "iam.organization",
			}))),
			want: "unsupported type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
