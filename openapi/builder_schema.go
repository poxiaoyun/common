// Copyright 2022 The kubegems.io Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openapi

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

const ComponentsSchemasRoot = "#/components/schemas/"

var (
	DefaultSchemas = openapi3.Schemas{}
	DefaultBuilder = NewBuilder(InterfaceBuildOptionOverride, DefaultSchemas)
)

type Builder struct {
	InterfaceBuildOption InterfaceBuildOption
	Schemas              openapi3.Schemas
}

type InterfaceBuildOption string

const (
	InterfaceBuildOptionDefault  InterfaceBuildOption = ""         // use a concrete sample when available, otherwise object
	InterfaceBuildOptionOverride InterfaceBuildOption = "override" // use the concrete sample value when available
	InterfaceBuildOptionIgnore   InterfaceBuildOption = "ignore"   // omit interface fields
	InterfaceBuildOptionMerge    InterfaceBuildOption = "merge"    // accept both object and concrete sample value
)

type SchemaBuildFunc func(v reflect.Value) *openapi3.SchemaRef

func NewBuilder(interfaceOption InterfaceBuildOption, schemas openapi3.Schemas) *Builder {
	if schemas == nil {
		schemas = openapi3.Schemas{}
	}
	return &Builder{
		Schemas:              schemas,
		InterfaceBuildOption: interfaceOption,
	}
}

func (b *Builder) Build(data any) *openapi3.SchemaRef {
	return b.BuildSchema(reflect.ValueOf(data))
}

var WellKnownGoTypeAsSchema = map[reflect.Type]*openapi3.SchemaRef{
	reflect.TypeOf(json.Number("")):      schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNumber), Format: "double"}),
	reflect.TypeOf(json.RawMessage(nil)): AnyProperty(),
	reflect.TypeOf([]byte(nil)):          schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeString), Format: "byte", ContentEncoding: "base64"}),
	reflect.TypeOf(time.Time{}):          schemaValue(openapi3.NewDateTimeSchema()),
	reflect.TypeOf(time.Duration(0)):     schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeInteger), Format: "int64", Description: "Duration in nanoseconds."}),
}

func (b *Builder) BuildSchema(v reflect.Value) *openapi3.SchemaRef {
	if !v.IsValid() {
		return nil
	}

	if schema, ok := WellKnownGoTypeAsSchema[v.Type()]; ok {
		return cloneSchemaRef(schema)
	}

	switch v.Kind() {
	case reflect.Bool:
		return schemaValue(openapi3.NewBoolSchema())
	case reflect.Float32:
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNumber), Format: "float"})
	case reflect.Float64:
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNumber), Format: "double"})
	case reflect.Complex64, reflect.Complex128:
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNumber), Format: v.Kind().String()})
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return integerProperty(v.Kind().String())
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return integerProperty("int32")
	case reflect.Int64, reflect.Int:
		return integerProperty("int64")
	case reflect.String:
		return schemaValue(openapi3.NewStringSchema())
	case reflect.Struct:
		return b.buildStruct(v)
	case reflect.Slice, reflect.Array:
		return b.buildSlice(v)
	case reflect.Interface:
		return b.buildInterface(v)
	case reflect.Map:
		return b.buildMap(v)
	case reflect.Ptr:
		return b.buildPtr(v)
	default:
		return ObjectProperty()
	}
}

// TypeName returns the type name and generic type parameters.
func TypeName(t reflect.Type) (string, string) {
	fullname := t.String()
	if index := strings.IndexRune(fullname, '['); index != -1 {
		subname := fullname[index:]
		if i := strings.LastIndex(subname, "/"); i != -1 {
			subname = "[" + subname[i+1:]
		}
		subname = strings.ReplaceAll(subname, "·", "_")
		return fullname[:index], subname
	}
	return fullname, ""
}

func componentName(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
}

func (b *Builder) buildPtr(v reflect.Value) *openapi3.SchemaRef {
	var ref *openapi3.SchemaRef
	if v.IsNil() {
		ref = b.BuildSchema(reflect.New(v.Type().Elem()).Elem())
	} else {
		ref = b.BuildSchema(v.Elem())
	}
	return nullableSchemaRef(ref)
}

