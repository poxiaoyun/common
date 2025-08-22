package openapi

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/go-openapi/spec"
	"xiaoshiai.cn/common/rest/api"
)

func AddSwaggerOperation(swagger *spec.Swagger, route api.Route, builder *Builder) {
	if route.NotDoc {
		return
	}
	operation := buildRouteOperation(route, builder)
	if swagger.Paths == nil {
		swagger.Paths = &spec.Paths{}
	}
	if swagger.Paths.Paths == nil {
		swagger.Paths.Paths = map[string]spec.PathItem{}
	}
	pathItem := swagger.Paths.Paths[route.Path]
	switch route.Method {
	case http.MethodGet, "":
		pathItem.Get = operation
	case http.MethodPost:
		pathItem.Post = operation
	case http.MethodPut:
		pathItem.Put = operation
	case http.MethodDelete:
		pathItem.Delete = operation
	case http.MethodPatch:
		pathItem.Patch = operation
	case http.MethodHead:
		pathItem.Head = operation
	case http.MethodOptions:
		pathItem.Options = operation
	}
	swagger.Paths.Paths[route.Path] = pathItem
}

func buildRouteOperation(route api.Route, builder *Builder) *spec.Operation {
	summary := route.Summary
	if summary == "" {
		summary = route.OperationName
	}
	desc := route.Description
	if desc == "" {
		desc = summary
	}
	return &spec.Operation{
		OperationProps: spec.OperationProps{
			ID: operationID(route),
			Tags: func() []string {
				if len(route.Tags) > 0 {
					// only use the last tag
					return route.Tags[len(route.Tags)-1:]
				}
				return []string{"Default"} // default tag
			}(),
			Summary:     summary,
			Description: desc,
			Consumes:    route.Consumes,
			Produces:    route.Produces,
			Deprecated:  route.IsDeprecated,
			Parameters: func() []spec.Parameter {
				var parameters []spec.Parameter
				for _, param := range route.Params {
					parameters = append(parameters, spec.Parameter{
						ParamProps: spec.ParamProps{
							Name:        param.Name,
							Description: param.Description,
							In:          string(param.Kind),
							Schema:      builder.Build(param.Example),
							Required:    param.Kind == api.ParamKindPath || param.Kind == api.ParamKindBody || !param.IsOptional,
						},
						CommonValidations: spec.CommonValidations{
							Enum:    param.Enum,
							Pattern: param.PatternExpr,
						},
						SimpleSchema: spec.SimpleSchema{
							Type:    param.DataType,
							Default: param.Default,
							Format:  param.DataFormat,
						},
					})
				}
				return parameters
			}(),
			Responses: &spec.Responses{
				ResponsesProps: spec.ResponsesProps{
					StatusCodeResponses: func() map[int]spec.Response {
						responses := map[int]spec.Response{}
						for _, resp := range route.Responses {
							response := spec.Response{
								ResponseProps: spec.ResponseProps{
									Description: resp.Description,
									Schema:      builder.Build(resp.Body),
									Headers: func() map[string]spec.Header {
										headers := map[string]spec.Header{}
										for k, h := range resp.Headers {
											headers[k] = spec.Header{HeaderProps: spec.HeaderProps{Description: h}}
										}
										return headers
									}(),
								},
							}
							if resp.Body != nil && !reflect.ValueOf(resp.Body).IsZero() {
								response.Schema.Example = resp.Body
							}
							responses[resp.Code] = response
						}
						if len(responses) == 0 {
							responses[200] = spec.Response{ResponseProps: spec.ResponseProps{Description: "OK"}}
						}
						return responses
					}(),
				},
			},
		},
	}
}

func operationID(route api.Route) string {
	if route.OperationName != "" {
		return strings.ReplaceAll(route.OperationName, " ", "_")
	}
	if route.Summary != "" {
		return strings.ReplaceAll(route.Summary, " ", "_")
	}
	return strings.ToLower(route.Method) + "_" + route.Path
}
