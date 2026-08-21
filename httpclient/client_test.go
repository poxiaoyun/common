package httpclient_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"xiaoshiai.cn/common/httpclient"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewClientFromOptionsWithTransportPreservesTLSAndAuthentication(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", request.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var wrappedTLS *tls.Config
	var tlsErr error
	client, err := httpclient.NewClientFromOptionsWithTransport(
		t.Context(),
		&httpclient.Options{
			Server:                server.URL,
			Token:                 "secret",
			InsecureSkipTLSVerify: true,
		},
		func(base http.RoundTripper) http.RoundTripper {
			wrappedTLS, tlsErr = httpclient.TLSClientConfig(base)
			return roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				return base.RoundTrip(request)
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if tlsErr != nil {
		t.Fatal(tlsErr)
	}
	if wrappedTLS == nil || !wrappedTLS.InsecureSkipVerify {
		t.Fatalf("wrapped TLS config = %#v, want InsecureSkipVerify", wrappedTLS)
	}
	response, err := client.Get("/assets").Do(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestNewClientFromOptionsWithTransportUsesBaseTransportWhenWrapperIsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := httpclient.NewClientFromOptionsWithTransport(
		t.Context(),
		&httpclient.Options{Server: server.URL},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get("/").Do(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestBuildClientConfigInstallsDialContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var called atomic.Bool
	dialer := &net.Dialer{}
	clientConfig, err := httpclient.BuildClientConfig(
		t.Context(),
		&httpclient.Options{Server: server.URL},
		httpclient.TransportConfig{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			called.Store(true)
			return dialer.DialContext(ctx, network, address)
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if clientConfig.DialContext == nil {
		t.Fatal("ClientConfig.DialContext is nil")
	}
	client := httpclient.NewClientFromClientConfig(clientConfig)
	response, err := client.Get("/").Do(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !called.Load() {
		t.Fatal("DialContext was not used by the HTTP transport")
	}
}

func TestBuildClientConfigRetainsProxyForWebSocket(t *testing.T) {
	clientConfig, err := httpclient.BuildClientConfig(
		t.Context(),
		&httpclient.Options{
			Server:   "https://files.example",
			ProxyURL: "http://proxy.example:8080",
		},
		httpclient.TransportConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://files.example/assets", nil)
	proxyURL, err := clientConfig.Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL.String() != "http://proxy.example:8080" {
		t.Fatalf("Proxy = %q, want http://proxy.example:8080", proxyURL)
	}
}
