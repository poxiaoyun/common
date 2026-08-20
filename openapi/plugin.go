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

// Document is an OpenAPI 3 document.
type Document = openapi3.T

const defaultPath = "/openapi"

type OpenAPIPlugin struct {
	OpenAPI *Document
	Builder *Builder
	UI      OpenAPIUI
	Path    string
}

// NewAPIDocPlugin creates an OpenAPI 3.1 documentation plugin served under
// /openapi.
func NewAPIDocPlugin() *OpenAPIPlugin {
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
	plugin := &OpenAPIPlugin{OpenAPI: document, Path: defaultPath}
	plugin.Builder = NewBuilder(InterfaceBuildOptionDefault, document.Components.Schemas)
	plugin.UI = NewOpenAPIUI(StaticOpenAPIHandler(func(r *http.Request) (any, error) {
		return plugin.OpenAPI, nil
	}))
	return plugin
}

// WithPath sets the route prefix for the OpenAPI UI and document and returns
// the same plugin for construction-time chaining.
func (p *OpenAPIPlugin) WithPath(path string) *OpenAPIPlugin {
	p.Path = path
	return p
}

// ConfigureDocument updates the plugin document in place and returns the same
// plugin for construction-time chaining. Call it before installing the plugin.
func (p *OpenAPIPlugin) ConfigureDocument(configure func(document *Document)) *OpenAPIPlugin {
	configure(p.OpenAPI)
	return p
}

func (p *OpenAPIPlugin) Install(m *api.API) error {
	m.Group(p.UI.Group(p.Path))
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
