package reflect

import (
	"reflect"
	"testing"
)

func nodesEqual(left, right Node) bool {
	if left.Name != right.Name || left.Kind != right.Kind || left.Type != right.Type || left.Tag != right.Tag ||
		!reflect.DeepEqual(left.Path, right.Path) || len(left.Fields) != len(right.Fields) ||
		left.Value.IsValid() != right.Value.IsValid() || left.Element == nil != (right.Element == nil) {
		return false
	}
	if left.Value.IsValid() && !reflect.DeepEqual(left.Value.Interface(), right.Value.Interface()) {
		return false
	}
	for i := range left.Fields {
		if !nodesEqual(left.Fields[i], right.Fields[i]) {
			return false
		}
	}
	if left.Element != nil && !nodesEqual(*left.Element, *right.Element) {
		return false
	}
	return true
}

func TestParseStructReturnsSemanticTree(t *testing.T) {
	type nested struct {
		Enabled bool `config:"enabled"`
	}
	type options struct {
		Active   *nested        `config:"active"`
		Disabled *nested        `config:"disabled"`
		Labels   map[string]int `config:"labels,omitempty"`
		Empty    map[string]int `config:"empty,omitempty"`
		Items    []nested       `config:"items"`
		Inline   nested         `config:",inline"`
		Ignored  string         `config:"-"`
	}
	input := &options{
		Active: &nested{Enabled: true},
		Labels: map[string]int{"one": 1},
		Items:  []nested{{Enabled: true}},
	}
	inputValue := reflect.ValueOf(input).Elem()
	optionsType := reflect.TypeFor[options]()
	nestedType := reflect.TypeFor[nested]()
	enabledField := nestedType.Field(0)

	want := Node{
		Name:  "options",
		Kind:  ObjectNode,
		Type:  reflect.TypeFor[*options](),
		Value: inputValue,
		Fields: []Node{
			{
				Name:  "active",
				Kind:  ObjectNode,
				Type:  reflect.TypeFor[*nested](),
				Tag:   optionsType.Field(0).Tag,
				Value: inputValue.Field(0).Elem(),
				Path:  []reflect.StructField{optionsType.Field(0)},
				Fields: []Node{{
					Name:  "enabled",
					Kind:  ValueNode,
					Type:  reflect.TypeFor[bool](),
					Tag:   enabledField.Tag,
					Value: inputValue.Field(0).Elem().Field(0),
					Path:  []reflect.StructField{enabledField},
				}},
			},
			{
				Name:  "labels",
				Kind:  ObjectNode,
				Type:  reflect.TypeFor[map[string]int](),
				Tag:   optionsType.Field(2).Tag,
				Value: inputValue.Field(2),
				Path:  []reflect.StructField{optionsType.Field(2)},
				Element: &Node{
					Kind: ValueNode,
					Type: reflect.TypeFor[int](),
				},
				Fields: []Node{{
					Name:  "one",
					Kind:  ValueNode,
					Type:  reflect.TypeFor[int](),
					Value: inputValue.Field(2).MapIndex(reflect.ValueOf("one")),
				}},
			},
			{
				Name:  "items",
				Kind:  ArrayNode,
				Type:  reflect.TypeFor[[]nested](),
				Tag:   optionsType.Field(4).Tag,
				Value: inputValue.Field(4),
				Path:  []reflect.StructField{optionsType.Field(4)},
				Element: &Node{
					Kind: ObjectNode,
					Type: nestedType,
					Fields: []Node{{
						Name: "enabled",
						Kind: ValueNode,
						Type: reflect.TypeFor[bool](),
						Tag:  enabledField.Tag,
						Path: []reflect.StructField{enabledField},
					}},
				},
				Fields: []Node{{
					Name:  "0",
					Kind:  ObjectNode,
					Type:  nestedType,
					Value: inputValue.Field(4).Index(0),
					Fields: []Node{{
						Name:  "enabled",
						Kind:  ValueNode,
						Type:  reflect.TypeFor[bool](),
						Tag:   enabledField.Tag,
						Value: inputValue.Field(4).Index(0).Field(0),
						Path:  []reflect.StructField{enabledField},
					}},
				}},
			},
			{
				Name:  "enabled",
				Kind:  ValueNode,
				Type:  reflect.TypeFor[bool](),
				Tag:   enabledField.Tag,
				Value: inputValue.Field(5).Field(0),
				Path:  []reflect.StructField{optionsType.Field(5), enabledField},
			},
		},
	}
	got := ParseStruct(input, Options{TagNames: []string{"config", "json"}})
	if !nodesEqual(got, want) {
		t.Fatalf("ParseStruct() tree:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseStructUsesDefaultTags(t *testing.T) {
	type options struct {
		Value string `json:"value" yaml:"yamlValue"`
	}
	node := ParseStruct(options{}, Options{})
	if len(node.Fields) != 1 || node.Fields[0].Name != "value" {
		t.Fatalf("fields = %#v", node.Fields)
	}
}

func TestParseStructUsesOnlyHighestPriorityPresentTag(t *testing.T) {
	type options struct {
		Named string `json:"jsonName,omitempty" config:"configName"`
		Token string `json:"token,omitempty" config:",sensitive"`
	}
	node := ParseStruct(options{}, Options{TagNames: []string{"config", "json"}})
	if len(node.Fields) != 2 || node.Fields[0].Name != "configName" || node.Fields[1].Name != "Token" {
		t.Fatalf("fields = %#v", node.Fields)
	}
}

func TestParseStructCanIgnoreOmitEmpty(t *testing.T) {
	type options struct {
		String   string            `json:"string,omitempty"`
		Boolean  bool              `json:"boolean,omitempty"`
		Slice    []string          `json:"slice,omitempty"`
		Map      map[string]string `json:"map,omitempty"`
		Explicit string            `config:"explicit,omitempty"`
		Pointer  *string           `json:"pointer,omitempty"`
	}

	node := ParseStruct(options{}, Options{
		TagNames:        []string{"config", "json"},
		IgnoreOmitEmpty: true,
	})
	want := []string{"string", "boolean", "slice", "map", "explicit"}
	got := make([]string, len(node.Fields))
	for index := range node.Fields {
		got[index] = node.Fields[index].Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func TestParseStructUsesRuntimeInterfaceValues(t *testing.T) {
	input := map[string]any{
		"object": map[string]any{"enabled": true},
		"array":  []any{"one", 2},
		"null":   nil,
	}
	node := ParseStruct(input, Options{})
	object := node.Fields[2]
	if object.Name != "object" || object.Kind != ObjectNode || len(object.Fields) != 1 || object.Fields[0].Kind != ValueNode {
		t.Fatalf("object = %#v", object)
	}
	array := node.Fields[0]
	if array.Name != "array" || array.Kind != ArrayNode || len(array.Fields) != 2 || array.Fields[0].Kind != ValueNode || array.Fields[1].Kind != ValueNode {
		t.Fatalf("array = %#v", array)
	}
	null := node.Fields[1]
	if null.Name != "null" || null.Kind != NullNode {
		t.Fatalf("null = %#v", null)
	}
	if root := ParseStruct(nil, Options{}); root.Kind != NullNode {
		t.Fatalf("nil root = %#v", root)
	}
}
