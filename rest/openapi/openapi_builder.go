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
	"strings"
	"time"

	"github.com/go-openapi/spec"
)

var (
	DefaultDefinitions = map[string]spec.Schema{}
	DefaultBuilder     = NewBuilder(InterfaceBuildOptionOverride, DefaultDefinitions)
)

const DefinitionsRoot = "#/definitions/"

type Builder struct {
	InterfaceBuildOption InterfaceBuildOption
	Definitions          map[string]spec.Schema
}

type InterfaceBuildOption string

const (
	InterfaceBuildOptionDefault  InterfaceBuildOption = ""         // default as an 'object{}'
	InterfaceBuildOptionOverride InterfaceBuildOption = "override" // override using interface's value if exist
	InterfaceBuildOptionIgnore   InterfaceBuildOption = "ignore"   // ignore interface field
	InterfaceBuildOptionMerge    InterfaceBuildOption = "merge"    // anyOf 'object{}' type and interface's value type
)

type SchemaBuildFunc func(v reflect.Value) *spec.Schema

func NewBuilder(interfaceOption InterfaceBuildOption, definations map[string]spec.Schema) *Builder {
	return &Builder{
		Definitions:          definations,
		InterfaceBuildOption: interfaceOption,
	}
}

func (b *Builder) Build(data any) *spec.Schema {
	return b.BuildSchema(reflect.ValueOf(data))
}

var WellKnowGoTypeAsSchema = map[reflect.Type]spec.Schema{
	reflect.TypeOf(json.Number("")): *spec.Float64Property(), // json.Number is double

	// https://json-schema.org/draft/2020-12/json-schema-validation.html#rfc.section.7.3.1
	reflect.TypeOf(time.Time{}):      *spec.DateTimeProperty(),         // time.Time is date-time
	reflect.TypeOf(time.Duration(0)): *spec.StrFmtProperty("duration"), // time.Duration is duration format

	// reflect.TypeOf((*any)(nil)).Elem(): *ObjectProperty(), // any as object
}

func (b *Builder) BuildSchema(v reflect.Value) *spec.Schema {
	if !v.IsValid() {
		return nil
	}

	if schema, ok := WellKnowGoTypeAsSchema[v.Type()]; ok {
		return &schema
	}

	// https://json-schema.org/draft/2020-12/json-schema-validation.html#rfc.section.6.1.1
	switch v.Kind() {
	case reflect.Bool:
		return spec.BooleanProperty()
	case reflect.Float32:
		return spec.Float32Property()
	case reflect.Float64:
		return spec.Float64Property()
	case reflect.Complex64, reflect.Complex128:
		return (&spec.Schema{}).Typed("number", v.Kind().String())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return IntFmtProperty(v.Kind().String())
	case reflect.Int8:
		return spec.Int8Property()
	case reflect.Int16:
		return spec.Int16Property()
	case reflect.Int32:
		return spec.Int32Property()
	case reflect.Int64, reflect.Int:
		return spec.Int64Property()
	case reflect.String:
		return spec.StringProperty()
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
		return ObjectProperty() // return object if not recognize
	}
}

func TypeName(t reflect.Type) string {
	fullname := t.String()
	if index := strings.IndexRune(fullname, '['); index != -1 {
		subname := fullname[index:]
		if i := strings.LastIndex(subname, "/"); i != -1 {
			subname = "[" + subname[i+1:]
		}
		subname = strings.ReplaceAll(subname, "·", "_")
		return fullname[:index] + subname
	}
	return fullname
}

func (b *Builder) buildPtr(v reflect.Value) *spec.Schema {
	if v.IsNil() {
		return b.BuildSchema(reflect.New(v.Type().Elem()).Elem())
	}
	return b.BuildSchema(v.Elem())
}

func (b *Builder) buildSlice(v reflect.Value) *spec.Schema {
	schema := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type: []string{"array"},
		},
	}
	items := []spec.Schema{}
	for i := 0; i < v.Len(); i++ {
		if itemSchema := b.BuildSchema(v.Index(i)); itemSchema != nil {
			items = append(items, *itemSchema)
		}
	}
	switch len(items) {
	case 0:
		if itemSchema := b.BuildSchema(reflect.New(v.Type().Elem())); itemSchema != nil {
			schema.Items = &spec.SchemaOrArray{Schema: itemSchema}
		}
	case 1:
		schema.Items = &spec.SchemaOrArray{Schema: &items[0]}
	default:
		schema.Items = &spec.SchemaOrArray{Schemas: items}
	}
	return &schema
}

