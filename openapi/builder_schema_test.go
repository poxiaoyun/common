package openapi

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestBuilderBuildPrimitiveSchemas(t *testing.T) {
	tests := []struct {
		name            string
		value           any
		types           []string
		format          string
		contentEncoding string
	}{
		{name: "string", value: "", types: []string{openapi3.TypeString}},
		{name: "boolean", value: false, types: []string{openapi3.TypeBoolean}},
		{name: "integer", value: int64(0), types: []string{openapi3.TypeInteger}, format: "int64"},
		{name: "number", value: float64(0), types: []string{openapi3.TypeNumber}, format: "double"},
		{name: "json number", value: json.Number("0"), types: []string{openapi3.TypeNumber}, format: "double"},
		{name: "bytes", value: []byte(nil), types: []string{openapi3.TypeString}, format: "byte", contentEncoding: "base64"},
		{name: "time", value: time.Time{}, types: []string{openapi3.TypeString}, format: "date-time"},
		{name: "duration", value: time.Duration(0), types: []string{openapi3.TypeInteger}, format: "int64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := NewBuilder(InterfaceBuildOptionDefault, nil).Build(tt.value)
			require.NotNil(t, ref)
			require.NotNil(t, ref.Value)
			assert.Equal(t, tt.types, ref.Value.Type.Slice())
			assert.Equal(t, tt.format, ref.Value.Format)
			assert.Equal(t, tt.contentEncoding, ref.Value.ContentEncoding)
		})
	}
}

func TestBuilderBuildStructComponents(t *testing.T) {
	type Embedded struct {
		Kind string `json:"kind"`
	}
	type Model struct {
		Embedded
		Name    string `json:"name"`
		Comment string `json:"comment,omitempty"`
		Ignored string `json:"-"`
	}

	builder := NewBuilder(InterfaceBuildOptionDefault, nil)
	ref := builder.Build(Model{})

	require.Equal(t, ComponentsSchemasRoot+"openapi.Model", ref.Ref)
	model := builder.Schemas["openapi.Model"]
	require.NotNil(t, model)
	require.NotNil(t, model.Value)
	require.Len(t, model.Value.AllOf, 2)

	inline := model.Value.AllOf[0].Value
	require.NotNil(t, inline)
	assert.Contains(t, inline.Properties, "name")
	assert.Contains(t, inline.Properties, "comment")
	assert.NotContains(t, inline.Properties, "Ignored")
	assert.Equal(t, []string{"name"}, inline.Required)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Embedded", model.Value.AllOf[1].Ref)
}

func TestBuilderBuildRecursiveSchema(t *testing.T) {
	type Node struct {
		Name     string  `json:"name"`
		Children []*Node `json:"children,omitempty"`
	}

	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	ref := builder.Build(Node{})
	require.Equal(t, ComponentsSchemasRoot+"openapi.Node", ref.Ref)

	node := builder.Schemas["openapi.Node"].Value
	require.NotNil(t, node)
	children := node.Properties["children"]
	require.NotNil(t, children)
	require.NotNil(t, children.Value)
	require.NotNil(t, children.Value.Items)
	require.Len(t, children.Value.Items.Value.AnyOf, 2)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Node", children.Value.Items.Value.AnyOf[0].Ref)
	assert.True(t, children.Value.Items.Value.AnyOf[1].Value.Type.Is(openapi3.TypeNull))

	require.NoError(t, AddOpenAPIOperation(document, api.GET("/nodes").Response(Node{}), builder))
	require.NoError(t, document.Validate(context.Background(), openapi3.IsOpenAPI31OrLater()))
}

func TestBuilderBuildNullablePointers(t *testing.T) {
	type Child struct {
		Name string `json:"name"`
	}
	type Model struct {
		Count *int64 `json:"count"`
		Child *Child `json:"child,omitempty"`
	}

	builder := NewBuilder(InterfaceBuildOptionDefault, nil)
	builder.Build(Model{})
	model := builder.Schemas["openapi.Model"].Value

	assert.Equal(t, []string{openapi3.TypeInteger, openapi3.TypeNull}, model.Properties["count"].Value.Type.Slice())
	require.Len(t, model.Properties["child"].Value.AnyOf, 2)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Child", model.Properties["child"].Value.AnyOf[0].Ref)
	assert.True(t, model.Properties["child"].Value.AnyOf[1].Value.Type.Is(openapi3.TypeNull))
}

func TestBuilderBuildDynamicInterfaceOverlay(t *testing.T) {
	type Value struct {
		Value string `json:"value"`
	}
	type Envelope struct {
		Data any `json:"data" openapi:"dynamic"`
	}

	builder := NewBuilder(InterfaceBuildOptionOverride, nil)
	ref := builder.Build(Envelope{Data: Value{}})
	require.NotNil(t, ref.Value)
	require.Len(t, ref.Value.AllOf, 2)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Envelope", ref.Value.AllOf[0].Ref)
	overlay := ref.Value.AllOf[1].Value
	require.NotNil(t, overlay)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Value", overlay.Properties["data"].Ref)

	base := builder.Schemas["openapi.Envelope"].Value
	require.NotNil(t, base)
	assert.NotNil(t, base.Properties["data"].Value)
	assert.Nil(t, base.Properties["data"].Value.Type)
}

