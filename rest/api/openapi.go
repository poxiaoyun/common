package api

import (
	"bytes"
	"html/template"
	"net/http"
	"path"
	"reflect"
	"strings"

	"github.com/go-openapi/spec"
	"xiaoshiai.cn/common/rest/openapi"
)

var _ RoutePlugin = &OpenAPIPlugin{}

type OpenAPIPlugin struct {
	Basepath string
	Swagger  *spec.Swagger
	Builder  *openapi.Builder
}

// OnBuild implements RoutePlugin.
func (s *OpenAPIPlugin) OnBuild(m *API, routes []*Route) error {
	return nil
}

func NewAPIDocPlugin(basepath string, fn func(swagger *spec.Swagger)) *OpenAPIPlugin {
	if basepath == "" {
		basepath = "/docs"
	}
	swagger := &spec.Swagger{SwaggerProps: spec.SwaggerProps{
		Swagger:     "2.0",
		Definitions: map[string]spec.Schema{},
	}}
	if fn != nil {
		fn(swagger)
	}
	return &OpenAPIPlugin{
		Swagger:  swagger,
		Builder:  openapi.NewBuilder(openapi.InterfaceBuildOptionDefault, swagger.Definitions),
		Basepath: basepath,
	}
}

// Install implements Plugin.
func (s *OpenAPIPlugin) Install(m *API) error {
	specpath := path.Join(s.Basepath, "/openapi.json")
	m.Route(GET(specpath).Doc("swagger api doc").To(func(w http.ResponseWriter, r *http.Request) {
		Raw(w, http.StatusOK, s.Swagger, nil)
	}))
	// UI
	swaggerui, redocui, stoplight := NewSwaggerUI(specpath), NewRedocUI(specpath), NewStoplightElementsUI(specpath)
	m.Route(GET(s.Basepath).
		Doc("swagger api html").
		Param(QueryParam("provider", "UI provider").In("swagger", "redoc")).
		To(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Query().Get("provider") {
			case "swagger", "":
				RenderHTML(w, swaggerui)
			case "redoc":
				RenderHTML(w, redocui)
			case "stoplight":
				RenderHTML(w, stoplight)
			}
		}),
	)
	return nil
}

// OnRoute implements Plugin.
func (s *OpenAPIPlugin) OnRoute(route *Route) error {
	AddSwaggerOperation(s.Swagger, *route, s.Builder)
	return nil
}

func AddSwaggerOperation(swagger *spec.Swagger, route Route, builder *openapi.Builder) {
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

func buildRouteOperation(route Route, builder *openapi.Builder) *spec.Operation {
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
							Required:    param.Kind == ParamKindPath || param.Kind == ParamKindBody || !param.IsOptional,
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

func operationID(route Route) string {
	if route.OperationName != "" {
		return strings.ReplaceAll(route.OperationName, " ", "_")
	}
	if route.Summary != "" {
		return strings.ReplaceAll(route.Summary, " ", "_")
	}
	return strings.ToLower(route.Method) + "_" + route.Path
}

const (
	RedocTemplate = `<!DOCTYPE html>
	<html>
	  <head>
		<title>API Documentation</title>
			<!-- needed for adaptive design -->
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1">
			<link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
	
		<!--
		ReDoc doesn't change outer page styles
		-->
		<style>
		  body {
			margin: 0;
			padding: 0;
		  }
		</style>
	  </head>
	  <body>
		<redoc spec-url='{{ .SpecURL }}'></redoc>
		<script src="https://cdn.jsdelivr.net/npm/redoc/bundles/redoc.standalone.js"> </script>
	  </body>
	</html>
	`
	SwaggerTemplate = `
	<!DOCTYPE html>
	<html lang="en">
	  <head>
		<meta charset="UTF-8">
			<title>API Documentation</title>
	
		<link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist/swagger-ui.css" >
		<link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist/favicon-32x32.png" sizes="32x32" />
		<link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist/favicon-16x16.png" sizes="16x16" />
		<style>
		  html
		  {
			box-sizing: border-box;
			overflow: -moz-scrollbars-vertical;
			overflow-y: scroll;
		  }
		  *,
		  *:before,
		  *:after
		  {
			box-sizing: inherit;
		  }
		  body
		  {
			margin:0;
			background: #fafafa;
		  }
		</style>
	  </head>
	  <body>
		<div id="swagger-ui"></div>
		<script src="https://unpkg.com/swagger-ui-dist/swagger-ui-bundle.js"> </script>
		<script src="https://unpkg.com/swagger-ui-dist/swagger-ui-standalone-preset.js"> </script>
		<script>
		window.onload = function() {
		  // Begin Swagger UI call region
		  const ui = SwaggerUIBundle({
			url: '{{ .SpecURL }}',
			dom_id: '#swagger-ui',
			deepLinking: true,
			presets: [
			  SwaggerUIBundle.presets.apis,
			  SwaggerUIStandalonePreset
			],
			plugins: [
			  SwaggerUIBundle.plugins.DownloadUrl
			],
			layout: "StandaloneLayout",
			oauth2RedirectUrl: '{{ .OAuthCallbackURL }}'
		  })
		  // End Swagger UI call region
	
		  window.ui = ui
		}
	  </script>
	  </body>
	</html>
	`
	StoplightElementsTemplate = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <title>Elements in HTML</title>
    <!-- Embed elements Elements via Web Component -->
    <script src="https://unpkg.com/@stoplight/elements/web-components.min.js"></script>
    <link rel="stylesheet" href="https://unpkg.com/@stoplight/elements/styles.min.css">
  </head>
  <body>

    <elements-api
      apiDescriptionUrl="{{ .SpecURL }}"
      router="hash"
      layout="sidebar"
    />

  </body>
</html>`
)

func NewSwaggerUI(specPath string) []byte {
	return render(specPath, SwaggerTemplate)
}

func NewRedocUI(specPath string) []byte {
	return render(specPath, RedocTemplate)
}

func NewStoplightElementsUI(specPath string) []byte {
	return render(specPath, StoplightElementsTemplate)
}

func render(specPath string, htmltemplate string) []byte {
	tmpl := template.Must(template.New("ui").Parse(htmltemplate))
	buf := bytes.NewBuffer(nil)
	_ = tmpl.Execute(buf, map[string]string{
		"SpecURL": specPath,
	})
	return buf.Bytes()
}
