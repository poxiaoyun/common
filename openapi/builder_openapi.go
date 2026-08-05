package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"xiaoshiai.cn/common/rest/api"
)

const (
	mediaTypeJSON      = "application/json"
	mediaTypeForm      = "application/x-www-form-urlencoded"
	mediaTypeMultipart = "multipart/form-data"
)

// AddOpenAPIOperation projects one route directly into an OpenAPI 3.1 document.
func AddOpenAPIOperation(document *openapi3.T, route api.Route, builder *Builder) error {
	if route.NotDoc {
		return nil
	}
	if document == nil {
		return fmt.Errorf("openapi document is nil")
	}
	if document.Paths == nil {
		document.Paths = openapi3.NewPaths()
	}
	method := route.Method
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
	default:
		return fmt.Errorf("unsupported HTTP method %q", method)
	}

	pathItem := document.Paths.Value(route.Path)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
	}
	if pathItem.GetOperation(method) != nil {
		return fmt.Errorf("%s %s is already documented", method, route.Path)
	}

	operation, err := buildRouteOperation(route, builder)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, route.Path, err)
	}
	pathItem.SetOperation(method, operation)
	document.Paths.Set(route.Path, pathItem)
	return nil
}

func buildRouteOperation(route api.Route, builder *Builder) (*openapi3.Operation, error) {
	if builder == nil {
		return nil, fmt.Errorf("schema builder is nil")
	}
	requestExample, err := normalizeExample(route.RequestSample)
	if err != nil {
		return nil, fmt.Errorf("request example is not valid JSON: %w", err)
	}
	responseExample, err := normalizeExample(route.ResponseSample)
	if err != nil {
		return nil, fmt.Errorf("response example is not valid JSON: %w", err)
	}
	tags := slices.Clone(route.Tags)
	if len(tags) == 0 {
		tags = []string{"Default"}
	}
	operation := &openapi3.Operation{
		Tags:        tags,
		Summary:     route.SummaryText,
		Description: route.Description,
		OperationID: operationID(route),
		Deprecated:  route.IsDeprecated,
		Responses:   openapi3.NewResponses(),
	}
	for key, value := range route.Properties {
		if strings.HasPrefix(strings.ToLower(key), "x-") {
			if operation.Extensions == nil {
				operation.Extensions = map[string]any{}
			}
			operation.Extensions[key] = value
		}
	}

	var bodyParams, formParams []api.Param
	for _, param := range route.Params {
		switch param.Kind {
		case api.ParamKindBody:
			bodyParams = append(bodyParams, param)
		case api.ParamKindForm:
			formParams = append(formParams, param)
		case api.ParamKindPath, api.ParamKindQuery, api.ParamKindHeader:
			operation.Parameters = append(operation.Parameters, &openapi3.ParameterRef{Value: buildParameter(param, builder)})
		default:
			return nil, fmt.Errorf("parameter %q has unsupported location %q", param.Name, param.Kind)
		}
	}
	if len(bodyParams) > 0 && len(formParams) > 0 {
		return nil, fmt.Errorf("body and form parameters cannot be combined in one OpenAPI request body")
	}
	if len(bodyParams) > 1 {
		return nil, fmt.Errorf("multiple body parameters are not supported")
	}
	if len(bodyParams) == 1 {
		param := bodyParams[0]
		schema := buildParameterSchema(param, builder)
		operation.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{
			Description: param.Description,
			Required:    !param.IsOptional,
			Content:     buildContent(route.Consumes, mediaTypeJSON, schema, requestExample),
		}}
	} else if len(formParams) > 0 {
		schema := openapi3.NewObjectSchema()
		for _, param := range formParams {
			schema.Properties[param.Name] = buildParameterSchema(param, builder)
			if !param.IsOptional {
				schema.Required = appendUnique(schema.Required, param.Name)
			}
		}
		fallback := mediaTypeForm
		if slices.Contains(route.Consumes, mediaTypeMultipart) || hasFileParameter(formParams) {
			fallback = mediaTypeMultipart
		}
		operation.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{
			Required: len(schema.Required) > 0,
			Content:  buildContent(route.Consumes, fallback, schemaValue(schema), requestExample),
		}}
	}

	for _, responseInfo := range route.Responses {
		if responseInfo.Code < 100 || responseInfo.Code > 599 {
			return nil, fmt.Errorf("response status code %d is outside the OpenAPI HTTP status range", responseInfo.Code)
		}
		response := openapi3.NewResponse().WithDescription(responseDescription(responseInfo.Code, responseInfo.Description))
		if responseInfo.Body != nil {
			response.Content = buildContent(route.Produces, mediaTypeJSON, builder.Build(responseInfo.Body), responseExample)
		}
		if len(responseInfo.Headers) > 0 {
			response.Headers = openapi3.Headers{}
			for name, description := range responseInfo.Headers {
				response.Headers[name] = &openapi3.HeaderRef{Value: &openapi3.Header{Parameter: openapi3.Parameter{
					Description: description,
					Schema:      schemaValue(openapi3.NewStringSchema()),
				}}}
			}
		}
		operation.Responses.Set(strconv.Itoa(responseInfo.Code), &openapi3.ResponseRef{Value: response})
	}
	if operation.Responses.Len() == 0 {
		operation.Responses.Set(strconv.Itoa(http.StatusOK), &openapi3.ResponseRef{
			Value: openapi3.NewResponse().WithDescription(http.StatusText(http.StatusOK)),
		})
	}
	return operation, nil
}

