// Package jsonschema provides reusable JSON Schema processing independent of
// HTTP and OpenAPI document generation.
package jsonschema

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Effective resolves local references and composition keywords at one schema
// node for value. Callers walking an object tree invoke it again for each child
// with that child's value.
func Effective(root, schema Schema, value any) (Schema, error) {
	return effective(root, schema, value, nil)
}

func effective(root, schema Schema, value any, refs []string) (Schema, error) {
	result := Schema{}
	if schema.Ref != "" {
		if slices.Contains(refs, schema.Ref) {
			return Schema{}, fmt.Errorf("cyclic reference %q", schema.Ref)
		}
		resolved, err := resolveLocalReference(root, schema.Ref)
		if err != nil {
			return Schema{}, err
		}
		result, err = effective(root, resolved, value, append(slices.Clone(refs), schema.Ref))
		if err != nil {
			return Schema{}, err
		}
	}
	result = Merge(result, schema)

	if schema.If != nil {
		condition, err := effective(root, *schema.If, value, refs)
		if err != nil {
			return Schema{}, err
		}
		branch := schema.Else
		if validateEffective(root, condition, value) == nil {
			branch = schema.Then
		}
		if branch != nil {
			resolved, err := effective(root, *branch, value, refs)
			if err != nil {
				return Schema{}, err
			}
			result = Merge(result, resolved)
		}
	}

	for i := range schema.AllOf {
		resolved, err := effective(root, schema.AllOf[i], value, refs)
		if err != nil {
			return Schema{}, err
		}
		result = Merge(result, resolved)
	}

	for i := range schema.AnyOf {
		candidate, err := effective(root, schema.AnyOf[i], value, refs)
		if err != nil {
			return Schema{}, err
		}
		if validateEffective(root, candidate, value) == nil {
			result = Merge(result, candidate)
		}
	}

	valid := make([]Schema, 0, 1)
	for i := range schema.OneOf {
		candidate, err := effective(root, schema.OneOf[i], value, refs)
		if err != nil {
			return Schema{}, err
		}
		if validateEffective(root, candidate, value) == nil {
			valid = append(valid, candidate)
		}
	}
	if len(valid) == 1 {
		result = Merge(result, valid[0])
	}

	// Composition has already been evaluated. Keeping it would make consumers
	// evaluate the original alternatives a second time.
	result.Ref = ""
	result.If = nil
	result.Then = nil
	result.Else = nil
	result.AllOf = nil
	result.AnyOf = nil
	result.OneOf = nil
	return result, nil
}

func validateEffective(root, schema Schema, value any) error {
	// Effective operates at a node while local references remain rooted at the
	// original document. Carry root definitions into the temporary validation
	// document so nested #/$defs references retain their original meaning.
	schema.Defs = mergeSchemaMaps(root.Defs, schema.Defs)
	return Validate(schema, value)
}

// Merge combines two schema fragments using the same rendering-oriented rules
// as schema-ts: required is a union, type is an intersection, and object schema
// maps are recursively merged. Other explicitly populated override fields take
// precedence.
func Merge(base, override Schema) Schema {
	result := base
	if override.ID != "" {
		result.ID = override.ID
	}
	if override.Schema != "" {
		result.Schema = override.Schema
	}
	if override.Comment != "" {
		result.Comment = override.Comment
	}
	if override.Anchor != "" {
		result.Anchor = override.Anchor
	}
	if override.DynamicAnchor != "" {
		result.DynamicAnchor = override.DynamicAnchor
	}
	if override.DynamicRef != "" {
		result.DynamicRef = override.DynamicRef
	}
	if override.Vocabulary != nil {
		result.Vocabulary = maps.Clone(override.Vocabulary)
	}
	if override.Title != "" {
		result.Title = override.Title
	}
	if override.Description != "" {
		result.Description = override.Description
	}
	if override.Default != nil {
		result.Default = override.Default
	}
	if override.Deprecated {
		result.Deprecated = true
	}
	if override.ReadOnly {
		result.ReadOnly = true
	}
	if override.WriteOnly {
		result.WriteOnly = true
	}
	if len(override.Examples) != 0 {
		result.Examples = slices.Clone(override.Examples)
	}
	if len(override.Type) != 0 {
		result.Type = intersectTypes(base.Type, override.Type)
	}
	if len(override.Enum) != 0 {
		result.Enum = slices.Clone(override.Enum)
	}
	if override.Const != nil {
		result.Const = override.Const
	}
	if override.MultipleOf != nil {
		result.MultipleOf = override.MultipleOf
	}
	if override.Maximum != nil {
		result.Maximum = override.Maximum
	}
	if override.ExclusiveMaximum != nil {
		result.ExclusiveMaximum = override.ExclusiveMaximum
	}
	if override.Minimum != nil {
		result.Minimum = override.Minimum
	}
	if override.ExclusiveMinimum != nil {
		result.ExclusiveMinimum = override.ExclusiveMinimum
	}
	if override.MaxLength != nil {
		result.MaxLength = override.MaxLength
	}
	if override.MinLength != nil {
		result.MinLength = override.MinLength
	}
	if override.Pattern != "" {
		result.Pattern = override.Pattern
	}
	if override.Format != "" {
		result.Format = override.Format
	}
	if override.MaxItems != nil {
		result.MaxItems = override.MaxItems
	}
	if override.MinItems != nil {
		result.MinItems = override.MinItems
	}
	if override.UniqueItems {
		result.UniqueItems = true
	}
	if override.MaxContains != nil {
		result.MaxContains = override.MaxContains
	}
	if override.MinContains != nil {
		result.MinContains = override.MinContains
	}
	if override.MaxProperties != nil {
		result.MaxProperties = override.MaxProperties
	}
	if override.MinProperties != nil {
		result.MinProperties = override.MinProperties
	}
	result.Required = unionStrings(base.Required, override.Required)
	result.Properties = mergeProperties(base.Properties, override.Properties)
	result.PatternProperties = mergeProperties(base.PatternProperties, override.PatternProperties)
	result.DependentRequired = mergeStringMaps(base.DependentRequired, override.DependentRequired)
	result.DependentSchemas = mergeSchemaMaps(base.DependentSchemas, override.DependentSchemas)
	result.Defs = mergeSchemaMaps(base.Defs, override.Defs)
	if override.PropertyNames != nil {
		result.PropertyNames = override.PropertyNames
	}
	if override.Contains != nil {
		result.Contains = override.Contains
	}
	if base.Items != nil && override.Items != nil {
		merged := Merge(*base.Items, *override.Items)
		result.Items = &merged
	} else if override.Items != nil {
		item := *override.Items
		result.Items = &item
	}
	result.PrefixItems = mergeSchemaSlices(base.PrefixItems, override.PrefixItems)
	if override.Not != nil {
		result.Not = override.Not
	}
	if override.AdditionalProperties != nil {
		result.AdditionalProperties = override.AdditionalProperties
	}
	if override.UnevaluatedProperties != nil {
		result.UnevaluatedProperties = override.UnevaluatedProperties
	}
	if override.UnevaluatedItems != nil {
		result.UnevaluatedItems = override.UnevaluatedItems
	}
	if override.ContentEncoding != "" {
		result.ContentEncoding = override.ContentEncoding
	}
	if override.ContentMediaType != "" {
		result.ContentMediaType = override.ContentMediaType
	}
	if override.ContentSchema != nil {
		result.ContentSchema = override.ContentSchema
	}
	result.ExtraProps = maps.Clone(base.ExtraProps)
	if result.ExtraProps == nil && override.ExtraProps != nil {
		result.ExtraProps = map[string]any{}
	}
	maps.Copy(result.ExtraProps, override.ExtraProps)
	return result
}

