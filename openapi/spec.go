package openapi

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
	"xiaoshiai.cn/common/collections"
)

const (
	SchemaTypeObject  = "object"
	SchemaTypeArray   = "array"
	SchemaTypeString  = "string"
	SchemaTypeNumber  = "number"
	SchemaTypeInteger = "integer"
	SchemaTypeBoolean = "boolean"
	SchemaTypeFile    = "file"
	SchemaTypeNull    = "null"
)

const XOrder = "x-order"

var knownSchemaFields map[string]bool

func init() {
	knownSchemaFields = make(map[string]bool)
	t := reflect.TypeOf(Schema{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		knownSchemaFields[name] = true
	}
}

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

	Type    StringOrArray `json:"type,omitempty"`
	Const   any           `json:"const,omitempty"`
	Enum    []any         `json:"enum,omitempty"`
	Default any           `json:"default,omitempty"`

	// This string SHOULD be a valid regular expression, according to the ECMA-262 regular expression dialect.
	Pattern   string `json:"pattern,omitempty"`
	Format    string `json:"format,omitempty"`
	MaxLength *int64 `json:"maxLength,omitempty"`
	MinLength *int64 `json:"minLength,omitempty"`

	MultipleOf       *float64 `json:"multipleOf,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`

	Properties           SchemaProperties               `json:"properties,omitempty"`
	Required             []string                       `json:"required,omitempty"`
	MaxProperties        *int64                         `json:"maxProperties,omitempty"`
	MinProperties        *int64                         `json:"minProperties,omitempty"`
	AdditionalProperties *SchemaOrBool                  `json:"additionalProperties,omitempty"`
	PatternProperties    SchemaProperties               `json:"patternProperties,omitempty"`
	PropertyNames        *Schema                        `json:"propertyNames,omitempty"`
	DependentRequired    map[string][]string            `json:"dependentRequired,omitempty"`
	Dependencies         map[string]SchemaOrStringArray `json:"dependencies,omitempty"`

	PrefixItems     []Schema      `json:"prefixItems,omitempty"`
	Items           *Schema       `json:"items,omitempty"`
	AdditionalItems *SchemaOrBool `json:"additionalItems,omitempty"`
	MaxItems        *int64        `json:"maxItems,omitempty"`
	MinItems        *int64        `json:"minItems,omitempty"`
	Contains        *Schema       `json:"contains,omitempty"`
	MinContains     *int64        `json:"minContains,omitempty"`
	MaxContains     *int64        `json:"maxContains,omitempty"`
	UniqueItems     bool          `json:"uniqueItems,omitempty"`

	ContentEncoding  string  `json:"contentEncoding,omitempty"`
	ContentMediaType string  `json:"contentMediaType,omitempty"`
	ContentSchema    *Schema `json:"contentSchema,omitempty"`

	UnevaluatedItems      *SchemaOrBool `json:"unevaluatedItems,omitempty"`
	UnevaluatedProperties *SchemaOrBool `json:"unevaluatedProperties,omitempty"`

	// fileds below are not specified in JSON Schema but widely used.
	Example       any                         `json:"example,omitempty"`
	Definitions   map[string]Schema           `json:"definitions,omitempty"`
	Nullable      bool                        `json:"nullable,omitempty"`
	Discriminator string                      `json:"discriminator,omitempty"`
	XML           *spec.XMLObject             `json:"xml,omitempty"`
	ExternalDocs  *spec.ExternalDocumentation `json:"externalDocs,omitempty"`

	ExtraProps map[string]any `json:"-"`
}

func (s *Schema) GetExtension(name string) any {
	if s.ExtraProps == nil {
		return nil
	}
	return s.ExtraProps[name]
}

func (s Schema) Empty() bool {
	return len(s.Type) == 0
}

// MarshalJSON marshal this to JSON
func (s Schema) MarshalJSON() ([]byte, error) {
	type schemaAlias Schema
	props, err := json.Marshal(schemaAlias(s))
	if err != nil {
		return nil, err
	}
	extprops, err := json.Marshal(s.ExtraProps)
	if err != nil {
		return nil, err
	}
	return swag.ConcatJSON(props, extprops), nil
}

