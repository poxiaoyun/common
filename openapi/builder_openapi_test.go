package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/openapi"
	"xiaoshiai.cn/common/rest/api"
)

const (
	mediaTypeJSON      = "application/json"
	mediaTypeMultipart = "multipart/form-data"
)

func TestAddOpenAPIOperationBuildsNativeOAS31(t *testing.T) {
	type CreateRequest struct {
		Name string `json:"name"`
	}
	type CreateResponse struct {
		ID string `json:"id"`
	}

	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/widgets/{id}").
		Summary("Create widget").
		Desc("Creates one widget").
		Tag("Widgets", "Write").
		ContentType(mediaTypeJSON).
		Accept(mediaTypeJSON).
		Param(
			api.PathParam("id", "Widget ID"),
			api.QueryParam("mode", "Creation mode").
				In("safe", "fast").
				Optional(),
			api.BodyParam("body", CreateRequest{}),
		).
		RequestExample(CreateRequest{Name: "example"}).
		ResponseExample(CreateResponse{ID: "widget-1"}).
		ResponseStatus(http.StatusCreated, CreateResponse{}, "Created").
		Property("x-internal", true)

	require.NoError(t, openapi.AddOpenAPIOperation(document, route, builder))
	operation := document.Paths.Value("/widgets/{id}").Post
	require.NotNil(t, operation)
	assert.Equal(t, "post_widgets_by_id", operation.OperationID)
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
	assert.Equal(t, openapi.ComponentsSchemasRoot+"openapi_test.CreateRequest", requestMedia.Schema.Ref)
	assert.Equal(t, map[string]any{"name": "example"}, requestMedia.Example)

	response := operation.Responses.Status(http.StatusCreated)
	require.NotNil(t, response)
	responseMedia := response.Value.Content[mediaTypeJSON]
	require.NotNil(t, responseMedia)
	assert.Equal(t, openapi.ComponentsSchemasRoot+"openapi_test.CreateResponse", responseMedia.Schema.Ref)
	assert.Equal(t, map[string]any{"id": "widget-1"}, responseMedia.Example)

	require.NoError(t, document.Validate(
		context.Background(),
		openapi3.IsOpenAPI31OrLater(),
	))
}

func TestOperationIDDefaultsToCanonicalMethodAndMountedPath(t *testing.T) {
	tests := []struct {
		name  string
		route api.Route
		want  string
	}{
		{
			name: "path parameter",
			route: api.GET("/v1/clusters/{cluster}/instances").
				Summary("List instances"),
			want: "get_v1_clusters_by_cluster_instances",
		},
		{
			name: "nested path parameters",
			route: api.GET("/v1/clusters/{cluster}/namespaces/{namespace}/instances").
				Summary("List instances"),
			want: "get_v1_clusters_by_cluster_namespaces_by_namespace_instances",
		},
		{
			name:  "Google API action suffix",
			route: api.PUT("/v1/clusters/{cluster}/instances/{instance}:start"),
			want:  "put_v1_clusters_by_cluster_instances_by_instance_start",
		},
		{
			name:  "wildcard parameter",
			route: api.GET("/v1/services/{name}/proxy/{subresource}*"),
			want:  "get_v1_services_by_name_proxy_by_subresource_wildcard",
		},
		{
			name:  "any route is documented as get",
			route: api.Any("/fallback"),
			want:  "get_fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := newTestDocument()
			builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
			require.NoError(t, openapi.AddOpenAPIOperation(document, tt.route, builder))
			method := tt.route.Method
			if method == "" {
				method = http.MethodGet
			}
			assert.Equal(t, tt.want, document.Paths.Value(tt.route.Path).
				GetOperation(method).OperationID)
		})
	}
}

func TestSummaryDoesNotOverrideGeneratedOperationID(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.GET("/widgets").
		Summary("list widgets")

	require.NoError(t, openapi.AddOpenAPIOperation(document, route, builder))
	operation := document.Paths.Value("/widgets").Get
	require.NotNil(t, operation)
	assert.Equal(t, "list widgets", operation.Summary)
	assert.Empty(t, operation.Description)
	assert.Equal(t, "get_widgets", operation.OperationID)
}

