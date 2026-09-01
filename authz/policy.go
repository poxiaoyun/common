package authz

import (
	"fmt"
	"net/netip"
	"time"
)

// PolicyVersion identifies one closed Policy expression vocabulary.
type PolicyVersion string

const (
	// PolicyVersionV1 identifies the initial Policy expression vocabulary.
	PolicyVersionV1 PolicyVersion = "v1"
)

// Policy is one versioned authorization expression.
type Policy struct {
	Version PolicyVersion
	Root    PolicyExpression
}

// NewPolicy constructs a Policy using the current expression vocabulary.
func NewPolicy(root PolicyExpression) Policy {
	return Policy{Version: PolicyVersionV1, Root: root}
}

// Validate verifies the versioned expression and value shapes of Policy.
func (policy Policy) Validate() error {
	if policy.Version != PolicyVersionV1 {
		return fmt.Errorf("unsupported policy version %q", policy.Version)
	}
	if err := validatePolicyExpression(policy.Root, "root"); err != nil {
		return fmt.Errorf("policy: %w", err)
	}
	return nil
}

// PolicyOperator identifies one PolicyExpression operation.
type PolicyOperator string

const (
	// PolicyAny is true when at least one child is true.
	PolicyAny PolicyOperator = "any"
	// PolicyAll is true when every child is true.
	PolicyAll PolicyOperator = "all"
	// PolicyNot negates one child expression.
	PolicyNot PolicyOperator = "not"
	// PolicyExists tests whether one registered property is present.
	PolicyExists PolicyOperator = "exists"
	// PolicyEqual compares two values for exact equality.
	PolicyEqual PolicyOperator = "equal"
	// PolicyNotEqual compares two values for exact inequality.
	PolicyNotEqual PolicyOperator = "notEqual"
	// PolicyLessThan compares two ordered values.
	PolicyLessThan PolicyOperator = "lessThan"
	// PolicyLessThanOrEqual compares two ordered values.
	PolicyLessThanOrEqual PolicyOperator = "lessThanOrEqual"
	// PolicyGreaterThan compares two ordered values.
	PolicyGreaterThan PolicyOperator = "greaterThan"
	// PolicyGreaterThanOrEqual compares two ordered values.
	PolicyGreaterThanOrEqual PolicyOperator = "greaterThanOrEqual"
	// PolicyIn tests membership in homogeneous literal operands.
	PolicyIn PolicyOperator = "in"
	// PolicyNotIn negates membership in homogeneous literal operands.
	PolicyNotIn PolicyOperator = "notIn"
	// PolicyStartsWith compares a string prefix.
	PolicyStartsWith PolicyOperator = "startsWith"
	// PolicyEndsWith compares a string suffix.
	PolicyEndsWith PolicyOperator = "endsWith"
	// PolicyIPInCIDR tests whether an IP is within a CIDR.
	PolicyIPInCIDR PolicyOperator = "ipInCIDR"
	// PolicyRelated resolves a relationship from the evaluated Subject.
	PolicyRelated PolicyOperator = "related"
)

// PolicyExpression is one recursive boolean Policy node. Expressions contains
// child boolean nodes; Values contains value operands. Related uses the current
// evaluated Subject and stores its target in Values.
type PolicyExpression struct {
	Operator     PolicyOperator
	Expressions  []PolicyExpression
	Values       []PolicyValue
	Relationship RelationshipReference
}

// PolicyValueSource identifies how a Policy value is obtained.
type PolicyValueSource string

const (
	// PolicyValueLiteral reads a literal stored in the Policy.
	PolicyValueLiteral PolicyValueSource = "literal"
	// PolicyValueBuiltin reads one trusted built-in evaluation fact.
	PolicyValueBuiltin PolicyValueSource = "builtin"
	// PolicyValueProperty reads one registered service-owned property.
	PolicyValueProperty PolicyValueSource = "property"
)

// PolicyBuiltin identifies a trusted value present in every applicable
// evaluation context.
type PolicyBuiltin string

const (
	// PolicySubjectType reads the evaluated Subject classification.
	PolicySubjectType PolicyBuiltin = "subject.type"
	// PolicySubjectID reads the evaluated globally unique Subject ID.
	PolicySubjectID PolicyBuiltin = "subject.id"
	// PolicyResourceType reads the evaluated ResourceType.
	PolicyResourceType PolicyBuiltin = "resource.type"
	// PolicyResourceID reads the evaluated ResourceID.
	PolicyResourceID PolicyBuiltin = "resource.id"
	// PolicyRequestTime reads the trusted request time.
	PolicyRequestTime PolicyBuiltin = "request.time"
	// PolicyRequestIP reads the trusted request IP.
	PolicyRequestIP PolicyBuiltin = "request.ip"
)