func (b *Builder) buildInterface(v reflect.Value) *spec.Schema {
	switch b.InterfaceBuildOption {
	case InterfaceBuildOptionMerge:
		if innerSchema := b.BuildSchema(v.Elem()); innerSchema != nil {
			return &spec.Schema{
				SchemaProps: spec.SchemaProps{
					AnyOf: []spec.Schema{
						*ObjectProperty(),
						*innerSchema,
					},
				},
			}
		}
	case InterfaceBuildOptionOverride, InterfaceBuildOptionDefault:
		if v.IsNil() {
			return ObjectProperty()
		}
		return b.BuildSchema(v.Elem())
	case InterfaceBuildOptionIgnore:
		return nil
	}
	return ObjectProperty()
}

func (b *Builder) buildMap(v reflect.Value) *spec.Schema {
	itemSchema := b.BuildSchema(reflect.New(v.Type().Elem()))
	schema := &spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type: []string{"object"},
			AdditionalProperties: &spec.SchemaOrBool{
				Allows: true,
				Schema: itemSchema,
			},
		},
	}

	// fixed properties
	properties := spec.SchemaProperties{}
	for _, k := range v.MapKeys() {
		if keySchema := b.BuildSchema(v.MapIndex(k)); keySchema != nil {
			properties[k.String()] = *keySchema
		}
	}
	if len(properties) > 0 {
		schema.Properties = properties
	}

	return schema
}

type empty struct{}

// buildStruct build struct schema of a struct instance
// it will add a  definition into builder
// return the ref of definition
// if fields container interface value, the return ref will allof them
func (b *Builder) buildStruct(v reflect.Value) *spec.Schema {
	if b.Definitions == nil {
		b.Definitions = map[string]spec.Schema{}
	}

	findOverridesOnly := false // avoid recursive find
	structTypeName := TypeName(v.Type())

	orignalSchama := ObjectPropertyProperties(map[string]spec.Schema{})
	if exists, ok := b.Definitions[structTypeName]; ok {
		findOverridesOnly = true // only find overrides fields
		orignalSchama = &exists
	} else {
		b.Definitions[structTypeName] = *orignalSchama
	}

	overrideProperties, embeddedProperties := map[string]spec.Schema{}, []spec.Schema{}
	for i := 0; i < v.NumField(); i++ {
		fieldv, structField := v.Field(i), v.Type().Field(i)
		isEmbedded, isIgnored, fieldName := structFieldInfo(structField)
		if isIgnored {
			continue
		}
		// dynamic type will be treated as override properties
		if !isEmbedded && IsDynamicInterface(structField.Type) && !fieldv.IsNil() {
			if fieldSchema := b.BuildSchema(fieldv); fieldSchema != nil {
				overrideProperties[fieldName] = *fieldSchema
			}
			continue
		}
		if findOverridesOnly {
			continue
		}
		fieldSchema := b.BuildSchema(fieldv)
		if fieldSchema == nil {
			continue
		}
		if isEmbedded {
			embeddedProperties = append(embeddedProperties, *fieldSchema)
		} else {
			orignalSchama.Properties[fieldName] = *fieldSchema
		}
	}
	if len(embeddedProperties) > 0 {
		allofSchema := &spec.Schema{}
		// check if empty object
		if len(*&orignalSchama.Properties) != 0 || orignalSchama.AdditionalProperties != nil {
			allofSchema.AllOf = append(allofSchema.AllOf, *orignalSchama)
		}
		allofSchema.AllOf = append(allofSchema.AllOf, embeddedProperties...)
		orignalSchama = allofSchema
	}
	if !findOverridesOnly {
		b.Definitions[structTypeName] = *orignalSchama // add self definition
	}
	ret := spec.RefSchema(DefinitionsRoot + structTypeName)
	if len(overrideProperties) > 0 {
		overrideSchema := &spec.Schema{}
		overrideSchema.AllOf = []spec.Schema{*ret, *ObjectPropertyProperties(overrideProperties)}
		ret = overrideSchema
	}
	return ret
}

func structFieldInfo(structField reflect.StructField) (bool, bool, string) {
	if !structField.IsExported() {
		return false, true, ""
	}
	isEmbedded, isIgnored, fieldName := structField.Anonymous, false, structField.Name
	// json
	if jsonTag := structField.Tag.Get("json"); jsonTag != "" {
		opts := strings.Split(jsonTag, ",")
		switch val := opts[0]; val {
		case "-":
			isIgnored = true
		case "":
		default:
			fieldName = val
			isEmbedded = false // if field is embedded,but json tag has name,then it is not embedded
		}
		for _, opt := range opts[1:] {
			if opt == "inline" {
				isEmbedded = true
			}
		}
	}
	return isEmbedded, isIgnored, fieldName
}

func IsDynamicInterface(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	return t.Kind() == reflect.Interface
}

// StrFmtProperty creates a property for the named string format
func IntFmtProperty(format string) *spec.Schema {
	return &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"integer"}, Format: format}}
}

func ObjectProperty() *spec.Schema {
	return ObjectPropertyProperties(nil)
}

func ObjectPropertyProperties(properties spec.SchemaProperties) *spec.Schema {
	return &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"object"}, Properties: properties}}
}