func buildParameter(param api.Param, builder *Builder) *openapi3.Parameter {
	parameter := &openapi3.Parameter{
		Name:        param.Name,
		In:          string(param.Kind),
		Description: param.Description,
		Required:    param.Kind == api.ParamKindPath || !param.IsOptional,
		Schema:      buildParameterSchema(param, builder),
	}
	if param.AllowMultiple {
		switch param.Kind {
		case api.ParamKindQuery:
			explode := true
			parameter.Style = openapi3.SerializationForm
			parameter.Explode = &explode
		case api.ParamKindPath, api.ParamKindHeader:
			parameter.Style = openapi3.SerializationSimple
		}
	}
	return parameter
}

func buildParameterSchema(param api.Param, builder *Builder) *openapi3.SchemaRef {
	var ref *openapi3.SchemaRef
	if param.Example != nil {
		ref = builder.Build(param.Example)
	}
	if ref == nil {
		ref = schemaFromDataType(param.DataType, param.DataFormat)
	}
	needsOverlay := param.DataFormat != "" || len(param.Enum) > 0 || param.Default != nil || param.PatternExpr != ""
	if ref.Ref != "" && needsOverlay {
		ref = schemaValue(&openapi3.Schema{AllOf: openapi3.SchemaRefs{ref}})
	}
	if ref.Value == nil {
		ref.Value = openapi3.NewSchema()
	}
	schema := ref.Value
	if param.DataFormat != "" {
		schema.Format = param.DataFormat
	}
	if len(param.Enum) > 0 {
		schema.Enum = slices.Clone(param.Enum)
	}
	if param.Default != nil {
		schema.Default = param.Default
	}
	if param.PatternExpr != "" {
		schema.Pattern = param.PatternExpr
	}
	if param.AllowMultiple && (schema.Type == nil || !schema.Type.Is(openapi3.TypeArray)) {
		ref = schemaValue(&openapi3.Schema{
			Type:  schemaTypes(openapi3.TypeArray),
			Items: ref,
		})
	}
	return ref
}

func schemaFromDataType(dataType, format string) *openapi3.SchemaRef {
	switch dataType {
	case "array":
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeArray), Items: AnyProperty()})
	case "boolean":
		return schemaValue(openapi3.NewBoolSchema())
	case "integer":
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeInteger), Format: format})
	case "number":
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeNumber), Format: format})
	case "object":
		return ObjectProperty()
	case "file":
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeString), Format: "binary"})
	case "string", "":
		return schemaValue(&openapi3.Schema{Type: schemaTypes(openapi3.TypeString), Format: format})
	default:
		return schemaValue(&openapi3.Schema{Type: schemaTypes(dataType), Format: format})
	}
}

func buildContent(mediaTypes []string, fallback string, schema *openapi3.SchemaRef, example any) openapi3.Content {
	if len(mediaTypes) == 0 {
		mediaTypes = []string{fallback}
	}
	content := openapi3.Content{}
	for _, mediaType := range mediaTypes {
		if mediaType == "" {
			continue
		}
		content[mediaType] = &openapi3.MediaType{Schema: schema, Example: example}
	}
	return content
}

func normalizeExample(example any) (any, error) {
	if example == nil {
		return nil, nil
	}
	data, err := json.Marshal(example)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func hasFileParameter(params []api.Param) bool {
	return slices.ContainsFunc(params, func(param api.Param) bool {
		return param.DataType == "file"
	})
}

func responseDescription(status int, description string) string {
	if description != "" {
		return description
	}
	if text := http.StatusText(status); text != "" {
		return text
	}
	return "Response"
}

func operationID(route api.Route) string {
	method := route.Method
	if method == "" {
		method = http.MethodGet
	}
	method = canonicalIdentifier(method)
	pathID := canonicalPathIdentifier(route.Path)
	if pathID == "" {
		return method
	}
	return method + "_" + pathID
}

var pathParameterPattern = regexp.MustCompile(`\{([^{}]+)\}(\*)?`)

// canonicalPathIdentifier preserves the useful parts of an HTTP path while
// producing an identifier-safe value. Path parameters become "by_<name>" and
// Google API custom-method suffixes (":ACTION") become a final action segment;
// for example, PUT /instances/{instance}:start becomes
// put_instances_by_instance_start.
func canonicalPathIdentifier(routePath string) string {
	rewritten := pathParameterPattern.ReplaceAllStringFunc(routePath, func(match string) string {
		parts := pathParameterPattern.FindStringSubmatch(match)
		identifier := "_by_" + parts[1]
		if parts[2] == "*" {
			identifier += "_wildcard"
		}
		return identifier + "_"
	})
	rewritten = strings.ReplaceAll(rewritten, "*", "_wildcard_")
	return canonicalIdentifier(rewritten)
}

func canonicalIdentifier(value string) string {
	var identifier strings.Builder
	separator := false
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			if separator && identifier.Len() > 0 {
				identifier.WriteByte('_')
			}
			identifier.WriteRune(char + ('a' - 'A'))
			separator = false
		case char >= 'a' && char <= 'z' || char >= '0' && char <= '9':
			if separator && identifier.Len() > 0 {
				identifier.WriteByte('_')
			}
			identifier.WriteRune(char)
			separator = false
		default:
			separator = true
		}
	}
	return identifier.String()
}
