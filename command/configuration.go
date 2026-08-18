package command

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"xiaoshiai.cn/common/log"
	libreflect "xiaoshiai.cn/common/reflect"
)

type configurationChange struct {
	// Property is the formatted semantic configuration path used in logs and
	// errors. Struct fields use dot notation and dynamic map keys use brackets,
	// such as "Database.Address" and "Labels[region]". This is the command
	// package's configuration path syntax, not JSONPath.
	Property string
	// Path locates the target value and preserves whether each segment is a
	// struct field or a map key.
	Path  []propertyPart
	Value any
	// EnsureObject preserves an empty object's presence by allocating its pointer
	// or map without assigning a leaf value. Unlike a null change, it does not
	// clear the target or delete a map entry.
	EnsureObject bool
	Sensitive    bool
}

func applyConfigurationSourceValue(
	ctx context.Context,
	target any,
	schema *libreflect.Node,
	source string,
	input SourceValue,
	policy UnknownPropertyPolicy,
) error {
	changes, err := configurationSourceChanges(ctx, schema, source, input, policy)
	if err != nil {
		return err
	}
	targetValue := reflect.ValueOf(target).Elem()
	for _, change := range changes {
		if err := applyConfigurationValue(targetValue, schema, change.Path, change.Value, change.EnsureObject); err != nil {
			if change.Sensitive {
				return fmt.Errorf("configuration source %q input %q: property %q has an invalid sensitive value", source, input.Name, change.Property)
			}
			return fmt.Errorf("configuration source %q input %q: property %q: %w", source, input.Name, change.Property, err)
		}
		if change.EnsureObject {
			continue
		}
		if err := logConfigurationValue(ctx, source, input.Name, change); err != nil {
			return fmt.Errorf("configuration source %q input %q: property %q: %w", source, input.Name, change.Property, err)
		}
	}
	return nil
}

func logDefaultConfiguration(
	ctx context.Context,
	defaults any,
	schema *libreflect.Node,
	policy UnknownPropertyPolicy,
) error {
	changes, err := configurationSourceChanges(
		ctx,
		schema,
		"default",
		SourceValue{Name: "default", Value: defaults},
		policy,
	)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if !change.EnsureObject {
			if err := logConfigurationValue(ctx, "default", change.Property, change); err != nil {
				return fmt.Errorf("default configuration property %q: %w", change.Property, err)
			}
		}
	}
	return nil
}

func configurationSourceChanges(
	ctx context.Context,
	schema *libreflect.Node,
	source string,
	input SourceValue,
	policy UnknownPropertyPolicy,
) ([]configurationChange, error) {
	patch := libreflect.ParseStruct(input.Value, configurationTreeOptions())
	return compileConfigurationChanges(ctx, schema, &patch, nil, false, source, input.Name, policy)
}

func compileConfigurationChanges(
	ctx context.Context,
	schema *libreflect.Node,
	patch *libreflect.Node,
	path []propertyPart,
	sensitive bool,
	source string,
	input string,
	policy UnknownPropertyPolicy,
) ([]configurationChange, error) {
	sensitive = sensitive || configurationNodeSensitive(schema)
	if patch.Kind == libreflect.ValueNode && patch.Value.IsValid() && patch.Value.Kind() == reflect.String &&
		libreflect.IndirectType(schema.Type).Kind() != reflect.String &&
		!configurationStringDecoder(schema.Type) {
		structured, decoded, err := decodeStructuredConfigurationValue(schema, patch.Value.String())
		if err != nil {
			return nil, configurationCompileError(source, input, formatPropertyPath(path), err)
		}
		if decoded {
			node := libreflect.ParseStruct(structured, configurationTreeOptions())
			return compileConfigurationChanges(ctx, schema, &node, path, sensitive, source, input, policy)
		}
	}

	switch patch.Kind {
	case libreflect.NullNode:
		return []configurationChange{newConfigurationChange(path, nil, false, sensitive)}, nil
	case libreflect.ObjectNode:
		if schema.Kind != libreflect.ObjectNode {
			return []configurationChange{newConfigurationChange(path, patch.Value.Interface(), false, sensitive)}, nil
		}
		changes := []configurationChange{}
		if len(patch.Fields) == 0 {
			changes = append(changes, newConfigurationChange(path, nil, true, sensitive))
		}
		fields := append([]libreflect.Node(nil), patch.Fields...)
		sort.SliceStable(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
		for index := range fields {
			field := &fields[index]
			part := propertyPart{Name: field.Name}
			child := schema.Element
			if child != nil {
				part.MapKey = true
			} else {
				if strings.ContainsAny(field.Name, ".[]") {
					return nil, configurationCompileError(
						source,
						input,
						formatPropertyPath(path),
						fmt.Errorf("nested configuration property %q must be a field name", field.Name),
					)
				}
				child = configurationChild(schema, field.Name)
				if child == nil {
					property := formatPropertyPath(appendPropertyPart(path, part))
					if err := unknownConfigurationField(ctx, source, input, property, policy); err != nil {
						return nil, err
					}
					continue
				}
			}
			compiled, err := compileConfigurationChanges(
				ctx,
				child,
				field,
				appendPropertyPart(path, part),
				sensitive,
				source,
				input,
				policy,
			)
			if err != nil {
				return nil, err
			}
			changes = append(changes, compiled...)
		}
		return changes, nil
	case libreflect.ArrayNode:
		return []configurationChange{newConfigurationChange(path, patch.Value.Interface(), false, sensitive)}, nil
	case libreflect.ValueNode, libreflect.AnyNode:
		return []configurationChange{newConfigurationChange(path, patch.Value.Interface(), false, sensitive)}, nil
	default:
		return nil, configurationCompileError(source, input, formatPropertyPath(path), fmt.Errorf("unsupported configuration value"))
	}
}

func newConfigurationChange(path []propertyPart, value any, ensureObject, sensitive bool) configurationChange {
	return configurationChange{
		Property:     formatPropertyPath(path),
		Path:         append([]propertyPart(nil), path...),
		Value:        value,
		EnsureObject: ensureObject,
		Sensitive:    sensitive,
	}
}

func decodeStructuredConfigurationValue(node *libreflect.Node, text string) (any, bool, error) {
	switch node.Kind {
	case libreflect.ObjectNode:
		value := any(nil)
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, true, err
		}
		if parsed := libreflect.ParseStruct(value, configurationTreeOptions()); parsed.Kind != libreflect.ObjectNode {
			return nil, true, fmt.Errorf("requires an object")
		}
		return value, true, nil
	case libreflect.ArrayNode:
		if strings.HasPrefix(strings.TrimSpace(text), "[") {
			value := any(nil)
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				return nil, true, err
			}
			if parsed := libreflect.ParseStruct(value, configurationTreeOptions()); parsed.Kind != libreflect.ArrayNode {
				return nil, true, fmt.Errorf("requires an array")
			}
			return value, true, nil
		}
		parts := strings.Split(text, ",")
		values := make([]any, len(parts))
		for index, part := range parts {
			values[index] = part
		}
		return values, true, nil
	default:
		return nil, false, nil
	}
}