func TestAddOpenAPIOperationBuildsFormRequestBody(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/uploads").
		Param(
			api.FormParam("file", "File to upload").
				Type("file"),
			api.FormParam("label", "Display label").
				Optional(),
		)

	require.NoError(t, openapi.AddOpenAPIOperation(document, route, builder))
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
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.GET("/widgets/{segments}").
		Param(
			api.PathParam("segments", "Path segments").
				Multiple(),
			api.QueryParam("labels", "Labels").
				Pattern("^[a-z]+$").
				Multiple(),
			api.HeaderParam("X-Flags", "Flags").
				Multiple().
				Optional(),
		)

	require.NoError(t, openapi.AddOpenAPIOperation(document, route, builder))
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
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/invalid").
		Param(
			api.BodyParam("body", struct{}{}),
			api.FormParam("field", "field"),
		)

	err := openapi.AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "body and form parameters")
}

func TestAddOpenAPIOperationRejectsMultipleBodyParameters(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/invalid").
		Param(
			api.BodyParam("first", struct{}{}),
			api.BodyParam("second", struct{}{}),
		)

	err := openapi.AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple body parameters")
}

func TestAddOpenAPIOperationUsesDefaultResponse(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)

	require.NoError(t, openapi.AddOpenAPIOperation(document, api.GET("/health"), builder))
	response := document.Paths.Value("/health").Get.Responses.Status(http.StatusOK)
	require.NotNil(t, response)
	require.NotNil(t, response.Value.Description)
	assert.Equal(t, "OK", *response.Value.Description)
}

func TestAddOpenAPIOperationRejectsInvalidResponseStatus(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.GET("/invalid").
		ResponseStatus(0, nil)

	err := openapi.AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "response status code 0")
}

func TestAddOpenAPIOperationRejectsNonJSONExample(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/invalid").
		Param(api.BodyParam("body", struct{}{})).
		RequestExample(make(chan struct{}))

	err := openapi.AddOpenAPIOperation(document, route, builder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request example is not valid JSON")
}

func TestAddOpenAPIOperationTreatsAnyRouteAsGet(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)

	require.NoError(t, openapi.AddOpenAPIOperation(document, api.Any("/fallback"), builder))
	operation := document.Paths.Value("/fallback").Get
	require.NotNil(t, operation)
	assert.Equal(t, "get_fallback", operation.OperationID)
}

func TestAddOpenAPIOperationMergesMediaVariants(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	jsonRoute := api.POST("/widgets").
		Summary("Create widget").
		ContentType("application/json").
		Accept("application/json").
		Param(api.BodyParam("body", map[string]string{})).
		ResponseStatus(http.StatusCreated, map[string]string{})
	xmlRoute := api.POST("/widgets").
		Summary("Create widget").
		ContentType("application/xml").
		Accept("application/xml").
		Param(api.BodyParam("body", "")).
		ResponseStatus(http.StatusCreated, "").
		ResponseStatus(http.StatusBadRequest, "")
	require.NoError(t, openapi.AddOpenAPIOperation(document, jsonRoute, builder))
	require.NoError(t, openapi.AddOpenAPIOperation(document, xmlRoute, builder))

	operation := document.Paths.Value("/widgets").Post
	assert.Equal(t, "post_widgets", operation.OperationID)
	assert.Equal(t, "Create widget", operation.Summary)
	require.Len(t, operation.RequestBody.Value.Content, 2)
	assert.True(t, operation.RequestBody.Value.Content["application/json"].Schema.Value.Type.Is(openapi3.TypeObject))
	assert.True(t, operation.RequestBody.Value.Content["application/xml"].Schema.Value.Type.Is(openapi3.TypeString))
	require.Len(t, operation.Responses.Status(http.StatusCreated).Value.Content, 2)
	require.Contains(t, operation.Responses.Status(http.StatusBadRequest).Value.Content, "application/xml")
	require.NoError(t, document.Validate(context.Background(), openapi3.IsOpenAPI31OrLater()))
}

