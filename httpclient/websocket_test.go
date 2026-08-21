package httpclient_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"xiaoshiai.cn/common/httpclient"
)

func TestStreamWebSocket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/events")
		}
		if got := r.URL.Query().Get("source"); got != "test" {
			t.Errorf("source = %q, want %q", got, "test")
		}
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer connection.Close()
		if err := connection.WriteMessage(websocket.TextMessage, []byte("ready")); err != nil {
			t.Errorf("write websocket message: %v", err)
		}
	}))
	defer server.Close()

	stop := errors.New("stop")
	address := "ws" + strings.TrimPrefix(server.URL, "http") + "/events?source=test"
	err := httpclient.StreamWebSocket(context.Background(), address, func(_ context.Context, message []byte) error {
		if got := string(message); got != "ready" {
			t.Errorf("message = %q, want %q", got, "ready")
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("StreamWebSocket() error = %v, want %v", err, stop)
	}
}

func TestNewWebSocketClientDoesNotMutateHTTPTransportTLSConfig(t *testing.T) {
	server := &url.URL{Scheme: "https", Host: "events.example"}
	original := &tls.Config{NextProtos: []string{"h2"}}

	_, err := httpclient.NewWebSocketClient(&httpclient.ClientConfig{
		Server:       server,
		RoundTripper: &http.Transport{TLSClientConfig: original},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := original.NextProtos, []string{"h2"}; !slices.Equal(got, want) {
		t.Fatalf("HTTP transport TLS NextProtos = %v, want unchanged %v", got, want)
	}
}

func TestWebSocketClientUsesConfiguredBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer websocket-token" {
			http.Error(w, "missing websocket authorization", http.StatusUnauthorized)
			return
		}
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer connection.Close()
		if err := connection.WriteMessage(websocket.TextMessage, []byte("ready")); err != nil {
			t.Errorf("write websocket message: %v", err)
		}
	}))
	defer server.Close()

	config, err := httpclient.BuildClientConfig(t.Context(), &httpclient.Options{
		Server: server.URL,
		Token:  "websocket-token",
	}, httpclient.TransportConfig{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := httpclient.NewWebSocketClient(config)
	if err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop")
	err = client.Stream(t.Context(), "events", func(_ context.Context, _ int, message []byte) error {
		if got := string(message); got != "ready" {
			t.Errorf("message = %q, want %q", got, "ready")
		}
		return stop
	}, httpclient.WebSocketOptions{})
	if !errors.Is(err, stop) {
		t.Fatalf("Stream() error = %v, want %v", err, stop)
	}
}
