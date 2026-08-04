package openapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/version"
)

var _ api.RoutePlugin = &OpenAPIPlugin{}

type OpenAPIPlugin struct {
	Basepath string
	OpenAPI  *openapi3.T
	Builder  *Builder
	UI       OpenAPIUI
}

// NewAPIDocPlugin creates an OpenAPI 3.1 documentation plugin.
// The configure callback runs before routes are projected into the document.
func NewAPIDocPlugin(basepath string, configure func(document *openapi3.T)) *OpenAPIPlugin {
	if basepath == "" {
		basepath = "/docs"
	}
	document := &openapi3.T{
		OpenAPI: "3.1.1",
		Info: &openapi3.Info{
			Title:       "API Documentation",
			Version:     version.Get().String(),
			Description: "API documentation",
		},
		Components: &openapi3.Components{Schemas: openapi3.Schemas{}},
		Paths:      openapi3.NewPaths(),
	}
	if configure != nil {
		configure(document)
	}
	if document.Info == nil {
		document.Info = &openapi3.Info{Title: "API Documentation", Version: version.Get().String()}
	}
	if document.Components == nil {
		document.Components = &openapi3.Components{}
	}
	if document.Components.Schemas == nil {
		document.Components.Schemas = openapi3.Schemas{}
	}
	if document.Paths == nil {
		document.Paths = openapi3.NewPaths()
	}

	plugin := &OpenAPIPlugin{
		Basepath: basepath,
		OpenAPI:  document,
		Builder:  NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas),
	}
	plugin.UI = NewOpenAPIUI(StaticOpenAPIHandler(func(r *http.Request) (any, error) {
		return plugin.OpenAPI, nil
	}))
	return plugin
}

func (p *OpenAPIPlugin) Install(m *api.API) error {
	m.Group(p.UI.Group(p.Basepath))
	return nil
}

func (p *OpenAPIPlugin) OnRoute(route *api.Route) error {
	return nil
}

func (p *OpenAPIPlugin) OnBuild(m *api.API, routes []*api.Route) error {
	for _, route := range routes {
		if err := AddOpenAPIOperation(p.OpenAPI, *route, p.Builder); err != nil {
			return err
		}
	}
	if err := p.OpenAPI.Validate(
		context.Background(),
		openapi3.IsOpenAPI31OrLater(),
	); err != nil {
		return fmt.Errorf("validate generated OpenAPI 3.1 document: %w", err)
	}
	return nil
}
