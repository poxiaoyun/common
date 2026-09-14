package authz

import (
	"cmp"
	"net/netip"
	"strings"
	"time"
)

// ComparePolicyValues evaluates a validated scalar comparison without value
// coercion. The second result reports whether the comparison is known; nil,
// unsupported types, and incompatible operand types produce Unknown. Callers
// establish operator arity through Policy or ResourcePropertyConstraint.Validate.
// Existence, boolean composition, and relationships belong to their evaluators.
func ComparePolicyValues(operator PolicyOperator, values ...any) (bool, bool) {
	switch operator {
	case PolicyEqual, PolicyNotEqual:
		equal, known := equalPolicyValues(values[0], values[1])
		return (operator == PolicyEqual) == equal, known
	case PolicyLessThan, PolicyLessThanOrEqual, PolicyGreaterThan, PolicyGreaterThanOrEqual:
		comparison, known := comparePolicyValues(values[0], values[1])
		switch operator {
		case PolicyLessThan:
			return comparison < 0, known
		case PolicyLessThanOrEqual:
			return comparison <= 0, known
		case PolicyGreaterThan:
			return comparison > 0, known
		default:
			return comparison >= 0, known
		}
	case PolicyIn, PolicyNotIn:
		if !validPolicyLiteral(values[0]) {
			return false, false
		}
		unknown := false
		for _, candidate := range values[1:] {
			equal, known := equalPolicyValues(values[0], candidate)
			if known && equal {
				return operator == PolicyIn, true
			}
			unknown = unknown || !known
		}
		return operator == PolicyNotIn, !unknown
	case PolicyStartsWith, PolicyEndsWith:
		left, leftOK := values[0].(string)
		right, rightOK := values[1].(string)
		if !leftOK || !rightOK {
			return false, false
		}
		if operator == PolicyStartsWith {
			return strings.HasPrefix(left, right), true
		}
		return strings.HasSuffix(left, right), true
	case PolicyIPInCIDR:
		ip, ipOK := values[0].(netip.Addr)
		prefix, prefixOK := values[1].(netip.Prefix)
		if !ipOK || !prefixOK {
			return false, false
		}
		return prefix.Contains(ip), true
	default:
		panic("not a scalar Policy comparison")
	}
}

func equalPolicyValues(left, right any) (bool, bool) {
	switch left := left.(type) {
	case bool:
		right, known := right.(bool)
		return left == right, known
	case string:
		right, known := right.(string)
		return left == right, known
	case int64:
		right, known := right.(int64)
		return left == right, known
	case time.Time:
		right, known := right.(time.Time)
		return left.Equal(right), known
	case netip.Addr:
		right, known := right.(netip.Addr)
		return left == right, known
	case netip.Prefix:
		right, known := right.(netip.Prefix)
		return left == right, known
	case ResourceReference:
		right, known := right.(ResourceReference)
		return left == right, known
	default:
		return false, false
	}
}

func comparePolicyValues(left, right any) (int, bool) {
	switch left := left.(type) {
	case string:
		right, known := right.(string)
		return strings.Compare(left, right), known
	case int64:
		right, known := right.(int64)
		return cmp.Compare(left, right), known
	case time.Time:
		right, known := right.(time.Time)
		return left.Compare(right), known
	default:
		return 0, false
	}
}
