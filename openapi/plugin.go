package openapi

import (
	"net/http"

	"github.com/go-openapi/spec"

	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/version"
)

var _ api.RoutePlugin = &OpenAPIPlugin{}

type OpenAPIPlugin struct {
	Basepath string
	Swagger  *spec.Swagger
	Builder  *Builder
	UI       OpenAPIUI
}

func NewAPIDocPlugin(basepath string, fn func(swagger *spec.Swagger)) *OpenAPIPlugin {
	if basepath == "" {
		basepath = "/docs"
	}
	swagger := &spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Swagger: "2.0",
			Info: &spec.Info{
				InfoProps: spec.InfoProps{
					Title:       "API Documentation",
					Version:     version.Get().String(),
					Description: "API documentation",
				},
			},
			Definitions: map[string]spec.Schema{},
		},
	}
	if fn != nil {
		fn(swagger)
	}
	return &OpenAPIPlugin{
		Swagger:  swagger,
		Builder:  NewBuilder(InterfaceBuildOptionDefault, swagger.Definitions),
		Basepath: basepath,
		UI: NewOpenAPIUI(StaticOpenAPIHandler(func(r *http.Request) (any, error) {
			return swagger, nil
		})),
	}
}

// Install implements Plugin.
func (s *OpenAPIPlugin) Install(m *api.API) error {
	m.Group(s.UI.Group(s.Basepath))
	return nil
}

// OnRoute implements Plugin.
func (s *OpenAPIPlugin) OnRoute(route *api.Route) error {
	// AddSwaggerOperation(s.Swagger, *route, s.Builder)
	return nil
}

// OnBuild implements RoutePlugin.
func (s *OpenAPIPlugin) OnBuild(m *api.API, routes []*api.Route) error {
	for _, route := range routes {
		AddSwaggerOperation(s.Swagger, *route, s.Builder)
	}
	return nil
}