func configurationStringDecoder(typ reflect.Type) bool {
	return libreflect.ImplementsJSONUnmarshaler(typ) || libreflect.ImplementsTextUnmarshaler(typ)
}

func applyConfigurationValue(target reflect.Value, schema *libreflect.Node, path []propertyPart, value any, ensureObject bool) error {
	if len(path) == 0 {
		return setConfigurationValue(target, value, ensureObject)
	}
	for target.Kind() == reflect.Pointer {
		if target.IsNil() {
			target.Set(reflect.New(target.Type().Elem()))
		}
		target = target.Elem()
	}
	part := path[0]
	if schema.Element == nil {
		child := configurationChild(schema, part.Name)
		field := libreflect.FieldByIndexAlloc(target, configurationNodeIndex(child))
		return applyConfigurationValue(field, child, path[1:], value, ensureObject)
	}
	if target.IsNil() {
		target.Set(reflect.MakeMap(target.Type()))
	}
	key := reflect.ValueOf(part.Name).Convert(target.Type().Key())
	if len(path) == 1 && value == nil && !ensureObject {
		target.SetMapIndex(key, reflect.Value{})
		return nil
	}
	element := reflect.New(target.Type().Elem()).Elem()
	if current := target.MapIndex(key); current.IsValid() {
		element.Set(current)
	}
	if err := applyConfigurationValue(element, schema.Element, path[1:], value, ensureObject); err != nil {
		return err
	}
	target.SetMapIndex(key, element)
	return nil
}

func setConfigurationValue(target reflect.Value, value any, ensureObject bool) error {
	if ensureObject {
		for target.Kind() == reflect.Pointer {
			if target.IsNil() {
				target.Set(reflect.New(target.Type().Elem()))
			}
			target = target.Elem()
		}
		if target.Kind() == reflect.Map && target.IsNil() {
			target.Set(reflect.MakeMap(target.Type()))
		}
		return nil
	}
	if value == nil {
		if !libreflect.IsNilable(target.Type()) {
			return fmt.Errorf("cannot set nil to %v", target.Type())
		}
		target.SetZero()
		return nil
	}
	if kind := libreflect.IndirectType(target.Type()).Kind(); kind == reflect.Slice || kind == reflect.Array {
		return libreflect.MergePatch(target.Addr().Interface(), value, configurationTreeOptions())
	}
	return libreflect.SetValue(target, value)
}

func logConfigurationValue(
	ctx context.Context,
	source string,
	key string,
	change configurationChange,
) error {
	value := "<redacted>"
	if change.Sensitive {
		log.FromContext(ctx).V(1).Info("config", "from", source, "key", key, "val", value)
		return nil
	}
	var err error
	value, err = libreflect.FormatValue(change.Value)
	if err != nil {
		return err
	}
	log.FromContext(ctx).V(1).Info("config",
		"from", source,
		"key", key,
		"val", value,
	)
	return nil
}

func unknownConfigurationField(ctx context.Context, source, input, property string, policy UnknownPropertyPolicy) error {
	if policy == RejectUnknownProperties {
		return fmt.Errorf("configuration source %q input %q: unknown configuration property %q", source, input, property)
	}
	log.FromContext(ctx).V(1).Info("unknown configuration property", "source", source, "input", input, "property", property)
	return nil
}

func configurationCompileError(source, input, property string, err error) error {
	if property == "" {
		return fmt.Errorf("configuration source %q input %q: %w", source, input, err)
	}
	return fmt.Errorf("configuration source %q input %q: property %q: %w", source, input, property, err)
}

func appendPropertyPart(path []propertyPart, part propertyPart) []propertyPart {
	return append(append([]propertyPart(nil), path...), part)
}

func formatPropertyPath(parts []propertyPart) string {
	var result strings.Builder
	for _, part := range parts {
		if part.MapKey {
			result.WriteByte('[')
			result.WriteString(part.Name)
			result.WriteByte(']')
			continue
		}
		if result.Len() != 0 {
			result.WriteByte('.')
		}
		result.WriteString(part.Name)
	}
	return result.String()
}
