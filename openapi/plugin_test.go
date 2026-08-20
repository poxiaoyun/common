package openapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestOpenAPIBuildAllowsBodyAndPathParametersWithSameName(t *testing.T) {
	m := api.New().Plugin(NewAPIDocPlugin())
	m.Route(api.PUT("/v1/tenants/{tenant}/alertrules/{rule}").Param(
		api.BodyParam("rule", struct{}{}),
	))

	require.NotPanics(t, func() {
		m.Build()
	})
}

func TestOpenAPIBuildNormalizesRelativeGroupPath(t *testing.T) {
	plugin := NewAPIDocPlugin()
	m := api.New().Plugin(plugin)
	m.Group(api.NewGroup("internal").Route(
		api.POST("/authorize"),
	))

	require.NotPanics(t, func() {
		m.Build()
	})
	require.NotNil(t, plugin.OpenAPI.Paths.Find("/internal/authorize"))
	require.Nil(t, plugin.OpenAPI.Paths.Find("internal/authorize"))
}

func TestOpenAPIPath(t *testing.T) {
	plugin := NewAPIDocPlugin().WithPath("/docs")
	handler := api.New().Plugin(plugin).Build()

	require.Equal(t, http.StatusOK, request(t, handler, "/docs/openapi.json").Code)
	require.Equal(t, http.StatusNotFound, request(t, handler, "/openapi/openapi.json").Code)
}
