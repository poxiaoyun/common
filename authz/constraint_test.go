package authz_test

import (
	"strings"
	"testing"

	"xiaoshiai.cn/common/authz"
	"xiaoshiai.cn/common/selector"
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
						Properties: selector.Requirement{
							Operator: selector.Equals,
							Key:      "visibility",
							Values:   []any{"public"},
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
				Properties: selector.Requirement{
					Operator: selector.Equals,
					Key:      "visibility",
				},
			},
			want: "one value",
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
