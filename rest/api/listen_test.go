package api_test

import (
	"errors"
	"net/http"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

func TestServeContextReturnsServerOptionError(t *testing.T) {
	expected := errors.New("invalid server configuration")
	err := api.ServeContext(t.Context(), "", http.NotFoundHandler(), func(*http.Server) error {
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("ServeContext() error = %v, want %v", err, expected)
	}
}

func TestWithDynamicTLSConfigRequiresReadableCertificatePair(t *testing.T) {
	server := &http.Server{}
	err := api.WithDynamicTLSConfig("missing.crt", "missing.key")(server)
	if err == nil {
		t.Fatal("WithDynamicTLSConfig returned no error")
	}
}