func mergeProperties(base, override Properties) Properties {
	result := slices.Clone(base)
	for _, property := range override {
		matched := false
		for i := range result {
			if result[i].Name == property.Name {
				result[i].Schema = Merge(result[i].Schema, property.Schema)
				matched = true
				break
			}
		}
		if !matched {
			result = append(result, property)
		}
	}
	return result
}

func intersectTypes(base, override Types) Types {
	if len(base) == 0 {
		return slices.Clone(override)
	}
	result := make(Types, 0, len(base))
	for _, value := range base {
		if slices.Contains(override, value) {
			result = append(result, value)
		}
	}
	return result
}

func unionStrings(base, override []string) []string {
	result := slices.Clone(base)
	for _, value := range override {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func mergeStringMaps(base, override map[string][]string) map[string][]string {
	if base == nil && override == nil {
		return nil
	}
	result := maps.Clone(base)
	if result == nil {
		result = map[string][]string{}
	}
	for key, values := range override {
		result[key] = unionStrings(result[key], values)
	}
	return result
}

func mergeSchemaMaps(base, override map[string]Schema) map[string]Schema {
	if base == nil && override == nil {
		return nil
	}
	result := maps.Clone(base)
	if result == nil {
		result = map[string]Schema{}
	}
	for key, schema := range override {
		if current, ok := result[key]; ok {
			result[key] = Merge(current, schema)
		} else {
			result[key] = schema
		}
	}
	return result
}

func mergeSchemaSlices(base, override []Schema) []Schema {
	if len(base) == 0 {
		return slices.Clone(override)
	}
	if len(override) == 0 {
		return slices.Clone(base)
	}
	result := make([]Schema, max(len(base), len(override)))
	for i := range result {
		switch {
		case i < len(base) && i < len(override):
			result[i] = Merge(base[i], override[i])
		case i < len(override):
			result[i] = override[i]
		default:
			result[i] = base[i]
		}
	}
	return result
}

func resolveLocalReference(root Schema, reference string) (Schema, error) {
	if !strings.HasPrefix(reference, "#/") {
		return Schema{}, fmt.Errorf("unsupported non-local reference %q", reference)
	}
	segments, err := parsePointer(strings.TrimPrefix(reference, "#"))
	if err != nil {
		return Schema{}, fmt.Errorf("invalid reference %q: %w", reference, err)
	}
	current := root
	for len(segments) > 0 {
		if len(segments) < 2 {
			return Schema{}, fmt.Errorf("invalid reference %q", reference)
		}
		switch segments[0] {
		case "$defs":
			var ok bool
			current, ok = current.Defs[segments[1]]
			if !ok {
				return Schema{}, fmt.Errorf("reference %q was not found", reference)
			}
		case "properties":
			found := false
			for _, property := range current.Properties {
				if property.Name == segments[1] {
					current = property.Schema
					found = true
					break
				}
			}
			if !found {
				return Schema{}, fmt.Errorf("reference %q was not found", reference)
			}
		default:
			return Schema{}, fmt.Errorf("unsupported reference %q", reference)
		}
		segments = segments[2:]
	}
	return current, nil
}

func parsePointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("must be an RFC 6901 JSON Pointer")
	}
	segments := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i, segment := range segments {
		var decoded strings.Builder
		for j := 0; j < len(segment); j++ {
			if segment[j] != '~' {
				decoded.WriteByte(segment[j])
				continue
			}
			if j+1 >= len(segment) || segment[j+1] != '0' && segment[j+1] != '1' {
				return nil, fmt.Errorf("contains an invalid escape")
			}
			j++
			if segment[j] == '0' {
				decoded.WriteByte('~')
			} else {
				decoded.WriteByte('/')
			}
		}
		segments[i] = decoded.String()
	}
	return segments, nil
}
