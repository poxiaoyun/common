package authz

import (
	"fmt"
	"strings"

	"xiaoshiai.cn/common/selector"
)

// ResourceConstraintOperator identifies one candidate-resource constraint
// operation.
type ResourceConstraintOperator string

const (
	// ConstraintNone matches no candidate resource and is the zero-value operator.
	ConstraintNone ResourceConstraintOperator = ""
	// ConstraintAll matches every candidate resource.
	ConstraintAll ResourceConstraintOperator = "all"
	// ConstraintAnd requires every child constraint to match.
	ConstraintAnd ResourceConstraintOperator = "and"
	// ConstraintOr requires at least one child constraint to match.
	ConstraintOr ResourceConstraintOperator = "or"
	// ConstraintNot negates its single child constraint.
	ConstraintNot ResourceConstraintOperator = "not"
	// ConstraintWithin matches candidates whose full resource path is at or
	// below Scope.
	ConstraintWithin ResourceConstraintOperator = "within"
	// ConstraintPathMatches matches candidates whose full resource path matches
	// ResourcePath.
	ConstraintPathMatches ResourceConstraintOperator = "pathMatches"
	// ConstraintProperties matches candidates whose registered properties
	// satisfy Properties.
	ConstraintProperties ResourceConstraintOperator = "properties"
	// ConstraintRelated matches candidates whose ObjectProperty has the named
	// relationship to the evaluated Subject.
	ConstraintRelated ResourceConstraintOperator = "related"
)

// ResourceReferencePattern matches one element in a complete resource path.
// Type and ID accept either one exact value or "*". An empty ID is permitted
// only for the terminal element of a non-descendant pattern and selects that
// resource collection.
type ResourceReferencePattern struct {
	Type string
	ID   string
}

// ResourcePathPattern matches one complete candidate resource path. When
// Descendants is true, Path is a prefix and every supplied element must include
// an ID. An empty descendant Path matches every resource path below root.
type ResourcePathPattern struct {
	Path        []ResourceReferencePattern
	Descendants bool
}

// ResourceRelationshipConstraint tests a relationship from the evaluated
// Subject to a candidate ResourceReference property.
type ResourceRelationshipConstraint struct {
	Relationship   RelationshipReference
	ObjectProperty PolicyAttributeReference
}

// ResourceConstraint is one closed recursive candidate-resource selection
// node. Operator determines which of the remaining fields is active. Its zero
// value is ConstraintNone and matches no candidate resource.
type ResourceConstraint struct {
	Operator     ResourceConstraintOperator
	Constraints  []ResourceConstraint
	Scope        Scope
	ResourcePath ResourcePathPattern
	Properties   selector.Requirement
	Related      ResourceRelationshipConstraint
}

// Validate verifies the operator-specific field shape and every nested
// constraint.
func (constraint ResourceConstraint) Validate() error {
	switch constraint.Operator {
	case ConstraintNone, ConstraintAll:
		if !constraint.emptyExceptOperator() {
			return fmt.Errorf("resource constraint %q cannot carry children or leaf values", constraint.Operator)
		}
	case ConstraintAnd, ConstraintOr:
		if !constraint.emptyLeaves() {
			return fmt.Errorf("resource constraint %q cannot carry leaf values", constraint.Operator)
		}
		if err := validateResourceConstraints(constraint.Constraints); err != nil {
			return fmt.Errorf("resource constraint %q: %w", constraint.Operator, err)
		}
	case ConstraintNot:
		if len(constraint.Constraints) != 1 || !constraint.emptyLeaves() {
			return fmt.Errorf("resource constraint %q requires exactly one child and no leaf values", constraint.Operator)
		}
		if err := constraint.Constraints[0].Validate(); err != nil {
			return fmt.Errorf("resource constraint %q child: %w", constraint.Operator, err)
		}
	case ConstraintWithin:
		if len(constraint.Constraints) != 0 || !constraint.emptyLeavesExceptScope() {
			return fmt.Errorf("resource constraint %q requires only a scope", constraint.Operator)
		}
		if err := validateConstraintScope(constraint.Scope); err != nil {
			return fmt.Errorf("resource constraint %q: %w", constraint.Operator, err)
		}
	case ConstraintPathMatches:
		if len(constraint.Constraints) != 0 || !constraint.emptyLeavesExceptResourcePath() {
			return fmt.Errorf("resource constraint %q requires only a resource path", constraint.Operator)
		}
		if err := constraint.ResourcePath.Validate(); err != nil {
			return fmt.Errorf("resource constraint %q: %w", constraint.Operator, err)
		}
	case ConstraintProperties:
		if len(constraint.Constraints) != 0 || !constraint.emptyLeavesExceptProperties() {
			return fmt.Errorf("resource constraint %q requires only properties", constraint.Operator)
		}
		if err := constraint.Properties.Validate(); err != nil {
			return fmt.Errorf("resource constraint %q: %w", constraint.Operator, err)
		}
	case ConstraintRelated:
		if len(constraint.Constraints) != 0 || !constraint.emptyLeavesExceptRelated() {
			return fmt.Errorf("resource constraint %q requires only a relationship", constraint.Operator)
		}
		if err := constraint.Related.Validate(); err != nil {
			return fmt.Errorf("resource constraint %q: %w", constraint.Operator, err)
		}
	default:
		return fmt.Errorf("unsupported resource constraint operator %q", constraint.Operator)
	}
	return nil
}