func TestAddOpenAPIOperationRejectsConflictingMediaVariants(t *testing.T) {
	base := func() api.Route {
		return api.POST("/widgets").
			ContentType("application/json").
			Accept("application/json").
			Param(api.BodyParam("body", "")).
			Response("")
	}
	tests := []struct {
		name   string
		modify func(api.Route) api.Route
		want   string
	}{
		{
			name: "summary",
			modify: func(route api.Route) api.Route {
				return route.Summary("Different operation")
			},
			want: "operation metadata",
		},
		{
			name: "parameters",
			modify: func(route api.Route) api.Route {
				return route.Param(api.QueryParam("limit", "Page size"))
			},
			want: "operation metadata",
		},
		{
			name: "request required",
			modify: func(route api.Route) api.Route {
				route.Params = []api.Param{api.BodyParam("body", "").
					Optional()}
				return route
			},
			want: "request body metadata",
		},
		{
			name: "request schema",
			modify: func(route api.Route) api.Route {
				route.Params = []api.Param{api.BodyParam("body", 0)}
				return route
			},
			want: "request body: conflicting media type \"application/json\"",
		},
		{
			name: "request example",
			modify: func(route api.Route) api.Route {
				return route.RequestExample("example")
			},
			want: "request body: conflicting media type \"application/json\"",
		},
		{
			name: "response schema after adding request media",
			modify: func(route api.Route) api.Route {
				route.ContentTypes = []string{"application/xml"}
				route.Responses = []api.ResponseInfo{{Code: http.StatusOK, Body: 0}}
				return route
			},
			want: "response 200: conflicting media type \"application/json\"",
		},
		{
			name: "response example",
			modify: func(route api.Route) api.Route {
				return route.ResponseExample("example")
			},
			want: "response 200: conflicting media type \"application/json\"",
		},
		{
			name: "response description",
			modify: func(route api.Route) api.Route {
				route.Responses = []api.ResponseInfo{{Code: http.StatusOK, Body: "", Description: "Different response"}}
				return route
			},
			want: "response 200: conflicting metadata",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := newTestDocument()
			builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
			require.NoError(t, openapi.AddOpenAPIOperation(document, base(), builder))
			before, err := json.Marshal(document.Paths.Value("/widgets").Post)
			require.NoError(t, err)

			err = openapi.AddOpenAPIOperation(document, test.modify(base()), builder)
			require.ErrorContains(t, err, "merge POST /widgets:")
			require.ErrorContains(t, err, test.want)
			after, err := json.Marshal(document.Paths.Value("/widgets").Post)
			require.NoError(t, err)
			assert.Equal(t, before, after, "a failed merge must preserve the previous operation")
		})
	}
}

func TestAddOpenAPIOperationUsesWireEqualityForSharedMetadata(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	route := api.POST("/widgets").
		Param(api.BodyParam("body", "")).
		Response("")
	require.NoError(t, openapi.AddOpenAPIOperation(document, route, builder))
	operation := document.Paths.Value("/widgets").Post
	operation.Parameters = openapi3.Parameters{}
	operation.Extensions = map[string]any{}
	operation.Responses.Extensions = map[string]any{}
	operation.Responses.Status(http.StatusOK).Value.Headers = openapi3.Headers{}

	variant := route.
		ContentType("application/json").
		Accept("application/json").
		Tag("Default")
	variant.Responses = []api.ResponseInfo{{Code: http.StatusOK, Body: "", Description: "OK"}}
	require.NoError(t, openapi.AddOpenAPIOperation(document, variant, builder))
	require.Len(t, document.Paths.Value("/widgets").Post.RequestBody.Value.Content, 1)
	require.NoError(t, document.Validate(context.Background(), openapi3.IsOpenAPI31OrLater()))
}

func TestAddOpenAPIOperationNormalizesMediaKeysBeforeMerging(t *testing.T) {
	document := newTestDocument()
	builder := openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, document.Components.Schemas)
	first := api.GET("/widgets").
		Accept("Application/JSON; Charset=UTF-8").
		Response("")
	second := api.GET("/widgets").
		Accept("application/json;charset=utf-8").
		Response(0)
	require.NoError(t, openapi.AddOpenAPIOperation(document, first, builder))
	err := openapi.AddOpenAPIOperation(document, second, builder)
	require.ErrorContains(t, err, "conflicting media type \"application/json; charset=utf-8\"")
}

func TestMediaVariantRoutesBuildOpenAPIDocumentation(t *testing.T) {
	plugin := openapi.NewAPIDocPlugin()
	m := api.New().
		Plugin(plugin)
	for _, mediaType := range []string{"application/json", "application/xml"} {
		m.Route(api.POST("/widgets").
			ContentType(mediaType).
			Accept(mediaType).
			Param(api.BodyParam("body", "")).
			Response(""))
	}
	require.NotPanics(t, func() { m.Build() })
	operation := plugin.OpenAPI.Paths.Value("/widgets").Post
	require.Len(t, operation.RequestBody.Value.Content, 2)
	require.Len(t, operation.Responses.Status(http.StatusOK).Value.Content, 2)
}

func newTestDocument() *openapi.Document {
	return &openapi.Document{
		OpenAPI:    "3.1.1",
		Info:       &openapi3.Info{Title: "Test", Version: "1.0.0"},
		Components: &openapi3.Components{Schemas: openapi3.Schemas{}},
		Paths:      openapi3.NewPaths(),
	}
}
