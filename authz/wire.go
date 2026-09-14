package authz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"time"
)

// MarshalJSON preserves the exact Policy fact type. Each non-null property is
// a tagged {type,value} literal; an explicit null remains distinct from absence.
func (properties Properties) MarshalJSON() ([]byte, error) {
	if properties == nil {
		return []byte("null"), nil
	}
	encoded := make(map[string]json.RawMessage, len(properties))
	for name, value := range properties {
		literal, err := marshalPolicyLiteral(value)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		encoded[name] = literal
	}
	return json.Marshal(encoded)
}

// UnmarshalJSON decodes tagged facts without numeric or string coercion.
func (properties *Properties) UnmarshalJSON(data []byte) error {
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	if encoded == nil {
		*properties = nil
		return nil
	}
	decoded := make(Properties, len(encoded))
	for name, raw := range encoded {
		value, err := unmarshalPolicyLiteral(raw)
		if err != nil {
			return fmt.Errorf("property %q: %w", name, err)
		}
		decoded[name] = value
	}
	*properties = decoded
	return nil
}

type encodedPolicyValue struct {
	Source   PolicyValueSource `json:"source"`
	Builtin  json.RawMessage   `json:"builtin,omitempty"`
	Property json.RawMessage   `json:"property,omitempty"`
	Literal  json.RawMessage   `json:"literal,omitempty"`
}

// MarshalJSON uses the same exact tagged literal representation as Properties.
// Literal source values must satisfy the closed Policy value vocabulary.
func (value PolicyValue) MarshalJSON() ([]byte, error) {
	if err := validatePolicyValue(value); err != nil {
		return nil, err
	}
	encoded := encodedPolicyValue{Source: value.Source}
	if value.Source == PolicyValueBuiltin {
		builtin, err := json.Marshal(value.Builtin)
		if err != nil {
			return nil, err
		}
		encoded.Builtin = builtin
	}
	if value.Source == PolicyValueProperty {
		property, err := json.Marshal(value.Property)
		if err != nil {
			return nil, err
		}
		encoded.Property = property
	}
	if value.Source == PolicyValueLiteral {
		literal, err := marshalPolicyLiteral(value.Literal)
		if err != nil {
			return nil, err
		}
		encoded.Literal = literal
	}
	return json.Marshal(encoded)
}

// UnmarshalJSON validates the closed value shape and restores the literal's
// exact type. Null is a resource fact, not a permitted Policy literal.
func (value *PolicyValue) UnmarshalJSON(data []byte) error {
	var encoded encodedPolicyValue
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return err
	}
	decoded := PolicyValue{Source: encoded.Source}
	if len(encoded.Builtin) != 0 {
		if encoded.Source != PolicyValueBuiltin {
			return fmt.Errorf("only builtin source accepts a builtin field")
		}
		if err := json.Unmarshal(encoded.Builtin, &decoded.Builtin); err != nil {
			return err
		}
	}
	if len(encoded.Property) != 0 {
		if encoded.Source != PolicyValueProperty {
			return fmt.Errorf("only property source accepts a property field")
		}
		decoder := json.NewDecoder(bytes.NewReader(encoded.Property))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&decoded.Property); err != nil {
			return err
		}
	}
	if len(encoded.Literal) != 0 {
		if encoded.Source != PolicyValueLiteral {
			return fmt.Errorf("only literal source accepts a literal field")
		}
		literal, err := unmarshalPolicyLiteral(encoded.Literal)
		if err != nil {
			return err
		}
		decoded.Literal = literal
	}
	if err := validatePolicyValue(decoded); err != nil {
		return err
	}
	*value = decoded
	return nil
}

// UnmarshalJSON rejects unknown constraint fields at the external boundary.
// The consuming wire adapter still validates the complete operator shape.
func (constraint *ResourceConstraint) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("resource constraint must be an object")
	}
	type encodedConstraint ResourceConstraint
	var decoded encodedConstraint
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*constraint = ResourceConstraint(decoded)
	return nil
}

