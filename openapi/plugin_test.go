package openapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestOpenAPIBuildAllowsBodyAndPathParametersWithSameName(t *testing.T) {
	m := api.New().Plugin(NewAPIDocPlugin("/docs", nil))
	m.Route(api.PUT("/v1/tenants/{tenant}/alertrules/{rule}").Param(
		api.BodyParam("rule", struct{}{}),
	))

	require.NotPanics(t, func() {
		m.Build()
	})
}

func TestOpenAPIBuildNormalizesRelativeGroupPath(t *testing.T) {
	plugin := NewAPIDocPlugin("/docs", nil)
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
