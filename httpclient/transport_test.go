package httpclient_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"testing"

	"xiaoshiai.cn/common/httpclient"
)

type unsupportedRoundTripper func(*http.Request) (*http.Response, error)

func (f unsupportedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCloneRequestOnlyCopiesHeadersDeeply(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://api.example/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add("X-Test", "original")
	clone := httpclient.CloneRequest(request)
	if clone == request {
		t.Fatal("CloneRequest() returned the original request")
	}
	if clone.URL != request.URL || clone.Body != request.Body {
		t.Fatal("CloneRequest() deep-copied fields other than Header")
	}
	clone.Header.Set("X-Test", "clone")
	if got := request.Header.Get("X-Test"); got != "original" {
		t.Fatalf("original header = %q, want original", got)
	}
}

func TestAuthorizationRoundTrippersCloneCallerRequest(t *testing.T) {
	tests := []struct {
		name      string
		transport func(http.RoundTripper) http.RoundTripper
		want      string
	}{
		{
			name: "bearer",
			transport: func(base http.RoundTripper) http.RoundTripper {
				return httpclient.NewBearerTokenRoundTripper("secret", base)
			},
			want: "Bearer secret",
		},
		{
			name: "basic",
			transport: func(base http.RoundTripper) http.RoundTripper {
				return httpclient.NewBasicAuthRoundTripper("user", "password", base)
			},
			want: "Basic dXNlcjpwYXNzd29yZA==",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "https://api.example/resource", nil)
			if err != nil {
				t.Fatal(err)
			}
			transport := test.transport(unsupportedRoundTripper(func(outgoing *http.Request) (*http.Response, error) {
				if outgoing == request {
					t.Fatal("RoundTrip received the caller's request instead of a clone")
				}
				if authorization := outgoing.Header.Get("Authorization"); authorization != test.want {
					t.Fatalf("Authorization = %q, want %q", authorization, test.want)
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}))
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if authorization := request.Header.Get("Authorization"); authorization != "" {
				t.Fatalf("caller Authorization = %q, want empty", authorization)
			}
		})
	}
}

type tlsHolderRoundTripper struct {
	config *tls.Config
}

func (t *tlsHolderRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func (t *tlsHolderRoundTripper) TLSClientConfig() *tls.Config {
	return t.config
}

type wrappedRoundTripper struct {
	transport http.RoundTripper
}

func (w *wrappedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return w.transport.RoundTrip(request)
}

func (w *wrappedRoundTripper) WrappedRoundTripper() http.RoundTripper {
	return w.transport
}

func TestTLSClientConfig(t *testing.T) {
	config := &tls.Config{ServerName: "files.example"}
	tests := []struct {
		name      string
		transport http.RoundTripper
		want      *tls.Config
		wantError bool
	}{
		{name: "http transport", transport: &http.Transport{TLSClientConfig: config}, want: config},
		{name: "holder", transport: &tlsHolderRoundTripper{config: config}, want: config},
		{
			name:      "wrapped holder",
			transport: &wrappedRoundTripper{transport: &tlsHolderRoundTripper{config: config}},
			want:      config,
		},
		{name: "unknown transport", transport: unsupportedRoundTripper(nil), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := httpclient.TLSClientConfig(test.transport)
			if (err != nil) != test.wantError {
				t.Fatalf("TLSClientConfig() error = %v, wantError = %t", err, test.wantError)
			}
			if got != test.want {
				t.Fatalf("TLSClientConfig() = %p, want %p", got, test.want)
			}
		})
	}
}

func TestFindRequestAuthenticator(t *testing.T) {
	transport := &wrappedRoundTripper{transport: httpclient.AuthorizationRoundTripper{
		Authorization: "Bearer secret",
		Transport:     http.DefaultTransport,
	}}
	authenticator := httpclient.FindRequestAuthenticator(transport)
	if authenticator == nil {
		t.Fatal("FindRequestAuthenticator() = nil")
	}
	request, err := http.NewRequest(http.MethodGet, "https://api.example/resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := authenticator.AuthenticateRequest(request); err != nil {
		t.Fatal(err)
	}
	if authorization := request.Header.Get("Authorization"); authorization != "Bearer secret" {
		t.Fatalf("Authorization = %q, want Bearer secret", authorization)
	}
	if got := httpclient.FindRequestAuthenticator(unsupportedRoundTripper(nil)); got != nil {
		t.Fatalf("FindRequestAuthenticator() = %#v, want nil", got)
	}
}