// UnmarshalJSON requires the complete constraint instead of interpreting a
// missing or null plan payload as the zero-value selection.
func (plan *ResourceConstraintPlan) UnmarshalJSON(data []byte) error {
	var encoded struct {
		Constraint *ResourceConstraint `json:"constraint"`
		Snapshot   string              `json:"snapshot,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return err
	}
	if encoded.Constraint == nil {
		return fmt.Errorf("resource constraint plan requires a constraint")
	}
	*plan = ResourceConstraintPlan{Constraint: *encoded.Constraint, Snapshot: encoded.Snapshot}
	return nil
}

// UnmarshalJSON requires an explicit boolean result because omitting it must
// not silently change which known truth set the predicate selects.
func (predicate *ResourcePropertyConstraint) UnmarshalJSON(data []byte) error {
	var encoded struct {
		Expression PolicyExpression `json:"expression"`
		Result     *bool            `json:"result"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return err
	}
	if encoded.Result == nil {
		return fmt.Errorf("property constraint requires a boolean result")
	}
	*predicate = ResourcePropertyConstraint{Expression: encoded.Expression, Result: *encoded.Result}
	return nil
}

// UnmarshalJSON requires an explicit relationship truth set and rejects
// unknown fields rather than silently broadening candidate selection.
func (relationship *ResourceRelationshipConstraint) UnmarshalJSON(data []byte) error {
	var encoded struct {
		Relationship   RelationshipReference    `json:"relationship"`
		ObjectProperty PolicyAttributeReference `json:"objectProperty"`
		Result         *bool                    `json:"result"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return err
	}
	if encoded.Result == nil {
		return fmt.Errorf("relationship constraint requires a boolean result")
	}
	*relationship = ResourceRelationshipConstraint{
		Relationship:   encoded.Relationship,
		ObjectProperty: encoded.ObjectProperty,
		Result:         *encoded.Result,
	}
	return nil
}

type encodedPolicyLiteral struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// UnmarshalJSON rejects fields outside the versioned Policy wire vocabulary.
// Policy installation still owns complete expression and attribute validation.
func (policy *Policy) UnmarshalJSON(data []byte) error {
	type encodedPolicy Policy
	var decoded encodedPolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*policy = Policy(decoded)
	return nil
}

func marshalPolicyLiteral(value any) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	kind, valid := policyLiteralKind(value)
	if !valid {
		return nil, fmt.Errorf("unsupported policy fact type %T", value)
	}
	typeName := string(kind)
	switch typed := value.(type) {
	case int64:
		// A decimal string avoids precision loss in JSON consumers using IEEE754.
		value = strconv.FormatInt(typed, 10)
	case netip.Addr:
		if !typed.IsValid() {
			return nil, fmt.Errorf("invalid policy IP fact")
		}
	case netip.Prefix:
		if !typed.IsValid() {
			return nil, fmt.Errorf("invalid policy CIDR fact")
		}
	case ResourceReference:
		typeName = "resourceReference"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(encodedPolicyLiteral{Type: typeName, Value: raw})
}

func unmarshalPolicyLiteral(data []byte) (any, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	var encoded encodedPolicyLiteral
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return nil, err
	}
	if len(encoded.Value) == 0 || bytes.Equal(bytes.TrimSpace(encoded.Value), []byte("null")) {
		return nil, fmt.Errorf("policy fact %q requires a non-null value", encoded.Type)
	}
	switch encoded.Type {
	case "bool":
		var value bool
		err := json.Unmarshal(encoded.Value, &value)
		return value, err
	case "string":
		var value string
		err := json.Unmarshal(encoded.Value, &value)
		return value, err
	case "int64":
		var decimal string
		if err := json.Unmarshal(encoded.Value, &decimal); err != nil {
			return nil, err
		}
		return strconv.ParseInt(decimal, 10, 64)
	case "timestamp":
		var value time.Time
		err := json.Unmarshal(encoded.Value, &value)
		return value, err
	case "ip":
		var text string
		if err := json.Unmarshal(encoded.Value, &text); err != nil {
			return nil, err
		}
		return netip.ParseAddr(text)
	case "cidr":
		var text string
		if err := json.Unmarshal(encoded.Value, &text); err != nil {
			return nil, err
		}
		return netip.ParsePrefix(text)
	case "resourceReference":
		var value ResourceReference
		decoder := json.NewDecoder(bytes.NewReader(encoded.Value))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if value.Type == "" || value.ID == "" {
			return nil, fmt.Errorf("policy resource reference requires type and ID")
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported policy fact type %q", encoded.Type)
	}
}
