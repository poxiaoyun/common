package openapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestVendoredScalarIntegrity(t *testing.T) {
	asset, err := UIFS.ReadFile("ui/static/scalar/scalar.js")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(asset), 512)
	digest := sha256.Sum256(asset)
	assert.Equal(t, "25e0a1ef537dc7f1aa41dd8d22b94d8703dcfab34361f4d2ee84ac0600c8a457", fmt.Sprintf("%x", digest))
	assert.Contains(t, string(asset[:512]), "@scalar/api-reference 1.64.0")
}

func TestOpenAPIPluginServesOAS31AndScalarUI(t *testing.T) {
	plugin := NewAPIDocPlugin().ConfigureDocument(func(document *Document) {
		document.Info.Title = "Widget API"
	})
	handler := api.New().
		Plugin(plugin).
		Route(api.GET("/widgets").Response([]struct {
			ID string `json:"id"`
		}{})).
		Build()

	t.Run("document", func(t *testing.T) {
		response := request(t, handler, "/openapi/openapi.json")
		assert.Equal(t, http.StatusOK, response.Code)

		var document map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
		assert.Equal(t, "3.1.1", document["openapi"])
		assert.NotContains(t, document, "swagger")
		assert.NotContains(t, response.Body.String(), "#/definitions/")
		assert.Contains(t, document["paths"], "/widgets")
		assert.Contains(t, document, "components")

		loaded, err := openapi3.NewLoader().LoadFromData(response.Body.Bytes())
		require.NoError(t, err)
		require.NoError(t, loaded.Validate(context.Background(), openapi3.IsOpenAPI31OrLater()))
	})

	t.Run("index", func(t *testing.T) {
		response := request(t, handler, "/openapi/?provider=swagger")
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), "static/scalar/scalar.js")
		assert.Contains(t, response.Body.String(), "static/scalar/config.js")
		assert.NotContains(t, strings.ToLower(response.Body.String()), "swagger-ui")
	})

	t.Run("vendored scalar configuration", func(t *testing.T) {
		response := request(t, handler, "/openapi/static/scalar/config.js")
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), `url: "openapi.json"`)
		assert.Contains(t, response.Body.String(), "telemetry: false")
	})

	t.Run("legacy swagger asset removed", func(t *testing.T) {
		response := request(t, handler, "/openapi/static/swagger-ui/swagger-ui.css")
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}

func request(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}