// PolicyAttributeNamespace identifies the trusted fact source for one
// service-owned policy attribute.
type PolicyAttributeNamespace string

const (
	// PolicyAttributeResource identifies a Resource.Properties fact.
	PolicyAttributeResource PolicyAttributeNamespace = "resource"
	// PolicyAttributeRequest identifies an Operation.Context fact.
	PolicyAttributeRequest PolicyAttributeNamespace = "request"
)

// PolicyAttributeReference identifies one service-owned typed policy fact.
type PolicyAttributeReference struct {
	Service   string
	Namespace PolicyAttributeNamespace
	Name      string
}

// RelationshipReference identifies one service-owned relationship predicate.
type RelationshipReference struct {
	Service string
	Name    string
}

// PolicyValue is one literal, built-in fact, or service-owned property value.
// Literal values are limited by the Policy vocabulary established by the
// policy owner; the mutation seam performs complete type checking.
type PolicyValue struct {
	Source   PolicyValueSource
	Builtin  PolicyBuiltin
	Property PolicyAttributeReference
	Literal  any
}

// Any constructs a disjunction. An empty Any expression is false.
func Any(expressions ...PolicyExpression) PolicyExpression {
	return PolicyExpression{Operator: PolicyAny, Expressions: expressions}
}

// All constructs a conjunction. An empty All expression is true.
func All(expressions ...PolicyExpression) PolicyExpression {
	return PolicyExpression{Operator: PolicyAll, Expressions: expressions}
}

// Not constructs the negation of expression.
func Not(expression PolicyExpression) PolicyExpression {
	return PolicyExpression{Operator: PolicyNot, Expressions: []PolicyExpression{expression}}
}

// Exists constructs a property-existence expression.
func Exists(value PolicyValue) PolicyExpression {
	return PolicyExpression{Operator: PolicyExists, Values: []PolicyValue{value}}
}

// Equal constructs an equality expression.
func Equal(left, right PolicyValue) PolicyExpression {
	return compare(PolicyEqual, left, right)
}

// NotEqual constructs an inequality expression.
func NotEqual(left, right PolicyValue) PolicyExpression {
	return compare(PolicyNotEqual, left, right)
}

// LessThan constructs an ordered less-than expression.
func LessThan(left, right PolicyValue) PolicyExpression {
	return compare(PolicyLessThan, left, right)
}

// LessThanOrEqual constructs an ordered less-than-or-equal expression.
func LessThanOrEqual(left, right PolicyValue) PolicyExpression {
	return compare(PolicyLessThanOrEqual, left, right)
}

// GreaterThan constructs an ordered greater-than expression.
func GreaterThan(left, right PolicyValue) PolicyExpression {
	return compare(PolicyGreaterThan, left, right)
}

// GreaterThanOrEqual constructs an ordered greater-than-or-equal expression.
func GreaterThanOrEqual(left, right PolicyValue) PolicyExpression {
	return compare(PolicyGreaterThanOrEqual, left, right)
}

// In constructs a set-membership expression. The first value is tested
// against the remaining homogeneous literal values.
func In(value PolicyValue, set ...PolicyValue) PolicyExpression {
	return compareSet(PolicyIn, value, set)
}

// NotIn constructs a negated set-membership expression.
func NotIn(value PolicyValue, set ...PolicyValue) PolicyExpression {
	return compareSet(PolicyNotIn, value, set)
}

// StartsWith constructs a string-prefix expression.
func StartsWith(value, prefix PolicyValue) PolicyExpression {
	return compare(PolicyStartsWith, value, prefix)
}

// EndsWith constructs a string-suffix expression.
func EndsWith(value, suffix PolicyValue) PolicyExpression {
	return compare(PolicyEndsWith, value, suffix)
}

// IPInCIDR constructs an IP network-membership expression.
func IPInCIDR(ip, cidr PolicyValue) PolicyExpression {
	return compare(PolicyIPInCIDR, ip, cidr)
}

// Related constructs a relationship expression from the current evaluated
// Subject to the resource reference produced by object.
func Related(relationship RelationshipReference, object PolicyValue) PolicyExpression {
	return PolicyExpression{
		Operator:     PolicyRelated,
		Values:       []PolicyValue{object},
		Relationship: relationship,
	}
}

// Literal constructs a typed literal Policy value.
func Literal(value any) PolicyValue {
	return PolicyValue{Source: PolicyValueLiteral, Literal: value}
}

// Builtin constructs a trusted built-in Policy value.
func Builtin(value PolicyBuiltin) PolicyValue {
	return PolicyValue{Source: PolicyValueBuiltin, Builtin: value}
}

