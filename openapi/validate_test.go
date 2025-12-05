package openapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-openapi/spec"
)

func TestValidator_ValidateJson(t *testing.T) {
	v := NewDefaultValidator()

	tests := []struct {
		name        string
		schema      Schema
		data        any
		expectValid bool
	}{
		// --- Type Validation ---
		{
			name:        "type string valid",
			schema:      Schema{Type: spec.StringOrArray{"string"}},
			data:        "test",
			expectValid: true,
		},
		{
			name:        "type string invalid",
			schema:      Schema{Type: spec.StringOrArray{"string"}},
			data:        123,
			expectValid: false,
		},
		{
			name:        "type number valid",
			schema:      Schema{Type: spec.StringOrArray{"number"}},
			data:        123.45,
			expectValid: true,
		},
		{
			name:        "type integer valid float",
			schema:      Schema{Type: spec.StringOrArray{"integer"}},
			data:        123.0,
			expectValid: true,
		},
		{
			name:        "type integer invalid float",
			schema:      Schema{Type: spec.StringOrArray{"integer"}},
			data:        123.45,
			expectValid: false,
		},
		{
			name:        "type integer valid json.Number",
			schema:      Schema{Type: spec.StringOrArray{"integer"}},
			data:        json.Number("123"),
			expectValid: true,
		},
		{
			name:        "type boolean valid",
			schema:      Schema{Type: spec.StringOrArray{"boolean"}},
			data:        true,
			expectValid: true,
		},
		{
			name:        "type null valid",
			schema:      Schema{Type: spec.StringOrArray{"null"}},
			data:        nil,
			expectValid: true,
		},
		{
			name:        "type array valid",
			schema:      Schema{Type: spec.StringOrArray{"array"}},
			data:        []any{1, 2},
			expectValid: true,
		},
		{
			name:        "type object valid",
			schema:      Schema{Type: spec.StringOrArray{"object"}},
			data:        map[string]any{"a": 1},
			expectValid: true,
		},

		// --- String Validation ---
		{
			name:        "string maxLength valid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, MaxLength: ptrInt64(5)},
			data:        "hello",
			expectValid: true,
		},
		{
			name:        "string maxLength invalid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, MaxLength: ptrInt64(5)},
			data:        "hello world",
			expectValid: false,
		},
		{
			name:        "string minLength valid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, MinLength: ptrInt64(2)},
			data:        "hi",
			expectValid: true,
		},
		{
			name:        "string minLength invalid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, MinLength: ptrInt64(2)},
			data:        "a",
			expectValid: false,
		},
		{
			name:        "string pattern valid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, Pattern: "^[a-z]+$"},
			data:        "abc",
			expectValid: true,
		},
		{
			name:        "string pattern invalid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, Pattern: "^[a-z]+$"},
			data:        "123",
			expectValid: false,
		},

		// --- Number Validation ---
		{
			name:        "number maximum valid",
			schema:      Schema{Type: spec.StringOrArray{"number"}, Maximum: ptrFloat64(10)},
			data:        10.0,
			expectValid: true,
		},
		{
			name:        "number maximum invalid",
			schema:      Schema{Type: spec.StringOrArray{"number"}, Maximum: ptrFloat64(10)},
			data:        10.1,
			expectValid: false,
		},
		{
			name:        "number exclusiveMaximum valid",
			schema:      Schema{Type: spec.StringOrArray{"number"}, ExclusiveMaximum: ptrFloat64(10)},
			data:        9.9,
			expectValid: true,
		},
		{
			name:        "number exclusiveMaximum invalid",
			schema:      Schema{Type: spec.StringOrArray{"number"}, ExclusiveMaximum: ptrFloat64(10)},
			data:        10.0,
			expectValid: false,
		},
		{
			name:        "number multipleOf valid",
			schema:      Schema{Type: spec.StringOrArray{"number"}, MultipleOf: ptrFloat64(0.5)},
			data:        1.5,
			expectValid: true,
		},
		{
			name:        "number multipleOf invalid",
			schema:      Schema{Type: spec.StringOrArray{"number"}, MultipleOf: ptrFloat64(0.5)},
			data:        1.6,
			expectValid: false,
		},

		// --- Array Validation ---
		{
			name:        "array minItems valid",
			schema:      Schema{Type: spec.StringOrArray{"array"}, MinItems: ptrInt64(1)},
			data:        []any{1},
			expectValid: true,
		},
		{
			name:        "array minItems invalid",
			schema:      Schema{Type: spec.StringOrArray{"array"}, MinItems: ptrInt64(1)},
			data:        []any{},
			expectValid: false,
		},
		{
			name:        "array uniqueItems valid",
			schema:      Schema{Type: spec.StringOrArray{"array"}, UniqueItems: true},
			data:        []any{1, 2, 3},
			expectValid: true,
		},
		{
			name:        "array uniqueItems invalid",
			schema:      Schema{Type: spec.StringOrArray{"array"}, UniqueItems: true},
			data:        []any{1, 2, 1},
			expectValid: false,
		},
		{
			name: "array items valid",
			schema: Schema{
				Type:  spec.StringOrArray{"array"},
				Items: &Schema{Type: spec.StringOrArray{"string"}},
			},
			data:        []any{"a", "b"},
			expectValid: true,
		},
		{
			name: "array items invalid",
			schema: Schema{
				Type:  spec.StringOrArray{"array"},
				Items: &Schema{Type: spec.StringOrArray{"string"}},
			},
			data:        []any{"a", 1},
			expectValid: false,
		},

		// --- Object Validation ---
		{
			name: "object required valid",
			schema: Schema{
				Type:     spec.StringOrArray{"object"},
				Required: []string{"id"},
			},
			data:        map[string]any{"id": 1},
			expectValid: true,
		},
		{
			name: "object required invalid",
			schema: Schema{
				Type:     spec.StringOrArray{"object"},
				Required: []string{"id"},
			},
			data:        map[string]any{"name": "test"},
			expectValid: false,
		},
		{
			name: "object properties valid",
			schema: Schema{
				Type: spec.StringOrArray{"object"},
				Properties: SchemaProperties{
					{Name: "name", Schema: Schema{Type: spec.StringOrArray{"string"}}},
				},
			},
			data:        map[string]any{"name": "test"},
			expectValid: true,
		},
		{
			name: "object properties invalid",
			schema: Schema{
				Type: spec.StringOrArray{"object"},
				Properties: SchemaProperties{
					{Name: "name", Schema: Schema{Type: spec.StringOrArray{"string"}}},
				},
			},
			data:        map[string]any{"name": 123},
			expectValid: false,
		},

		// --- Format Validation ---
		{
			name:        "format date-time valid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, Format: "date-time"},
			data:        "2023-10-01T12:00:00Z",
			expectValid: true,
		},
		{
			name:        "format date-time invalid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, Format: "date-time"},
			data:        "2023/10/01",
			expectValid: false,
		},
		{
			name:        "format ipv4 valid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, Format: "ipv4"},
			data:        "192.168.1.1",
			expectValid: true,
		},
		{
			name:        "format ipv4 invalid",
			schema:      Schema{Type: spec.StringOrArray{"string"}, Format: "ipv4"},
			data:        "256.256.256.256",
			expectValid: false,
		},

		// --- Combinators ---
		{
			name: "allOf valid",
			schema: Schema{
				AllOf: []Schema{
					{Type: spec.StringOrArray{"string"}},
					{MinLength: ptrInt64(3)},
				},
			},
			data:        "abc",
			expectValid: true,
		},
		{
			name: "allOf invalid",
			schema: Schema{
				AllOf: []Schema{
					{Type: spec.StringOrArray{"string"}},
					{MinLength: ptrInt64(3)},
				},
			},
			data:        "ab",
			expectValid: false,
		},
		{
			name: "anyOf valid",
			schema: Schema{
				AnyOf: []Schema{
					{Type: spec.StringOrArray{"string"}},
					{Type: spec.StringOrArray{"number"}},
				},
			},
			data:        123,
			expectValid: true,
		},
		{
			name: "anyOf invalid",
			schema: Schema{
				AnyOf: []Schema{
					{Type: spec.StringOrArray{"string"}},
					{Type: spec.StringOrArray{"number"}},
				},
			},
			data:        true,
			expectValid: false,
		},
		{
			name: "oneOf valid",
			schema: Schema{
				OneOf: []Schema{
					{Type: spec.StringOrArray{"number"}, MultipleOf: ptrFloat64(5)},
					{Type: spec.StringOrArray{"number"}, MultipleOf: ptrFloat64(3)},
				},
			},
			data:        10, // multiple of 5, not 3
			expectValid: true,
		},
		{
			name: "oneOf invalid (both match)",
			schema: Schema{
				OneOf: []Schema{
					{Type: spec.StringOrArray{"number"}, MultipleOf: ptrFloat64(5)},
					{Type: spec.StringOrArray{"number"}, MultipleOf: ptrFloat64(3)},
				},
			},
			data:        15, // multiple of both
			expectValid: false,
		},
		{
			name: "not valid",
			schema: Schema{
				Not: &Schema{Type: spec.StringOrArray{"string"}},
			},
			data:        123,
			expectValid: true,
		},
		{
			name: "not invalid",
			schema: Schema{
				Not: &Schema{Type: spec.StringOrArray{"string"}},
			},
			data:        "string",
			expectValid: false,
		},
		// --- If/Then/Else Validation ---
		{
			name: "if true then true",
			schema: Schema{
				If:   &Schema{Minimum: ptrFloat64(0)},
				Then: &Schema{Maximum: ptrFloat64(10)},
				Else: &Schema{Minimum: ptrFloat64(-10)},
			},
			data:        5.0,
			expectValid: true,
		},
		{
			name: "if true then false",
			schema: Schema{
				If:   &Schema{Minimum: ptrFloat64(0)},
				Then: &Schema{Maximum: ptrFloat64(10)},
				Else: &Schema{Minimum: ptrFloat64(-10)},
			},
			data:        15.0,
			expectValid: false,
		},
		{
			name: "if false else true",
			schema: Schema{
				If:   &Schema{Minimum: ptrFloat64(0)},
				Then: &Schema{Maximum: ptrFloat64(10)},
				Else: &Schema{Minimum: ptrFloat64(-10)},
			},
			data:        -5.0,
			expectValid: true,
		},
		{
			name: "if false else false",
			schema: Schema{
				If:   &Schema{Minimum: ptrFloat64(0)},
				Then: &Schema{Maximum: ptrFloat64(10)},
				Else: &Schema{Minimum: ptrFloat64(-10)},
			},
			data:        -15.0,
			expectValid: false,
		},
		{
			name: "if false no else",
			schema: Schema{
				If:   &Schema{Minimum: ptrFloat64(0)},
				Then: &Schema{Maximum: ptrFloat64(10)},
			},
			data:        -5.0,
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := v.ValidateJson(tt.schema, tt.data)
			if output.Valid != tt.expectValid {
				t.Errorf("expected valid=%v, got %v. Message: %s, Errors: %v", tt.expectValid, output.Valid, output.Message, output.Errors)
			}
		})
	}
}

func ptrInt64(i int64) *int64       { return &i }
func ptrFloat64(f float64) *float64 { return &f }

type CustomValidator struct{}

func (c *CustomValidator) Validate(ctx context.Context, schema Schema, data any) OutPutError {
	if val, ok := data.(string); ok && val == "custom" {
		return OutPutError{Valid: true}
	}
	return OutPutError{
		Valid:   false,
		Message: "validation failed for custom extension",
	}
}

func TestValidator_Extension(t *testing.T) {
	v := NewDefaultValidator()
	v.Extensions["x-custom"] = &CustomValidator{}

	schema := Schema{
		Type: spec.StringOrArray{"string"},
		ExtraProps: map[string]any{
			"x-custom": true,
		},
	}

	// Valid case
	output := v.ValidateJson(schema, "custom")
	if !output.Valid {
		t.Errorf("expected valid, got invalid: %s", output.Message)
	}

	// Invalid case
	output = v.ValidateJson(schema, "other")
	if output.Valid {
		t.Errorf("expected invalid, got valid")
	}
}
