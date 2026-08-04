package openapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestAddOpenAPIOperationBuildsNativeOAS31(t *testing.T) {
	type CreateRequest struct {
		Name string `json:"name"`
	}
	type CreateResponse struct {
		ID string `json:"id"`
	}

	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/widgets/{id}").
		Operation("create widget").
		Doc("Create widget").
		Desc("Creates one widget").
		Tag("Widgets", "Write").
		Consume(mediaTypeJSON).
		Produce(mediaTypeJSON).
		Param(
			api.PathParam("id", "Widget ID"),
			api.QueryParam("mode", "Creation mode").In("safe", "fast").Optional(),
			api.BodyParam("body", CreateRequest{}),
		).
		RequestExample(CreateRequest{Name: "example"}).
		ResponseExample(CreateResponse{ID: "widget-1"}).
		ResponseStatus(http.StatusCreated, CreateResponse{}, "Created").
		Property("x-internal", true)

	require.NoError(t, AddOpenAPIOperation(document, route, builder))
	operation := document.Paths.Value("/widgets/{id}").Post
	require.NotNil(t, operation)
	assert.Equal(t, "create_widget", operation.OperationID)
	assert.Equal(t, []string{"Widgets", "Write"}, operation.Tags)
	assert.Equal(t, true, operation.Extensions["x-internal"])

	require.Len(t, operation.Parameters, 2)
	assert.Equal(t, openapi3.ParameterInPath, operation.Parameters[0].Value.In)
	assert.True(t, operation.Parameters[0].Value.Required)
	assert.Equal(t, []any{"safe", "fast"}, operation.Parameters[1].Value.Schema.Value.Enum)

	require.NotNil(t, operation.RequestBody)
	require.NotNil(t, operation.RequestBody.Value)
	assert.True(t, operation.RequestBody.Value.Required)
	requestMedia := operation.RequestBody.Value.Content[mediaTypeJSON]
	require.NotNil(t, requestMedia)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.CreateRequest", requestMedia.Schema.Ref)
	assert.Equal(t, map[string]any{"name": "example"}, requestMedia.Example)

	response := operation.Responses.Status(http.StatusCreated)
	require.NotNil(t, response)
	responseMedia := response.Value.Content[mediaTypeJSON]
	require.NotNil(t, responseMedia)
	assert.Equal(t, ComponentsSchemasRoot+"openapi.CreateResponse", responseMedia.Schema.Ref)
	assert.Equal(t, map[string]any{"id": "widget-1"}, responseMedia.Example)

	require.NoError(t, document.Validate(
		context.Background(),
		openapi3.IsOpenAPI31OrLater(),
	))
}

func TestAddOpenAPIOperationBuildsFormRequestBody(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/uploads").Param(
		api.FormParam("file", "File to upload").Type("file"),
		api.FormParam("label", "Display label").Optional(),
	)

	require.NoError(t, AddOpenAPIOperation(document, route, builder))
	requestBody := document.Paths.Value("/uploads").Post.RequestBody.Value
	require.NotNil(t, requestBody)
	media := requestBody.Content[mediaTypeMultipart]
	require.NotNil(t, media)
	require.NotNil(t, media.Schema.Value)
	assert.Equal(t, []string{"file"}, media.Schema.Value.Required)
	assert.Equal(t, "binary", media.Schema.Value.Properties["file"].Value.Format)
	assert.True(t, media.Schema.Value.Properties["label"].Value.Type.Is(openapi3.TypeString))
	require.NoError(t, document.Validate(context.Background(), openapi3.IsOpenAPI31OrLater()))
}

func TestAddOpenAPIOperationBuildsArrayParameters(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.GET("/widgets/{segments}").Param(
		api.PathParam("segments", "Path segments").Multiple(),
		api.QueryParam("labels", "Labels").Pattern("^[a-z]+$").Multiple(),
		api.HeaderParam("X-Flags", "Flags").Multiple().Optional(),
	)

	require.NoError(t, AddOpenAPIOperation(document, route, builder))
	parameters := document.Paths.Value("/widgets/{segments}").Get.Parameters
	require.Len(t, parameters, 3)

	assert.Equal(t, openapi3.SerializationSimple, parameters[0].Value.Style)
	assert.True(t, parameters[0].Value.Schema.Value.Type.Is(openapi3.TypeArray))
	assert.Equal(t, openapi3.SerializationForm, parameters[1].Value.Style)
	assert.True(t, *parameters[1].Value.Explode)
	assert.Equal(t, "^[a-z]+$", parameters[1].Value.Schema.Value.Items.Value.Pattern)
	assert.Equal(t, openapi3.SerializationSimple, parameters[2].Value.Style)
	require.NoError(t, document.Validate(context.Background(), openapi3.IsOpenAPI31OrLater()))
}

func TestAddOpenAPIOperationRejectsAmbiguousRequestBody(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/invalid").Param(
		api.BodyParam("body", struct{}{}),
		api.FormParam("field", "field"),
	)

	err := AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "body and form parameters")
}

func TestAddOpenAPIOperationRejectsMultipleBodyParameters(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/invalid").Param(
		api.BodyParam("first", struct{}{}),
		api.BodyParam("second", struct{}{}),
	)

	err := AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple body parameters")
}

func TestAddOpenAPIOperationUsesDefaultResponse(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)

	require.NoError(t, AddOpenAPIOperation(document, api.GET("/health"), builder))
	response := document.Paths.Value("/health").Get.Responses.Status(http.StatusOK)
	require.NotNil(t, response)
	require.NotNil(t, response.Value.Description)
	assert.Equal(t, "OK", *response.Value.Description)
}

func TestAddOpenAPIOperationRejectsInvalidResponseStatus(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.GET("/invalid").ResponseStatus(0, nil)

	err := AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "response status code 0")
}

func TestAddOpenAPIOperationRejectsNonJSONExample(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/invalid").
		Param(api.BodyParam("body", struct{}{})).
		RequestExample(make(chan struct{}))

	err := AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request example is not valid JSON")
}

func TestAddOpenAPIOperationTreatsAnyRouteAsGet(t *testing.T) {
	document := newTestDocument()
	builder := NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)

	require.NoError(t, AddOpenAPIOperation(document, api.Any("/fallback"), builder))
	operation := document.Paths.Value("/fallback").Get
	require.NotNil(t, operation)
	assert.Equal(t, "get_/fallback", operation.OperationID)
}

func newTestDocument() *openapi3.T {
	return &openapi3.T{
		OpenAPI:    "3.1.1",
		Info:       &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Components: &openapi3.Components{Schemas: openapi3.Schemas{}},
		Paths:      openapi3.NewPaths(),
	}
}
