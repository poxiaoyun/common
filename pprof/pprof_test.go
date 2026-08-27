package pprof_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	commonpprof "xiaoshiai.cn/common/pprof"
)

func TestHandlerServesGetRequests(t *testing.T) {
	handler := commonpprof.Handler()
	for _, path := range []string{"/debug/vars", "/debug/pprof/", "/debug/pprof/cmdline", "/debug/pprof/symbol"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
			}
		})
	}
}

func TestHandlerRejectsNonGetRequests(t *testing.T) {
	handler := commonpprof.Handler()
	paths := []string{
		"/debug/vars",
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/heap",
	}
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, path := range paths {
			t.Run(method+" "+path, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
				if response.Code != http.StatusMethodNotAllowed {
					t.Fatalf("%s %s status = %d, want %d", method, path, response.Code, http.StatusMethodNotAllowed)
				}
				if allow := response.Header().Get("Allow"); allow != "GET, HEAD" {
					t.Fatalf("%s %s Allow = %q, want %q", method, path, allow, "GET, HEAD")
				}
			})
		}
	}
}