func (b *Builder) buildSlice(v reflect.Value) *openapi3.SchemaRef {
	schema := openapi3.NewArraySchema()
	items := make(openapi3.SchemaRefs, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		if itemSchema := b.BuildSchema(v.Index(i)); itemSchema != nil {
			items = append(items, itemSchema)
		}
	}

	switch len(items) {
	case 0:
		schema.Items = b.BuildSchema(reflect.New(v.Type().Elem()).Elem())
	case 1:
		schema.Items = items[0]
	default:
		schema.Items = schemaValue(&openapi3.Schema{AnyOf: items})
	}
	return schemaValue(schema)
}

func (b *Builder) buildInterface(v reflect.Value) *openapi3.SchemaRef {
	switch b.InterfaceBuildOption {
	case InterfaceBuildOptionMerge:
		if !v.IsNil() {
			if innerSchema := b.BuildSchema(v.Elem()); innerSchema != nil {
				return schemaValue(&openapi3.Schema{AnyOf: openapi3.SchemaRefs{ObjectProperty(), innerSchema}})
			}
		}
	case InterfaceBuildOptionOverride, InterfaceBuildOptionDefault:
		if !v.IsNil() {
			return b.BuildSchema(v.Elem())
		}
	case InterfaceBuildOptionIgnore:
		return nil
	}
	return ObjectProperty()
}

func (b *Builder) buildMap(v reflect.Value) *openapi3.SchemaRef {
	itemSchema := b.BuildSchema(reflect.New(v.Type().Elem()).Elem())
	schema := openapi3.NewObjectSchema()
	schema.AdditionalProperties = openapi3.AdditionalProperties{Schema: itemSchema}

	if v.Type().Key().Kind() == reflect.String {
		for _, key := range v.MapKeys() {
			if keySchema := b.BuildSchema(v.MapIndex(key)); keySchema != nil {
				schema.Properties[key.String()] = keySchema
			}
		}
	}
	return schemaValue(schema)
}

// buildStruct adds a reusable component schema and returns a reference to it.
// Concrete values in dynamic interface fields are overlaid with allOf without
// changing the reusable base component.
func (b *Builder) buildStruct(v reflect.Value) *openapi3.SchemaRef {
	if b.Schemas == nil {
		b.Schemas = openapi3.Schemas{}
	}

	findOverridesOnly := false
	typeName, suffix := TypeName(v.Type())
	structTypeName := typeName
	if !HasDynamicField(v.Type()) {
		structTypeName += suffix
	}
	structTypeName = componentName(structTypeName)

	originalSchema := openapi3.NewObjectSchema()
	if existing, ok := b.Schemas[structTypeName]; ok {
		findOverridesOnly = true
		if existing.Value != nil {
			originalSchema = existing.Value
		}
	} else {
		b.Schemas[structTypeName] = schemaValue(originalSchema)
	}

	overrideProperties := openapi3.Schemas{}
	embeddedSchemas := openapi3.SchemaRefs{}
	for i := 0; i < v.NumField(); i++ {
		fieldValue, structField := v.Field(i), v.Type().Field(i)
		isEmbedded, isIgnored, fieldName, isRequired := structFieldInfo(structField)
		if isIgnored {
			continue
		}
		if !isEmbedded && IsDynamicInterface(structField) {
			if fieldSchema := b.BuildSchema(fieldValue); fieldSchema != nil {
				overrideProperties[fieldName] = fieldSchema
			} else if b.InterfaceBuildOption == InterfaceBuildOptionIgnore {
				continue
			}
			if originalSchema.Properties == nil {
				originalSchema.Properties = openapi3.Schemas{}
			}
			originalSchema.Properties[fieldName] = AnyProperty()
			if isRequired {
				originalSchema.Required = appendUnique(originalSchema.Required, fieldName)
			}
			continue
		}
		if findOverridesOnly {
			continue
		}
		fieldSchema := b.BuildSchema(fieldValue)
		if fieldSchema == nil {
			continue
		}
		if isEmbedded {
			embeddedSchemas = append(embeddedSchemas, fieldSchema)
			continue
		}
		originalSchema.Properties[fieldName] = fieldSchema
		if isRequired {
			originalSchema.Required = appendUnique(originalSchema.Required, fieldName)
		}
	}

	if len(embeddedSchemas) > 0 {
		allOf := openapi3.SchemaRefs{}
		if len(originalSchema.Properties) != 0 || originalSchema.AdditionalProperties.Schema != nil || originalSchema.AdditionalProperties.Has != nil {
			allOf = append(allOf, schemaValue(originalSchema))
		}
		allOf = append(allOf, embeddedSchemas...)
		originalSchema = &openapi3.Schema{AllOf: allOf}
	}
	if !findOverridesOnly {
		b.Schemas[structTypeName] = schemaValue(originalSchema)
	}

	ref := &openapi3.SchemaRef{
		Ref:   ComponentsSchemasRoot + structTypeName,
		Value: b.Schemas[structTypeName].Value,
	}
	if len(overrideProperties) == 0 {
		return ref
	}
	return schemaValue(&openapi3.Schema{
		AllOf: openapi3.SchemaRefs{
			ref,
			ObjectPropertyProperties(overrideProperties),
		},
	})
}

