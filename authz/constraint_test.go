package authz_test

import (
	"strings"
	"testing"

	"xiaoshiai.cn/common/authz"
)

func TestResourceConstraintValidateAcceptsCompleteTree(t *testing.T) {
	constraint := authz.ResourceConstraint{
		Operator: authz.ConstraintAnd,
		Constraints: []authz.ResourceConstraint{
			{
				Operator: authz.ConstraintWithin,
				Scope: authz.Scope{
					authz.ResourceReference{Type: "iam.organization", ID: "organization-1"},
				},
			},
			{
				Operator: authz.ConstraintPathMatches,
				ResourcePath: authz.ResourcePathPattern{
					Path: []authz.ResourceReferencePattern{
						{Type: "iam.organization", ID: "organization-1"},
						{Type: "moha.repository", ID: "*"},
					},
				},
			},
			{
				Operator: authz.ConstraintOr,
				Constraints: []authz.ResourceConstraint{
					{
						Operator: authz.ConstraintProperties,
						Properties: authz.ResourcePropertyConstraint{
							Expression: authz.Equal(authz.ResourceProperty("moha", "visibility"), authz.Literal("public")),
							Result:     true,
						},
					},
					{
						Operator: authz.ConstraintRelated,
						Related: authz.ResourceRelationshipConstraint{
							Relationship: authz.RelationshipReference{Service: "iam", Name: "organization.member"},
							ObjectProperty: authz.PolicyAttributeReference{
								Service:   "moha",
								Namespace: authz.PolicyAttributeResource,
								Name:      "organization",
							},
						},
					},
				},
			},
		},
	}
	if err := constraint.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceConstraintValidateAcceptsConstantsAndEmptyCompositions(t *testing.T) {
	constraints := []authz.ResourceConstraint{
		{},
		{Operator: authz.ConstraintAll},
		{Operator: authz.ConstraintAnd},
		{Operator: authz.ConstraintOr},
		{Operator: authz.ConstraintWithin},
		{
			Operator:     authz.ConstraintPathMatches,
			ResourcePath: authz.ResourcePathPattern{Descendants: true},
		},
		{
			Operator: authz.ConstraintPathMatches,
			ResourcePath: authz.ResourcePathPattern{
				Path: []authz.ResourceReferencePattern{{Type: "moha.repository"}},
			},
		},
	}
	for index := range constraints {
		if err := constraints[index].Validate(); err != nil {
			t.Fatalf("constraint %d: %v", index, err)
		}
	}
}

func TestPropertyPredicateSelectsKnownResult(t *testing.T) {
	condition := authz.Equal(authz.ResourceProperty("catalog", "visibility"), authz.Literal("public"))
	tests := []struct {
		name       string
		properties authz.Properties
		wantTrue   bool
		wantFalse  bool
	}{
		{"public", authz.Properties{"visibility": "public"}, true, false},
		{"private", authz.Properties{"visibility": "private"}, false, true},
		{"missing", nil, false, false},
		{"null", authz.Properties{"visibility": nil}, false, false},
		{"wrong type", authz.Properties{"visibility": true}, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := authz.Resource{Type: "catalog.item", ID: "item", Properties: test.properties}
			positive := authz.ResourcePropertyConstraint{Expression: condition, Result: true}
			negative := authz.ResourcePropertyConstraint{Expression: condition, Result: false}
			if err := positive.Validate(); err != nil {
				t.Fatal(err)
			}
			if positive.Match(resource) != test.wantTrue || negative.Match(resource) != test.wantFalse {
				t.Fatalf("predicate results do not match expected true/false sets")
			}
			present := authz.ResourcePropertyConstraint{
				Expression: authz.Exists(authz.ResourceProperty("catalog", "visibility")),
				Result:     true,
			}
			if present.Match(resource) != (test.name != "missing") {
				t.Fatal("existence confused present null with absence")
			}
		})
	}
}

func TestResourceConstraintValidateRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		name       string
		constraint authz.ResourceConstraint
		want       string
	}{
		{
			name:       "unknown operator",
			constraint: authz.ResourceConstraint{Operator: "future"},
			want:       "unsupported resource constraint operator",
		},
		{
			name: "constant fields",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintAll,
				Scope:    authz.Scope{{Type: "organization", ID: "o1"}},
			},
			want: "cannot carry children or leaf values",
		},
		{
			name:       "not arity",
			constraint: authz.ResourceConstraint{Operator: authz.ConstraintNot},
			want:       "exactly one child",
		},
		{
			name: "incomplete scope",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintWithin,
				Scope:    authz.Scope{{Type: "organization"}},
			},
			want: "complete resource reference",
		},
		{
			name: "missing path",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintPathMatches,
			},
			want: "resource path pattern is required",
		},
		{
			name: "ancestor collection",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintPathMatches,
				ResourcePath: authz.ResourcePathPattern{
					Path: []authz.ResourceReferencePattern{
						{Type: "organization"},
						{Type: "project", ID: "p1"},
					},
				},
			},
			want: "resource ID is required",
		},
		{
			name: "descendant collection",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintPathMatches,
				ResourcePath: authz.ResourcePathPattern{
					Path:        []authz.ResourceReferencePattern{{Type: "organization"}},
					Descendants: true,
				},
			},
			want: "resource ID is required",
		},
		{
			name: "partial wildcard",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintPathMatches,
				ResourcePath: authz.ResourcePathPattern{
					Path: []authz.ResourceReferencePattern{{Type: "moha.*", ID: "r1"}},
				},
			},
			want: "partial wildcard",
		},
		{
			name: "invalid properties",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintProperties,
				Properties: authz.ResourcePropertyConstraint{
					Expression: authz.PolicyExpression{Operator: authz.PolicyEqual},
				},
			},
			want: "two values",
		},
		{
			name: "request relationship property",
			constraint: authz.ResourceConstraint{
				Operator: authz.ConstraintRelated,
				Related: authz.ResourceRelationshipConstraint{
					Relationship: authz.RelationshipReference{Service: "iam", Name: "organization.member"},
					ObjectProperty: authz.PolicyAttributeReference{
						Service:   "moha",
						Namespace: authz.PolicyAttributeRequest,
						Name:      "organization",
					},
				},
			},
			want: "resource object property",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.constraint.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
