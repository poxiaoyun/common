package command

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	libreflect "xiaoshiai.cn/common/reflect"
)

func configurationTreeOptions() libreflect.Options {
	return libreflect.Options{
		TagNames:        []string{"config", "json"},
		IgnoreOmitEmpty: true,
	}
}

type projectedConfigurationField struct {
	Node      *libreflect.Node
	Canonical string
	GoPath    string
	Sensitive bool
}

func compileConfigurationSchema(options any) (*libreflect.Node, error) {
	root := libreflect.ParseStruct(options, configurationTreeOptions())
	if root.Kind != libreflect.ObjectNode || root.Element != nil {
		return nil, fmt.Errorf("options must be a struct, got %v", root.Type)
	}
	if err := validateConfigurationNode(&root, nil, false); err != nil {
		return nil, err
	}
	flags := map[string]projectedConfigurationField{}
	environments := map[string]projectedConfigurationField{}
	for _, field := range projectConfigurationFields(&root, "", nil, false, nil) {
		flag := configurationFlagName(field.Canonical)
		if existing, exists := flags[flag]; exists {
			return nil, fmt.Errorf("configuration fields %s and %s both define --%s", existing.GoPath, field.GoPath, flag)
		}
		if flag == "help" || flag == "config-file" {
			return nil, fmt.Errorf("configuration field %s defines reserved option --%s", field.GoPath, flag)
		}
		flags[flag] = field
		environment := configurationEnvironmentName(field.Canonical)
		if existing, exists := environments[environment]; exists {
			return nil, fmt.Errorf("configuration fields %s and %s both define %s", existing.GoPath, field.GoPath, environment)
		}
		environments[environment] = field
	}
	return &root, nil
}

func validateConfigurationNode(node *libreflect.Node, goPath []string, dynamic bool) error {
	path := strings.Join(goPath, ".")
	switch node.Kind {
	case libreflect.InvalidNode, libreflect.NullNode:
		return fmt.Errorf("configuration field %s has unsupported type %v; exclude it with config:\"-\"", path, node.Type)
	case libreflect.AnyNode:
		if !dynamic {
			return fmt.Errorf("configuration field %s has unsupported type %v; exclude it with config:\"-\"", path, node.Type)
		}
	case libreflect.ObjectNode:
		if node.Element != nil {
			return validateConfigurationNode(node.Element, append(goPath, "<key>"), true)
		}
		for index := range node.Fields {
			field := &node.Fields[index]
			fieldPath := configurationFieldGoPath(goPath, field)
			for previous := range index {
				if node.Fields[previous].Name == field.Name {
					return fmt.Errorf(
						"configuration fields %s and %s both define %s",
						strings.Join(configurationFieldGoPath(goPath, &node.Fields[previous]), "."),
						strings.Join(fieldPath, "."),
						field.Name,
					)
				}
			}
			if err := validateConfigurationNode(field, fieldPath, false); err != nil {
				return err
			}
		}
	case libreflect.ArrayNode:
		return validateConfigurationNode(node.Element, append(goPath, "<item>"), true)
	}
	return nil
}

func projectConfigurationFields(
	node *libreflect.Node,
	canonical string,
	goPath []string,
	sensitive bool,
	fields []projectedConfigurationField,
) []projectedConfigurationField {
	sensitive = sensitive || configurationNodeSensitive(node)
	if node.Kind == libreflect.ObjectNode && node.Element == nil {
		if canonical != "" && configurationStringDecoder(node.Type) {
			fields = append(fields, projectedConfigurationField{
				Node:      node,
				Canonical: canonical,
				GoPath:    strings.Join(goPath, "."),
				Sensitive: sensitive,
			})
		}
		children := make([]*libreflect.Node, len(node.Fields))
		for index := range node.Fields {
			children[index] = &node.Fields[index]
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
		for _, child := range children {
			fields = projectConfigurationFields(
				child,
				joinCanonical(canonical, child.Name),
				configurationFieldGoPath(goPath, child),
				sensitive,
				fields,
			)
		}
		return fields
	}
	if canonical != "" {
		fields = append(fields, projectedConfigurationField{
			Node:      node,
			Canonical: canonical,
			GoPath:    strings.Join(goPath, "."),
			Sensitive: sensitive,
		})
	}
	return fields
}

func configurationFieldGoPath(parent []string, node *libreflect.Node) []string {
	path := append([]string(nil), parent...)
	for _, field := range node.Path {
		path = append(path, field.Name)
	}
	return path
}

func configurationFieldSensitive(field reflect.StructField) bool {
	for _, option := range strings.Split(field.Tag.Get("config"), ",")[1:] {
		if option == "sensitive" {
			return true
		}
	}
	return false
}

func encodeConfigurationNode(node *libreflect.Node, value reflect.Value) any {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	switch node.Kind {
	case libreflect.ObjectNode:
		if node.Element != nil {
			if value.IsNil() {
				return nil
			}
			object := map[string]any{}
			iterator := value.MapRange()
			for iterator.Next() {
				object[iterator.Key().String()] = encodeConfigurationNode(node.Element, iterator.Value())
			}
			return object
		}
		object := map[string]any{}
		for index := range node.Fields {
			child := &node.Fields[index]
			field, exists := libreflect.FieldByIndex(value, configurationNodeIndex(child))
			if !exists {
				continue
			}
			object[child.Name] = encodeConfigurationNode(child, field)
		}
		return object
	case libreflect.ArrayNode:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		items := make([]any, value.Len())
		for index := 0; index < value.Len(); index++ {
			items[index] = encodeConfigurationNode(node.Element, value.Index(index))
		}
		return items
	case libreflect.AnyNode, libreflect.ValueNode:
		return value.Interface()
	default:
		panic("unknown configuration node kind")
	}
}

func configurationNodeIndex(node *libreflect.Node) []int {
	index := []int{}
	for _, field := range node.Path {
		index = append(index, field.Index...)
	}
	return index
}

func configurationNodeSensitive(node *libreflect.Node) bool {
	for _, field := range node.Path {
		if configurationFieldSensitive(field) {
			return true
		}
	}
	return false
}