// Validate verifies the structure of a resource path pattern.
func (pattern ResourcePathPattern) Validate() error {
	if len(pattern.Path) == 0 && !pattern.Descendants {
		return fmt.Errorf("resource path pattern is required")
	}
	for index, reference := range pattern.Path {
		if err := validatePatternToken("type", string(reference.Type), false); err != nil {
			return fmt.Errorf("resource path element %d: %w", index, err)
		}
		allowCollection := index == len(pattern.Path)-1 && !pattern.Descendants
		if err := validatePatternToken("ID", string(reference.ID), allowCollection); err != nil {
			return fmt.Errorf("resource path element %d: %w", index, err)
		}
	}
	return nil
}

// Validate verifies the relationship and resource-property reference.
func (relationship ResourceRelationshipConstraint) Validate() error {
	if relationship.Relationship.Service == "" || relationship.Relationship.Name == "" {
		return fmt.Errorf("complete relationship reference is required")
	}
	property := relationship.ObjectProperty
	if property.Service == "" || property.Namespace != PolicyAttributeResource || property.Name == "" {
		return fmt.Errorf("complete resource object property is required")
	}
	return nil
}

func validateResourceConstraints(constraints []ResourceConstraint) error {
	for index := range constraints {
		if err := constraints[index].Validate(); err != nil {
			return fmt.Errorf("child %d: %w", index, err)
		}
	}
	return nil
}

func validateConstraintScope(scope Scope) error {
	for index, reference := range scope {
		if reference.Type == "" || reference.ID == "" {
			return fmt.Errorf("scope element %d requires a complete resource reference", index)
		}
	}
	return nil
}

func validatePatternToken(field, value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("resource %s is required", field)
	}
	if value != "*" && strings.Contains(value, "*") {
		return fmt.Errorf("resource %s %q contains a partial wildcard", field, value)
	}
	return nil
}

func (constraint ResourceConstraint) emptyExceptOperator() bool {
	return len(constraint.Constraints) == 0 && constraint.emptyLeaves()
}

func (constraint ResourceConstraint) emptyLeaves() bool {
	return len(constraint.Scope) == 0 && constraint.emptyLeavesExceptScope()
}

func (constraint ResourceConstraint) emptyLeavesExceptScope() bool {
	return constraint.resourcePathEmpty() && constraint.propertiesEmpty() && constraint.Related == (ResourceRelationshipConstraint{})
}

func (constraint ResourceConstraint) emptyLeavesExceptResourcePath() bool {
	return len(constraint.Scope) == 0 && constraint.propertiesEmpty() && constraint.Related == (ResourceRelationshipConstraint{})
}

func (constraint ResourceConstraint) emptyLeavesExceptProperties() bool {
	return len(constraint.Scope) == 0 && constraint.resourcePathEmpty() && constraint.Related == (ResourceRelationshipConstraint{})
}

func (constraint ResourceConstraint) emptyLeavesExceptRelated() bool {
	return len(constraint.Scope) == 0 && constraint.resourcePathEmpty() && constraint.propertiesEmpty()
}

func (constraint ResourceConstraint) resourcePathEmpty() bool {
	return len(constraint.ResourcePath.Path) == 0 && !constraint.ResourcePath.Descendants
}

func (constraint ResourceConstraint) propertiesEmpty() bool {
	return constraint.Properties.Operator == selector.None && constraint.Properties.Key == "" && len(constraint.Properties.Values) == 0 && len(constraint.Properties.Requirements) == 0
}