func (s *Schema) UnmarshalJSON(data []byte) error {
	type schemaAlias Schema
	var sch schemaAlias
	if err := json.Unmarshal(data, &sch); err != nil {
		return err
	}
	dict := map[string]any{}
	if err := json.Unmarshal(data, &dict); err != nil {
		return err
	}
	for k, v := range dict {
		if knownSchemaFields[k] {
			continue
		}
		if sch.ExtraProps == nil {
			sch.ExtraProps = map[string]any{}
		}
		sch.ExtraProps[k] = v
	}
	*s = Schema(sch)
	return nil
}
// SchemaProperties is a map representing the properties of a Schema object.
type SchemaProperties collections.OrderedMap[string, Schema]

func (s SchemaProperties) MarshalJSON() ([]byte, error) {
	return json.Marshal(collections.OrderedMap[string, Schema](s))
}

func (s *SchemaProperties) UnmarshalJSON(data []byte) error {
	var props collections.OrderedMap[string, Schema]
	if err := json.Unmarshal(data, &props); err != nil {
		return err
	}
	// must use stable sort to ensure the order of properties
	slices.SortStableFunc(props, func(a, b collections.OrderedMapEntry[string, Schema]) int {
		aOrder, _ := a.Value.GetExtension(XOrder).(float64)
		bOrder, _ := b.Value.GetExtension(XOrder).(float64)
		return CompareFloat(aOrder, bOrder)
	})
	*s = SchemaProperties(props)
	return nil
}

func CompareFloat[T float64](a, b T) int {
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}

// SchemaOrArray represents a value that can either be a Schema
// or an array of Schema. Mainly here for serialization purposes
type SchemaOrArray []Schema

// ContainsType returns true when one of the schemas is of the specified type

// MarshalJSON converts this schema object or array into JSON structure
func (s SchemaOrArray) MarshalJSON() ([]byte, error) {
	if len(s) > 0 {
		return json.Marshal(s[0])
	}
	return json.Marshal(s)
}

// UnmarshalJSON converts this schema object or array from a JSON structure
func (s *SchemaOrArray) UnmarshalJSON(data []byte) error {
	var first byte
	if len(data) > 1 {
		first = data[0]
	}
	if first == '{' {
		var sch Schema
		if err := json.Unmarshal(data, &sch); err != nil {
			return err
		}
		*s = SchemaOrArray{sch}
	}
	if first == '[' {
		var list []Schema
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*s = SchemaOrArray(list)
	}
	return nil
}

type SchemaOrBool struct {
	Allows bool
	Schema *Schema
}

func (s *SchemaOrBool) MarshalJSON() ([]byte, error) {
	if s.Allows {
		return json.Marshal(true)
	}
	if s.Schema != nil {
		return json.Marshal(s.Schema)
	}
	return json.Marshal(false)
}

func (s *SchemaOrBool) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("true")) {
		s.Allows = true
		return nil
	}
	if bytes.Equal(data, []byte("false")) {
		s.Allows = false
		return nil
	}
	var sch Schema
	if err := json.Unmarshal(data, &sch); err != nil {
		return err
	}
	s.Schema = &sch
	return nil
}

// SchemaOrStringArray represents a schema or a string array
type SchemaOrStringArray struct {
	Schema   *Schema
	Property []string
}

// MarshalJSON converts this schema object or array into JSON structure
func (s SchemaOrStringArray) MarshalJSON() ([]byte, error) {
	if len(s.Property) > 0 {
		return json.Marshal(s.Property)
	}
	if s.Schema != nil {
		return json.Marshal(s.Schema)
	}
	return []byte("null"), nil
}

// UnmarshalJSON converts this schema object or array from a JSON structure
func (s *SchemaOrStringArray) UnmarshalJSON(data []byte) error {
	var first byte
	if len(data) > 1 {
		first = data[0]
	}
	var nw SchemaOrStringArray
	if first == '{' {
		var sch Schema
		if err := json.Unmarshal(data, &sch); err != nil {
			return err
		}
		nw.Schema = &sch
	}
	if first == '[' {
		if err := json.Unmarshal(data, &nw.Property); err != nil {
			return err
		}
	}
	*s = nw
	return nil
}

type StringOrArray = spec.StringOrArray
