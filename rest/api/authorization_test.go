package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
)

func TestAuthorizationFilterPreservesStatusError(t *testing.T) {
	filter := NewAuthorizationFilter(AuthorizerFunc(
		func(context.Context, AuthenticationInfo, Attributes) (Decision, string, error) {
			return DecisionDeny, "resource is hidden", commonerrors.NewNotFound("document", "secret")
		},
	))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/documents/secret", nil)
	request = request.WithContext(WithAuthentication(request.Context(), AuthenticationInfo{
		Subject: Subject{ID: "user"},
	}))
	request = request.WithContext(WithAttributes(request.Context(), &Attributes{
		Action: "get",
		Path:   "/documents/secret",
	}))

	filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached handler")
	}))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestRequestAuthorizationFilterPreservesCustomErrorResponse(t *testing.T) {
	want := commonerrors.NewTooManyRequests("try again later", 0)
	filter := NewRequestAuthorizationFilter(RequestAuthorizerFunc(
		func(*http.Request) (Decision, string, error) {
			return DecisionDeny, "", want
		},
	))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/resource", nil)

	filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached handler")
	}))

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
}

func TestAuthorizationFiltersRedactAndLogDiagnosticErrors(t *testing.T) {
	diagnostic := errors.New("policy database DSN contains a secret")
	tests := []struct {
		name   string
		filter Filter
	}{
		{
			name: "domain authorizer",
			filter: NewAuthorizationFilter(AuthorizerFunc(
				func(context.Context, AuthenticationInfo, Attributes) (Decision, string, error) {
					return DecisionDeny, "", diagnostic
				},
			)),
		},
		{
			name: "request authorizer",
			filter: NewRequestAuthorizationFilter(RequestAuthorizerFunc(
				func(*http.Request) (Decision, string, error) {
					return DecisionDeny, "", diagnostic
				},
			)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logOutput strings.Builder
			logger := funcr.New(func(prefix, args string) {
				logOutput.WriteString(prefix)
				logOutput.WriteString(args)
			}, funcr.Options{})
			request := httptest.NewRequest(http.MethodGet, "/documents/secret", nil)
			request = request.WithContext(log.NewContext(request.Context(), logger))
			request = request.WithContext(WithAuthentication(request.Context(), AuthenticationInfo{Subject: Subject{ID: "user"}}))
			request = request.WithContext(WithAttributes(request.Context(), &Attributes{Action: "get", Path: "/documents/secret"}))
			response := httptest.NewRecorder()

			tt.filter.Process(response, request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("rejected request reached handler")
			}))

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if strings.Contains(response.Body.String(), diagnostic.Error()) {
				t.Fatalf("response exposed diagnostic error: %s", response.Body.String())
			}
			if !strings.Contains(logOutput.String(), diagnostic.Error()) {
				t.Fatalf("log did not contain diagnostic error: %s", logOutput.String())
			}
		})
	}
}

func TestForbiddenMessage(t *testing.T) {
	tests := []struct {
		name           string
		authentication AuthenticationInfo
		attributes     *Attributes
		want           string
	}{
		{
			name:           "request path without resources",
			authentication: AuthenticationInfo{Subject: Subject{ID: "user-1"}},
			attributes:     &Attributes{Action: "get", Path: "/documents/secret"},
			want:           `subject "user-1" cannot get path "/documents/secret"`,
		},
		{
			name:           "nested named resources",
			authentication: AuthenticationInfo{Subject: Subject{ID: "user-1", Name: "Alice"}},
			attributes: &Attributes{
				Action: "update",
				Resources: []AttributeResource{
					{Resource: "namespaces", Name: "default"},
					{Resource: "deployments", Name: "api"},
				},
			},
			want: `subject "Alice" cannot update resource "namespaces:default:deployments:api"`,
		},
		{
			name:           "collection resource omits empty name",
			authentication: AuthenticationInfo{Subject: Subject{ID: "user-1", Email: "alice@example.com"}},
			attributes: &Attributes{
				Action: "list",
				Resources: []AttributeResource{
					{Resource: "namespaces", Name: "default"},
					{Resource: "pods"},
				},
			},
			want: `subject "alice@example.com" cannot list resource "namespaces:default:pods"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForbiddenMessage(t.Context(), tt.authentication, tt.attributes); got != tt.want {
				t.Fatalf("ForbiddenMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