// ResourceProperty constructs a service-owned resource property value.
func ResourceProperty(service string, name string) PolicyValue {
	return PolicyValue{
		Source: PolicyValueProperty,
		Property: PolicyAttributeReference{
			Service:   service,
			Namespace: PolicyAttributeResource,
			Name:      name,
		},
	}
}

// RequestProperty constructs a service-owned request property value.
func RequestProperty(service string, name string) PolicyValue {
	return PolicyValue{
		Source: PolicyValueProperty,
		Property: PolicyAttributeReference{
			Service:   service,
			Namespace: PolicyAttributeRequest,
			Name:      name,
		},
	}
}

func compare(operator PolicyOperator, left, right PolicyValue) PolicyExpression {
	return PolicyExpression{Operator: operator, Values: []PolicyValue{left, right}}
}

func compareSet(operator PolicyOperator, value PolicyValue, set []PolicyValue) PolicyExpression {
	values := make([]PolicyValue, 1, len(set)+1)
	values[0] = value
	return PolicyExpression{Operator: operator, Values: append(values, set...)}
}

func validatePolicyExpression(expression PolicyExpression, path string) error {
	if err := validatePolicyExpressionShape(expression); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for index := range expression.Values {
		if err := validatePolicyValue(expression.Values[index]); err != nil {
			return fmt.Errorf("%s value %d: %w", path, index, err)
		}
	}
	if err := validatePolicyOperands(expression); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for index := range expression.Expressions {
		if err := validatePolicyExpression(expression.Expressions[index], fmt.Sprintf("%s expression %d", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validatePolicyExpressionShape(expression PolicyExpression) error {
	hasRelationship := expression.Relationship != (RelationshipReference{})
	switch expression.Operator {
	case PolicyAny, PolicyAll:
		if len(expression.Values) != 0 || hasRelationship {
			return fmt.Errorf("operator %q accepts only child expressions", expression.Operator)
		}
	case PolicyNot:
		if len(expression.Expressions) != 1 || len(expression.Values) != 0 || hasRelationship {
			return fmt.Errorf("operator %q requires exactly one child expression", expression.Operator)
		}
	case PolicyExists:
		if len(expression.Expressions) != 0 || len(expression.Values) != 1 || hasRelationship {
			return fmt.Errorf("operator %q requires exactly one value", expression.Operator)
		}
	case PolicyEqual, PolicyNotEqual, PolicyLessThan, PolicyLessThanOrEqual,
		PolicyGreaterThan, PolicyGreaterThanOrEqual, PolicyStartsWith,
		PolicyEndsWith, PolicyIPInCIDR:
		if len(expression.Expressions) != 0 || len(expression.Values) != 2 || hasRelationship {
			return fmt.Errorf("operator %q requires exactly two values", expression.Operator)
		}
	case PolicyIn, PolicyNotIn:
		if len(expression.Expressions) != 0 || len(expression.Values) == 0 || hasRelationship {
			return fmt.Errorf("operator %q requires a value and zero or more set values", expression.Operator)
		}
	case PolicyRelated:
		if len(expression.Expressions) != 0 || len(expression.Values) != 1 ||
			expression.Relationship.Service == "" || expression.Relationship.Name == "" {
			return fmt.Errorf("operator %q requires one value and a complete relationship reference", expression.Operator)
		}
	default:
		return fmt.Errorf("unsupported operator %q", expression.Operator)
	}
	return nil
}

func validatePolicyValue(value PolicyValue) error {
	switch value.Source {
	case PolicyValueLiteral:
		if value.Builtin != "" || value.Property != (PolicyAttributeReference{}) || value.Literal == nil {
			return fmt.Errorf("literal value requires only a non-nil literal")
		}
		if !validPolicyLiteral(value.Literal) {
			return fmt.Errorf("literal value has unsupported type %T", value.Literal)
		}
	case PolicyValueBuiltin:
		if value.Property != (PolicyAttributeReference{}) || value.Literal != nil || !validPolicyBuiltin(value.Builtin) {
			return fmt.Errorf("builtin value requires one supported builtin")
		}
	case PolicyValueProperty:
		if value.Builtin != "" || value.Literal != nil || value.Property.Service == "" ||
			value.Property.Name == "" || !validPolicyAttributeNamespace(value.Property.Namespace) {
			return fmt.Errorf("property value requires one complete property reference")
		}
	default:
		return fmt.Errorf("unsupported value source %q", value.Source)
	}
	return nil
}

type policyValueKind string

const (
	policyValueBool              policyValueKind = "bool"
	policyValueString            policyValueKind = "string"
	policyValueInt64             policyValueKind = "int64"
	policyValueTimestamp         policyValueKind = "timestamp"
	policyValueIP                policyValueKind = "ip"
	policyValueCIDR              policyValueKind = "cidr"
	policyValueResourceReference policyValueKind = "resource reference"
)

func validatePolicyOperands(expression PolicyExpression) error {
	switch expression.Operator {
	case PolicyExists:
		if expression.Values[0].Source != PolicyValueProperty {
			return fmt.Errorf("operator %q requires a property value", expression.Operator)
		}
	case PolicyEqual, PolicyNotEqual:
		return requireMatchingKnownPolicyKinds(expression)
	case PolicyLessThan, PolicyLessThanOrEqual, PolicyGreaterThan, PolicyGreaterThanOrEqual:
		if err := requireMatchingKnownPolicyKinds(expression); err != nil {
			return err
		}
		for _, value := range expression.Values {
			if kind, known := knownPolicyValueKind(value); known &&
				kind != policyValueString && kind != policyValueInt64 && kind != policyValueTimestamp {
				return fmt.Errorf("operator %q does not order %s values", expression.Operator, kind)
			}
		}
	case PolicyIn, PolicyNotIn:
		return validatePolicySet(expression)
	case PolicyStartsWith, PolicyEndsWith:
		return requireKnownPolicyKinds(expression, policyValueString, policyValueString)
	case PolicyIPInCIDR:
		return requireKnownPolicyKinds(expression, policyValueIP, policyValueCIDR)
	case PolicyRelated:
		if kind, known := knownPolicyValueKind(expression.Values[0]); known && kind != policyValueResourceReference {
			return fmt.Errorf("operator %q requires a resource reference object", expression.Operator)
		}
	}
	return nil
}

func validatePolicySet(expression PolicyExpression) error {
	var setKind policyValueKind
	for index, value := range expression.Values[1:] {
		if value.Source != PolicyValueLiteral {
			return fmt.Errorf("operator %q set value %d must be a literal", expression.Operator, index)
		}
		kind, _ := knownPolicyValueKind(value)
		if index == 0 {
			setKind = kind
		} else if kind != setKind {
			return fmt.Errorf("operator %q set values must have one type", expression.Operator)
		}
	}
	if len(expression.Values) == 1 {
		return nil
	}
	if valueKind, known := knownPolicyValueKind(expression.Values[0]); known && valueKind != setKind {
		return fmt.Errorf("operator %q value and set must have one type", expression.Operator)
	}
	return nil
}

func requireMatchingKnownPolicyKinds(expression PolicyExpression) error {
	left, leftKnown := knownPolicyValueKind(expression.Values[0])
	right, rightKnown := knownPolicyValueKind(expression.Values[1])
	if leftKnown && rightKnown && left != right {
		return fmt.Errorf("operator %q values must have one type", expression.Operator)
	}
	return nil
}

func requireKnownPolicyKinds(expression PolicyExpression, expected ...policyValueKind) error {
	for index, value := range expression.Values {
		if kind, known := knownPolicyValueKind(value); known && kind != expected[index] {
			return fmt.Errorf("operator %q value %d requires %s, got %s", expression.Operator, index, expected[index], kind)
		}
	}
	return nil
}

func knownPolicyValueKind(value PolicyValue) (policyValueKind, bool) {
	switch value.Source {
	case PolicyValueLiteral:
		kind, ok := policyLiteralKind(value.Literal)
		return kind, ok
	case PolicyValueBuiltin:
		switch value.Builtin {
		case PolicySubjectType, PolicySubjectID, PolicyResourceType, PolicyResourceID:
			return policyValueString, true
		case PolicyRequestTime:
			return policyValueTimestamp, true
		case PolicyRequestIP:
			return policyValueIP, true
		}
	}
	return "", false
}

func policyLiteralKind(value any) (policyValueKind, bool) {
	switch value := value.(type) {
	case bool:
		return policyValueBool, true
	case string:
		return policyValueString, true
	case int64:
		return policyValueInt64, true
	case time.Time:
		return policyValueTimestamp, true
	case netip.Addr:
		return policyValueIP, true
	case netip.Prefix:
		return policyValueCIDR, true
	case ResourceReference:
		return policyValueResourceReference, value.Type != "" && value.ID != ""
	default:
		return "", false
	}
}

func validPolicyLiteral(value any) bool {
	_, valid := policyLiteralKind(value)
	return valid
}

func validPolicyBuiltin(value PolicyBuiltin) bool {
	switch value {
	case PolicySubjectType, PolicySubjectID, PolicyResourceType, PolicyResourceID,
		PolicyRequestTime, PolicyRequestIP:
		return true
	default:
		return false
	}
}

func validPolicyAttributeNamespace(value PolicyAttributeNamespace) bool {
	switch value {
	case PolicyAttributeResource, PolicyAttributeRequest:
		return true
	default:
		return false
	}
}