func structFieldInfo(structField reflect.StructField) (embedded, ignored bool, name string, required bool) {
	if !structField.IsExported() {
		return false, true, "", false
	}
	embedded, name, required = structField.Anonymous, structField.Name, true
	if jsonTag := structField.Tag.Get("json"); jsonTag != "" {
		opts := strings.Split(jsonTag, ",")
		switch opts[0] {
		case "-":
			return false, true, "", false
		case "":
		default:
			name = opts[0]
			embedded = false
		}
		for _, opt := range opts[1:] {
			switch opt {
			case "inline":
				embedded = true
			case "omitempty", "omitzero":
				required = false
			}
		}
	}
	if embedded {
		required = false
	}
	return embedded, false, name, required
}

func HasDynamicField(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		if IsDynamicInterface(t.Field(i)) {
			return true
		}
	}
	return false
}

func IsDynamicInterface(field reflect.StructField) bool {
	if tag := field.Tag.Get("openapi"); tag != "" {
		if slices.Contains(strings.Split(tag, ","), "dynamic") {
			return true
		}
	}
	t := field.Type
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	return t.Kind() == reflect.Interface
}

func integerProperty(format string) *openapi3.SchemaRef {
	return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeInteger), Format: format})
}

func ObjectProperty() *openapi3.SchemaRef {
	return schemaValue(openapi3.NewObjectSchema())
}

func AnyProperty() *openapi3.SchemaRef {
	return schemaValue(openapi3.NewSchema())
}

func ObjectPropertyProperties(properties openapi3.Schemas) *openapi3.SchemaRef {
	return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeObject), Properties: properties})
}

func schemaValue(schema *openapi3.Schema) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: schema}
}

func schemaTypes(types ...string) *openapi3.Types {
	t := openapi3.Types(types)
	return &t
}

func cloneSchemaRef(ref *openapi3.SchemaRef) *openapi3.SchemaRef {
	if ref == nil {
		return nil
	}
	cloned := &openapi3.SchemaRef{Ref: ref.Ref}
	if ref.Value != nil {
		value := *ref.Value
		cloned.Value = &value
	}
	return cloned
}

func nullableSchemaRef(ref *openapi3.SchemaRef) *openapi3.SchemaRef {
	if ref == nil {
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNull)})
	}
	if ref.Ref == "" && ref.Value != nil && ref.Value.Type != nil {
		value := *ref.Value
		types := slices.Clone(value.Type.Slice())
		if !slices.Contains(types, openapi3.TypeNull) {
			types = append(types, openapi3.TypeNull)
		}
		value.Type = schemaTypes(types...)
		return schemaValue(&value)
	}
	return schemaValue(&openapi3.Schema{AnyOf: openapi3.SchemaRefs{
		ref,
		schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNull)}),
	}})
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}
