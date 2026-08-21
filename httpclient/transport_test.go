package httpclient_test

import (
	"crypto/tls"
	"net/http"
	"testing"

	"xiaoshiai.cn/common/httpclient"
)

type unsupportedRoundTripper func(*http.Request) (*http.Response, error)

func (f unsupportedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
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
