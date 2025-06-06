package openapi

import "github.com/go-openapi/spec"

func ConvertSchemaToSpecSchema(s Schema) spec.Schema {
	schema := spec.Schema{
		SchemaProps: spec.SchemaProps{
			ID:                   s.ID,
			Description:          s.Description,
			Type:                 s.Type,
			Nullable:             s.Nullable,
			Format:               s.Format,
			Title:                s.Title,
			Default:              s.Default,
			Maximum:              s.Maximum,
			ExclusiveMaximum:     s.ExclusiveMaximum,
			Minimum:              s.Minimum,
			ExclusiveMinimum:     s.ExclusiveMinimum,
			MaxLength:            s.MaxLength,
			MinLength:            s.MinLength,
			Pattern:              s.Pattern,
			MaxItems:             s.MaxItems,
			MinItems:             s.MinItems,
			UniqueItems:          s.UniqueItems,
			MultipleOf:           s.MultipleOf,
			Enum:                 s.Enum,
			MaxProperties:        s.MaxProperties,
			MinProperties:        s.MinProperties,
			Required:             s.Required,
			AllOf:                convertSchemas(s.AllOf),
			OneOf:                convertSchemas(s.OneOf),
			AnyOf:                convertSchemas(s.AnyOf),
			Not:                  convertSchemaPtr(s.Not),
			Properties:           convertSchemaProperties(s.Properties),
			AdditionalProperties: convertSchemaOrBool(s.AdditionalProperties),
			PatternProperties:    convertSchemaProperties(s.PatternProperties),
			Dependencies:         convertDependencies(s.Dependencies),
			AdditionalItems:      convertSchemaOrBool(s.AdditionalItems),
			Definitions:          convertDefinitions(s.Definitions),
			Items:                convertSchemaOrArray(s.Items),
		},
		VendorExtensible: spec.VendorExtensible{
			Extensions: s.Extensions,
		},
		ExtraProps: s.ExtraProps,
		SwaggerSchemaProps: spec.SwaggerSchemaProps{
			Example:       s.Example,
			Discriminator: s.Discriminator,
			ExternalDocs:  s.ExternalDocs,
			ReadOnly:      s.ReadOnly,
			XML:           s.XML,
		},
	}
	return schema
}

func convertSchemas(schemas []Schema) []spec.Schema {
	result := make([]spec.Schema, len(schemas))
	for i, schema := range schemas {
		result[i] = ConvertSchemaToSpecSchema(schema)
	}
	return result
}

func convertSchemaPtr(s *Schema) *spec.Schema {
	if s == nil {
		return nil
	}
	converted := ConvertSchemaToSpecSchema(*s)
	return &converted
}

func convertSchemaProperties(props SchemaProperties) spec.SchemaProperties {
	result := make(spec.SchemaProperties, len(props))
	for _, v := range props {
		result[v.Name] = ConvertSchemaToSpecSchema(v.Schema)
	}
	return result
}

func convertSchemaOrBool(s *SchemaOrBool) *spec.SchemaOrBool {
	if s == nil {
		return nil
	}
	return &spec.SchemaOrBool{Allows: s.Allows, Schema: convertSchemaPtr(s.Schema)}
}

func convertDependencies(deps map[string]SchemaOrStringArray) map[string]spec.SchemaOrStringArray {
	result := make(map[string]spec.SchemaOrStringArray, len(deps))
	for k, v := range deps {
		result[k] = spec.SchemaOrStringArray{Schema: convertSchemaPtr(v.Schema), Property: v.Property}
	}
	return result
}

func convertDefinitions(defs map[string]Schema) map[string]spec.Schema {
	result := make(map[string]spec.Schema, len(defs))
	for k, v := range defs {
		result[k] = ConvertSchemaToSpecSchema(v)
	}
	return result
}

func convertSchemaOrArray(s SchemaOrArray) *spec.SchemaOrArray {
	if len(s) == 0 {
		return nil
	}
	result := make([]spec.Schema, len(s))
	for i, schema := range s {
		result[i] = ConvertSchemaToSpecSchema(schema)
	}
	return &spec.SchemaOrArray{Schemas: result}
}
