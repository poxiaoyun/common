package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

func TestWhitelistAuthorizerMatchesExtractedHTTPPath(t *testing.T) {
	filter := api.NewAuthorizationFilter(api.NewWhitelistAuthorizer(`^/healthz$`))

	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/healthz", wantStatus: http.StatusNoContent},
		{path: "/readyz", wantStatus: http.StatusForbidden},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request = request.WithContext(api.WithAttributes(request.Context(), &api.Attributes{Path: test.path}))
			response := httptest.NewRecorder()
			filter.Process(response, request, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}
