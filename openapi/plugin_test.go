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
