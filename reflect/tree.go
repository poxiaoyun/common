package reflect

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

// NodeKind describes the value represented by a Node.
type NodeKind uint8

const (
	// InvalidNode represents a Go value that has no supported tree semantics.
	InvalidNode NodeKind = iota
	// NullNode represents a nil pointer value.
	NullNode
	// ValueNode represents a scalar or self-decoding value.
	ValueNode
	// ObjectNode represents a struct or a string-keyed map.
	ObjectNode
	// ArrayNode represents a slice or array.
	ArrayNode
	// AnyNode represents an empty interface value without a runtime value.
	AnyNode
)

// Node is one value in a semantic reflection tree.
type Node struct {
	// Name is the resolved field name, map key, or collection index.
	Name string
	// Kind describes the value's semantic role in the tree.
	Kind NodeKind
	// Type is the declared Go type, including pointer layers.
	Type reflect.Type
	// Tag is the tag of the final struct field in Path.
	Tag reflect.StructTag
	// Value is the runtime value after dereferencing non-nil pointers. It is
	// invalid for declared Element nodes.
	Value reflect.Value
	// Path contains the struct fields traversed from the parent node. Inline
	// fields are included in this path but do not appear as separate nodes.
	Path []reflect.StructField
	// Fields contains the final semantic children. Struct fields, map entries,
	// and collection items all appear directly in this slice.
	Fields []Node
	// Element is the semantic type of a map value or collection item.
	Element *Node
}

// Options controls semantic tree parsing.
type Options struct {
	// TagNames defines field-tag precedence. The default is json followed by yaml.
	TagNames []string
	// IgnoreOmitEmpty keeps empty runtime fields even when the selected tag
	// contains omitempty. Nil pointer fields remain excluded.
	IgnoreOmitEmpty bool
}

// ParseStruct builds the semantic runtime tree represented by data. Ignored
// and nil-pointer fields are excluded, inline fields are flattened, and empty
// omitempty fields are excluded unless Options.IgnoreOmitEmpty is set.
func ParseStruct(data any, options Options) Node {
	options = resolveOptions(options)
	value := reflect.ValueOf(data)
	if !value.IsValid() {
		return Node{Kind: NullNode}
	}
	return parseValueNode("", "", value.Type(), value, nil, options)
}

func resolveOptions(options Options) Options {
	if len(options.TagNames) == 0 {
		options.TagNames = []string{"json", "yaml"}
	}
	return options
}

func parseValueNode(name string, tag reflect.StructTag, typ reflect.Type, value reflect.Value, path []reflect.StructField, options Options) Node {
	for value.IsValid() {
		switch value.Kind() {
		case reflect.Interface:
			if value.IsNil() {
				return Node{Name: name, Kind: NullNode, Type: typ, Tag: tag, Value: value, Path: path}
			}
			value = value.Elem()
			typ = value.Type()
		case reflect.Pointer:
			if value.IsNil() {
				return Node{Name: name, Kind: NullNode, Type: typ, Tag: tag, Value: value, Path: path}
			}
			value = value.Elem()
		default:
			goto parsed
		}
	}

parsed:
	node := Node{Name: name, Kind: nodeKindForType(typ), Type: typ, Tag: tag, Value: value, Path: path}
	switch node.Kind {
	case ObjectNode:
		if value.Kind() == reflect.Struct {
			if node.Name == "" {
				node.Name = value.Type().Name()
			}
			for i := 0; i < value.NumField(); i++ {
				field := value.Type().Field(i)
				if field.PkgPath != "" {
					continue
				}
				inline, ignored, omitEmpty, fieldName := StructFieldInfoNByTags(field, options.TagNames...)
				if ignored {
					continue
				}
				fieldValue := value.Field(i)
				if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() ||
					!options.IgnoreOmitEmpty && omitEmpty && isEmptyValue(fieldValue) {
					continue
				}
				child := parseValueNode(fieldName, field.Tag, field.Type, fieldValue, []reflect.StructField{field}, options)
				if inline {
					node.Fields = append(node.Fields, inlineFields(child)...)
				} else {
					node.Fields = append(node.Fields, child)
				}
			}
			return node
		}
		element, err := parseTypeNode("", "", value.Type().Elem(), nil, options, map[reflect.Type]bool{})
		if err != nil {
			return Node{Name: name, Kind: InvalidNode, Type: typ, Tag: tag, Value: value, Path: path}
		}
		node.Element = &element
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, key := range keys {
			node.Fields = append(node.Fields, parseValueNode(key.String(), "", value.Type().Elem(), value.MapIndex(key), nil, options))
		}
	case ArrayNode:
		element, err := parseTypeNode("", "", value.Type().Elem(), nil, options, map[reflect.Type]bool{})
		if err != nil {
			return Node{Name: name, Kind: InvalidNode, Type: typ, Tag: tag, Value: value, Path: path}
		}
		node.Element = &element
		for i := 0; i < value.Len(); i++ {
			node.Fields = append(node.Fields, parseValueNode(strconv.Itoa(i), "", value.Type().Elem(), value.Index(i), nil, options))
		}
	}
	return node
}

func parseTypeNode(name string, tag reflect.StructTag, typ reflect.Type, path []reflect.StructField, options Options, stack map[reflect.Type]bool) (Node, error) {
	indirectType := IndirectType(typ)
	node := Node{Name: name, Kind: nodeKindForType(typ), Type: typ, Tag: tag, Path: path}
	if node.Kind != ObjectNode && node.Kind != ArrayNode {
		return node, nil
	}
	if stack[indirectType] {
		return Node{}, fmt.Errorf("recursive type %v", indirectType)
	}
	stack[indirectType] = true
	defer delete(stack, indirectType)
	if node.Kind == ArrayNode || indirectType.Kind() == reflect.Map {
		element, err := parseTypeNode("", "", indirectType.Elem(), nil, options, stack)
		if err != nil {
			return Node{}, err
		}
		node.Element = &element
		return node, nil
	}
	for i := 0; i < indirectType.NumField(); i++ {
		field := indirectType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		inline, ignored, _, fieldName := StructFieldInfoNByTags(field, options.TagNames...)
		if ignored {
			continue
		}
		child, err := parseTypeNode(fieldName, field.Tag, field.Type, []reflect.StructField{field}, options, stack)
		if err != nil {
			return Node{}, err
		}
		if inline {
			node.Fields = append(node.Fields, inlineFields(child)...)
		} else {
			node.Fields = append(node.Fields, child)
		}
	}
	return node, nil
}

func nodeKindForType(typ reflect.Type) NodeKind {
	if ImplementsTextUnmarshaler(typ) {
		return ValueNode
	}
	typ = IndirectType(typ)
	switch typ.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return ValueNode
	case reflect.Struct:
		return ObjectNode
	case reflect.Map:
		if typ.Key().Kind() == reflect.String {
			return ObjectNode
		}
	case reflect.Slice, reflect.Array:
		return ArrayNode
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			return AnyNode
		}
	}
	return InvalidNode
}

func inlineFields(node Node) []Node {
	fields := make([]Node, len(node.Fields))
	for i, field := range node.Fields {
		field.Path = append(append([]reflect.StructField(nil), node.Path...), field.Path...)
		fields[i] = field
	}
	return fields
}

func isEmptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return value.IsZero()
	default:
		return false
	}
}
