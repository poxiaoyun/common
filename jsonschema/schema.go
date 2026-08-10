package jsonschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	TypeObject  = "object"
	TypeArray   = "array"
	TypeString  = "string"
	TypeNumber  = "number"
	TypeInteger = "integer"
	TypeBoolean = "boolean"
	TypeNull    = "null"
)

var knownSchemaFields = func() map[string]bool {
	result := map[string]bool{}
	typeOfSchema := reflect.TypeFor[Schema]()
	for i := 0; i < typeOfSchema.NumField(); i++ {
		tag := typeOfSchema.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			result[name] = true
		}
	}
	return result
}()

// Schema represents the JSON Schema 2020-12 keywords used by common schema
// processing. Unknown keywords are retained in ExtraProps.
type Schema struct {
	ID            string            `json:"$id,omitempty"`
	Schema        string            `json:"$schema,omitempty"`
	Ref           string            `json:"$ref,omitempty"`
	DynamicRef    string            `json:"$dynamicRef,omitempty"`
	Comment       string            `json:"$comment,omitempty"`
	Defs          map[string]Schema `json:"$defs,omitempty"`
	Anchor        string            `json:"$anchor,omitempty"`
	DynamicAnchor string            `json:"$dynamicAnchor,omitempty"`
	Vocabulary    map[string]bool   `json:"$vocabulary,omitempty"`

	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	WriteOnly   bool   `json:"writeOnly,omitempty"`
	Examples    []any  `json:"examples,omitempty"`

	AllOf []Schema `json:"allOf,omitempty"`
	AnyOf []Schema `json:"anyOf,omitempty"`
	OneOf []Schema `json:"oneOf,omitempty"`
	Not   *Schema  `json:"not,omitempty"`

	If               *Schema           `json:"if,omitempty"`
	Then             *Schema           `json:"then,omitempty"`
	Else             *Schema           `json:"else,omitempty"`
	DependentSchemas map[string]Schema `json:"dependentSchemas,omitempty"`

	Type    Types `json:"type,omitempty"`
	Const   any   `json:"const,omitempty"`
	Enum    []any `json:"enum,omitempty"`
	Default any   `json:"default,omitempty"`

	Pattern   string `json:"pattern,omitempty"`
	Format    string `json:"format,omitempty"`
	MaxLength *int64 `json:"maxLength,omitempty"`
	MinLength *int64 `json:"minLength,omitempty"`

	MultipleOf       *float64 `json:"multipleOf,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`

	Properties           Properties          `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	MaxProperties        *int64              `json:"maxProperties,omitempty"`
	MinProperties        *int64              `json:"minProperties,omitempty"`
	AdditionalProperties *SchemaOrBool       `json:"additionalProperties,omitempty"`
	PatternProperties    Properties          `json:"patternProperties,omitempty"`
	PropertyNames        *Schema             `json:"propertyNames,omitempty"`
	DependentRequired    map[string][]string `json:"dependentRequired,omitempty"`

	PrefixItems []Schema `json:"prefixItems,omitempty"`
	Items       *Schema  `json:"items,omitempty"`
	MaxItems    *int64   `json:"maxItems,omitempty"`
	MinItems    *int64   `json:"minItems,omitempty"`
	Contains    *Schema  `json:"contains,omitempty"`
	MinContains *int64   `json:"minContains,omitempty"`
	MaxContains *int64   `json:"maxContains,omitempty"`
	UniqueItems bool     `json:"uniqueItems,omitempty"`

	ContentEncoding  string  `json:"contentEncoding,omitempty"`
	ContentMediaType string  `json:"contentMediaType,omitempty"`
	ContentSchema    *Schema `json:"contentSchema,omitempty"`

	UnevaluatedItems      *SchemaOrBool `json:"unevaluatedItems,omitempty"`
	UnevaluatedProperties *SchemaOrBool `json:"unevaluatedProperties,omitempty"`

	ExtraProps map[string]any `json:"-"`
}

func (s Schema) GetExtension(name string) any {
	return s.ExtraProps[name]
}

func (s Schema) MarshalJSON() ([]byte, error) {
	type schemaAlias Schema
	fields, err := json.Marshal(schemaAlias(s))
	if err != nil {
		return nil, err
	}
	extra, err := json.Marshal(s.ExtraProps)
	if err != nil {
		return nil, err
	}
	return joinObjects(fields, extra), nil
}

func (s *Schema) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("true")) {
		*s = Schema{}
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("false")) {
		*s = Schema{Not: &Schema{}}
		return nil
	}
	type schemaAlias Schema
	decoded := schemaAlias{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, raw := range fields {
		if knownSchemaFields[name] {
			continue
		}
		if decoded.ExtraProps == nil {
			decoded.ExtraProps = map[string]any{}
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		decoded.ExtraProps[name] = value
	}
	*s = Schema(decoded)
	return nil
}

type Types []string

// SchemaOrBool represents JSON Schema keywords whose value can be either a
// schema or a boolean schema.
type SchemaOrBool struct {
	Allows bool
	Schema *Schema
}

func (s SchemaOrBool) MarshalJSON() ([]byte, error) {
	if s.Schema != nil {
		return json.Marshal(s.Schema)
	}
	return json.Marshal(s.Allows)
}

func (s *SchemaOrBool) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("true")) {
		*s = SchemaOrBool{Allows: true}
		return nil
	}
	if bytes.Equal(trimmed, []byte("false")) {
		*s = SchemaOrBool{}
		return nil
	}
	schema := Schema{}
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		return err
	}
	*s = SchemaOrBool{Schema: &schema}
	return nil
}

func (t Types) MarshalJSON() ([]byte, error) {
	if len(t) == 1 {
		return json.Marshal(t[0])
	}
	return json.Marshal([]string(t))
}

func (t *Types) UnmarshalJSON(data []byte) error {
	values := []string{}
	if err := json.Unmarshal(data, &values); err == nil {
		*t = values
		return nil
	}
	value := ""
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*t = Types{value}
	return nil
}

// Properties preserves schema declaration order.
type Properties []Property

type Property struct {
	Name   string
	Schema Schema
}

func (p Properties) MarshalJSON() ([]byte, error) {
	buffer := &bytes.Buffer{}
	buffer.WriteByte('{')
	for i, property := range p {
		if i != 0 {
			buffer.WriteByte(',')
		}
		name, err := json.Marshal(property.Name)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(property.Schema)
		if err != nil {
			return nil, err
		}
		buffer.Write(name)
		buffer.WriteByte(':')
		buffer.Write(value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func (p *Properties) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("schema properties must be an object")
	}
	result := Properties{}
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return err
		}
		property := Schema{}
		if err := decoder.Decode(&property); err != nil {
			return err
		}
		result = append(result, Property{Name: name.(string), Schema: property})
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	*p = result
	return nil
}

// Validate compiles schema as draft 2020-12 and validates value.
func Validate(schema Schema, value any) error {
	data, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	document, err := validator.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	compiler := validator.NewCompiler()
	compiler.DefaultDraft(validator.Draft2020)
	const location = "urn:common:jsonschema"
	if err := compiler.AddResource(location, document); err != nil {
		return err
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return err
	}
	return compiled.Validate(value)
}

func joinObjects(left, right []byte) []byte {
	if len(right) == 0 || bytes.Equal(right, []byte("{}")) || bytes.Equal(right, []byte("null")) {
		return left
	}
	result := make([]byte, 0, len(left)+len(right)-1)
	result = append(result, left[:len(left)-1]...)
	result = append(result, ',')
	result = append(result, right[1:]...)
	return result
}
