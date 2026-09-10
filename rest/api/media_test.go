package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/rest/api"
)

func TestRouteMediaConditions(t *testing.T) {
	tests := []struct {
		name         string
		contentTypes []string
		accepts      []string
		contentType  []string
		accept       []string
		match        bool
	}{
		{name: "distinct input and output", contentTypes: []string{"application/json"}, accepts: []string{"application/yaml"}, contentType: []string{"application/json"}, accept: []string{"application/yaml"}, match: true},
		{name: "input is not output", contentTypes: []string{"application/json"}, accepts: []string{"application/yaml"}, contentType: []string{"application/yaml"}, accept: []string{"application/json"}},
		{name: "missing content type", contentTypes: []string{"application/json"}},
		{name: "multiple content types", contentTypes: []string{"application/json"}, contentType: []string{"application/json", "application/json"}},
		{name: "wildcard is not a content type", contentTypes: []string{"*/*"}, contentType: []string{"*/*"}},
		{name: "content type range", contentTypes: []string{"application/*"}, contentType: []string{"application/json; charset=UTF-8"}, match: true},
		{name: "content type parameters", contentTypes: []string{"application/json; charset=utf-8"}, contentType: []string{"Application/JSON; Charset=UTF-8; profile=extra"}, match: true},
		{name: "missing constrained parameter", contentTypes: []string{"application/json; profile=v1"}, contentType: []string{"application/json"}},
		{name: "parameter value is case sensitive", contentTypes: []string{"application/json; profile=V1"}, contentType: []string{"application/json; profile=v1"}},
		{name: "empty parameter is not omitted", contentTypes: []string{"application/json; profile=\"\""}, contentType: []string{"application/json"}},
		{name: "missing accept", accepts: []string{"application/json"}, match: true},
		{name: "empty accept", accepts: []string{"application/json"}, accept: []string{""}},
		{name: "accept wildcard", accepts: []string{"application/json"}, accept: []string{"*/*"}, match: true},
		{name: "accept type range", accepts: []string{"application/json"}, accept: []string{"application/*"}, match: true},
		{name: "accept list", accepts: []string{"application/json"}, accept: []string{"text/html, application/json;q=0.8"}, match: true},
		{name: "accept multiple header fields", accepts: []string{"application/json"}, accept: []string{"text/html", "application/json"}, match: true},
		{name: "zero quality", accepts: []string{"application/json"}, accept: []string{"application/json;q=0"}},
		{name: "specific exclusion before wildcard", accepts: []string{"application/json"}, accept: []string{"application/json;q=0, */*;q=1"}},
		{name: "specific exclusion after wildcard", accepts: []string{"application/json"}, accept: []string{"*/*;q=1, application/json;q=0"}},
		{name: "type exclusion", accepts: []string{"application/json"}, accept: []string{"application/*;q=0, */*;q=1"}},
		{name: "specific allowance", accepts: []string{"application/json"}, accept: []string{"application/json;q=0.1, */*;q=0"}, match: true},
		{name: "parameter exclusion", accepts: []string{"text/plain;format=flowed"}, accept: []string{"text/plain;format=flowed;q=0, text/plain;q=1"}},
		{name: "unoffered response parameter", accepts: []string{"application/json"}, accept: []string{"application/json;profile=v1"}},
		{name: "quoted comma parameter", accepts: []string{"application/json;profile=\"a,b\""}, accept: []string{"text/html, application/json;profile=\"a,b\";q=0.5"}, match: true},
		{name: "accept charset is case insensitive", accepts: []string{"text/plain;charset=UTF-8"}, accept: []string{"text/plain;CHARSET=utf-8"}, match: true},
		{name: "quality outside range", accepts: []string{"application/json"}, accept: []string{"application/json;q=1.1"}},
		{name: "invalid quality", accepts: []string{"application/json"}, accept: []string{"application/json;q=NaN"}},
		{name: "too many quality digits", accepts: []string{"application/json"}, accept: []string{"application/json;q=0.1234"}},
		{name: "smallest positive quality", accepts: []string{"application/json"}, accept: []string{"application/json;q=0.001"}, match: true},
		{name: "invalid accept type", accepts: []string{"application/json"}, accept: []string{"*/json"}},
		{name: "invalid quoted parameter", accepts: []string{"application/json"}, accept: []string{"application/json;profile=\"broken"}},
		{name: "offered wildcard keeps nonexcluded representations", accepts: []string{"application/*"}, accept: []string{"application/json;q=0, */*;q=1"}, match: true},
		{name: "offered wildcard all excluded", accepts: []string{"application/*"}, accept: []string{"application/*;q=0, */*;q=1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := api.NewMux()
			fallback := mediaRoute("fallback")
			require.NoError(t, mux.Register(&fallback))
			route := mediaRoute("matched").
				ContentType(test.contentTypes...).
				Accept(test.accepts...)
			require.NoError(t, mux.Register(&route))
			request := httptest.NewRequest(http.MethodPost, "/media", nil)
			for _, value := range test.contentType {
				request.Header.Add("Content-Type", value)
			}
			for _, value := range test.accept {
				request.Header.Add("Accept", value)
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			want := "fallback"
			if test.match {
				want = "matched"
			}
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, want, response.Body.String())
		})
	}
}

func TestMediaVariantPreference(t *testing.T) {
	mux := api.NewMux()
	for _, route := range []api.Route{
		mediaRoute("fallback"),
		mediaRoute("json").
			Accept("application/json"),
		mediaRoute("xml").
			Accept("application/xml"),
	} {
		require.NoError(t, mux.Register(&route))
	}
	for _, test := range []struct{ accept, want string }{
		{"application/json;q=0.2, application/xml;q=0.9", "xml"},
		{"application/json;q=0.2, application/*;q=0.9", "xml"},
		{"application/json, application/xml", "json"},
		{"application/json;q=0, application/xml;q=0", "fallback"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/media", nil)
		request.Header.Set("Accept", test.accept)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, test.want, response.Body.String())
	}
}

func TestContentTypeSpecificity(t *testing.T) {
	mux := api.NewMux()
	for _, route := range []api.Route{
		mediaRoute("fallback"),
		mediaRoute("wildcard").
			ContentType("*/*"),
		mediaRoute("application").
			ContentType("application/*"),
		mediaRoute("json").
			ContentType("application/json"),
		mediaRoute("profile").
			ContentType("application/json;profile=v1"),
	} {
		require.NoError(t, mux.Register(&route))
	}
	for _, test := range []struct{ contentType, want string }{
		{"", "fallback"},
		{"text/plain", "wildcard"},
		{"application/xml", "application"},
		{"application/json", "json"},
		{"application/json;profile=v1", "profile"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/media", nil)
		request.Header.Set("Content-Type", test.contentType)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		require.Equal(t, test.want, response.Body.String())
	}
}

func TestMediaRegistrationRejectsInvalidAndDuplicateConditions(t *testing.T) {
	mux := api.NewMux()
	route := mediaRoute("json").
		ContentType("APPLICATION/JSON; CHARSET=UTF-8", "application/xml")
	require.NoError(t, mux.Register(&route))
	duplicate := mediaRoute("duplicate").
		ContentType("application/xml", "application/json;charset=utf-8")
	require.ErrorContains(t, mux.Register(&duplicate), "already registered")
	for _, value := range []string{"json", "*/json", "application/json;q=0.5", "application/json;broken"} {
		invalid := mediaRoute("invalid").
			ContentType(value)
		require.Error(t, mux.Register(&invalid))
	}
}

func mediaRoute(marker string) api.Route {
	return api.POST("/media").
		To(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, marker)
		})
}
