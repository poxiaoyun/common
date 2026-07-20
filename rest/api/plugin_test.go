package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyCheckPlugin(t *testing.T) {
	tests := []struct {
		name   string
		check  func(context.Context) error
		status int
	}{
		{name: "ready", status: http.StatusOK},
		{name: "not ready", check: func(context.Context) error { return errors.New("store unavailable") }, status: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := New().Plugin(ReadyCheckPlugin{CheckFun: test.check}).Build()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}