func TestBuilderSanitizesGenericComponentNames(t *testing.T) {
	type Page[T any] struct {
		Items []T `json:"items"`
	}

	builder := NewBuilder(InterfaceBuildOptionDefault, nil)
	ref := builder.Build(Page[string]{})
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Page_string_", ref.Ref)
	assert.Contains(t, builder.Schemas, "openapi.Page_string_")

	type 模型 struct {
		Name string `json:"name"`
	}
	unicodeRef := builder.Build(模型{})
	assert.Equal(t, ComponentsSchemasRoot+"openapi.__", unicodeRef.Ref)
	assert.Contains(t, builder.Schemas, "openapi.__")
}

func TestBuilderBuildMapAndHeterogeneousSlice(t *testing.T) {
	builder := NewBuilder(InterfaceBuildOptionOverride, nil)

	mapRef := builder.Build(map[string]int{"one": 1})
	require.NotNil(t, mapRef.Value)
	assert.True(t, mapRef.Value.Type.Is(openapi3.TypeObject))
	assert.NotNil(t, mapRef.Value.AdditionalProperties.Schema)
	assert.Contains(t, mapRef.Value.Properties, "one")

	sliceRef := builder.Build([]any{"value", int64(1)})
	require.NotNil(t, sliceRef.Value)
	require.NotNil(t, sliceRef.Value.Items)
	require.NotNil(t, sliceRef.Value.Items.Value)
	assert.Len(t, sliceRef.Value.Items.Value.AnyOf, 2)
}

func TestBuilderInterfaceModes(t *testing.T) {
	type Value struct {
		Data any
	}
	value := reflect.ValueOf(Value{Data: "sample"}).FieldByName("Data")
	emptyValue := reflect.ValueOf(Value{}).FieldByName("Data")

	tests := []struct {
		name   string
		option InterfaceBuildOption
		value  reflect.Value
		check  func(*testing.T, *openapi3.SchemaRef)
	}{
		{
			name:   "default concrete value",
			option: InterfaceBuildOptionDefault,
			check: func(t *testing.T, ref *openapi3.SchemaRef) {
				assert.True(t, ref.Value.Type.Is(openapi3.TypeString))
			},
		},
		{
			name:   "default object without sample",
			option: InterfaceBuildOptionDefault,
			value:  emptyValue,
			check: func(t *testing.T, ref *openapi3.SchemaRef) {
				assert.True(t, ref.Value.Type.Is(openapi3.TypeObject))
			},
		},
		{
			name:   "override concrete value",
			option: InterfaceBuildOptionOverride,
			check: func(t *testing.T, ref *openapi3.SchemaRef) {
				assert.True(t, ref.Value.Type.Is(openapi3.TypeString))
			},
		},
		{
			name:   "merge object and concrete value",
			option: InterfaceBuildOptionMerge,
			check: func(t *testing.T, ref *openapi3.SchemaRef) {
				require.Len(t, ref.Value.AnyOf, 2)
				assert.True(t, ref.Value.AnyOf[0].Value.Type.Is(openapi3.TypeObject))
				assert.True(t, ref.Value.AnyOf[1].Value.Type.Is(openapi3.TypeString))
			},
		},
		{
			name:   "ignore",
			option: InterfaceBuildOptionIgnore,
			check: func(t *testing.T, ref *openapi3.SchemaRef) {
				assert.Nil(t, ref)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldValue := tt.value
			if !fieldValue.IsValid() {
				fieldValue = value
			}
			tt.check(t, NewBuilder(tt.option, nil).BuildSchema(fieldValue))
		})
	}
}

func TestBuilderIgnoreDynamicInterfaceField(t *testing.T) {
	type Envelope struct {
		Name string `json:"name"`
		Data any    `json:"data"`
	}

	builder := NewBuilder(InterfaceBuildOptionIgnore, nil)
	ref := builder.Build(Envelope{Data: "ignored"})
	require.NotNil(t, ref)
	schema := builder.Schemas["openapi.Envelope"].Value
	assert.Contains(t, schema.Properties, "name")
	assert.NotContains(t, schema.Properties, "data")
}

func TestBuilderBuildEmptyCollectionsFromElementType(t *testing.T) {
	type Item struct {
		ID string `json:"id"`
	}

	builder := NewBuilder(InterfaceBuildOptionDefault, nil)
	slice := builder.Build([]Item{})
	require.NotNil(t, slice.Value.Items)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Item", slice.Value.Items.Ref)

	mapping := builder.Build(map[string]Item{})
	require.NotNil(t, mapping.Value.AdditionalProperties.Schema)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Item", mapping.Value.AdditionalProperties.Schema.Ref)
}

func TestBuilderBuildDynamicGenericOverlays(t *testing.T) {
	type Envelope[T any] struct {
		Items []T `json:"items" openapi:"dynamic"`
	}

	builder := NewBuilder(InterfaceBuildOptionOverride, nil)
	stringEnvelope := builder.Build(Envelope[string]{})
	integerEnvelope := builder.Build(Envelope[int64]{})

	assert.Contains(t, builder.Schemas, "openapi.Envelope")
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Envelope", stringEnvelope.Value.AllOf[0].Ref)
	assert.True(t, stringEnvelope.Value.AllOf[1].Value.Properties["items"].Value.Items.Value.Type.Is(openapi3.TypeString))
	assert.Equal(t, ComponentsSchemasRoot+"openapi.Envelope", integerEnvelope.Value.AllOf[0].Ref)
	assert.True(t, integerEnvelope.Value.AllOf[1].Value.Properties["items"].Value.Items.Value.Type.Is(openapi3.TypeInteger))
}
