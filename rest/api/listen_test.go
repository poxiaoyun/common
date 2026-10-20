package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"xiaoshiai.cn/common/log"
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
	if err != nil {
		return
	}
	t.Fatal("WithDynamicTLSConfig returned no error")
}

func TestServerDrainsRequests(t *testing.T) {
	for _, protocol := range []struct{ secure, http2 bool }{{false, false}, {false, true}, {true, false}, {true, true}} {
		t.Run(fmt.Sprintf("tls=%t/http2=%t", protocol.secure, protocol.http2), func(t *testing.T) {
			type runtimeKey struct{}
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), runtimeKey{}, "runtime"))
			defer cancel()
			ready := make(chan string, 1)
			// Discover the ephemeral listener through the server's structured log
			// so this test exercises the default BaseContext.
			logger := funcr.NewJSON(func(record string) {
				var event struct {
					Listen string `json:"listen"`
				}
				if err := json.Unmarshal([]byte(record), &event); err != nil {
					t.Error(err)
					return
				}
				if event.Listen != "" {
					ready <- event.Listen
				}
			}, funcr.Options{})
			ctx = log.NewContext(ctx, logger)
			entered := make(chan context.Context, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			server := &api.Server{Server: http.Server{
				Addr: "127.0.0.1:0",
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					entered <- r.Context()
					<-release
					_, _ = io.WriteString(w, fmt.Sprintf("completed HTTP/%d", r.ProtoMajor))
				}),
			}}
			client := &http.Client{Timeout: 5 * time.Second}
			scheme := "http"
			if protocol.secure {
				seed := httptest.NewTLSServer(http.NotFoundHandler())
				server.TLSConfig = seed.TLS.Clone()
				client = seed.Client()
				client.Timeout = 5 * time.Second
				seed.Close()
				scheme = "https"
			}
			transport := client.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			configured := transport.(*http.Transport).
				Clone()
			configured.Protocols = &http.Protocols{}
			configured.Protocols.SetHTTP1(!protocol.http2)
			configured.Protocols.SetHTTP2(protocol.http2)
			configured.Protocols.SetUnencryptedHTTP2(protocol.http2 && !protocol.secure)
			client.Transport = configured
			defer client.CloseIdleConnections()
			shutdown := make(chan struct{})
			server.RegisterOnShutdown(func() { close(shutdown) })
			done := make(chan error, 1)
			go func() { done <- server.Run(ctx) }()
			address := <-ready
			responseDone := make(chan error, 1)
			go func() {
				response, err := client.Get(scheme + "://" + address)
				if err != nil {
					responseDone <- err
					return
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil {
					responseDone <- err
					return
				}
				expected := "completed HTTP/1"
				if protocol.http2 {
					expected = "completed HTTP/2"
				}
				if response.StatusCode != 200 || string(body) != expected {
					responseDone <- fmt.Errorf("response: %d %s", response.StatusCode, body)
					return
				}
				responseDone <- nil
			}()
			requestCtx := <-entered
			if got := requestCtx.Value(runtimeKey{}); got != "runtime" {
				t.Fatalf("runtime context value = %v", got)
			}
			cancel()
			<-shutdown
			select {
			case err := <-done:
				t.Fatalf("Run returned before request drained: %v", err)
			default:
			}
			select {
			case <-requestCtx.Done():
				t.Fatal("shutdown canceled an active request")
			default:
			}
			releaseOnce.Do(func() { close(release) })
			if err := <-responseDone; err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("normal shutdown: %v", err)
			}
		})
	}
}

func TestServerDrainsExternalShutdown(t *testing.T) {
	ready := make(chan string, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	server := &api.Server{Server: http.Server{
		Addr: "127.0.0.1:0",
		BaseContext: func(listener net.Listener) context.Context {
			ready <- listener.Addr().
				String()
			return context.Background()
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-release
			_, _ = io.WriteString(w, "completed")
		}),
	}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	address := <-ready
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	responseDone := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + address)
		if err != nil {
			responseDone <- err
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			responseDone <- err
			return
		}
		if string(body) != "completed" {
			responseDone <- fmt.Errorf("response = %q", body)
			return
		}
		responseDone <- nil
	}()
	<-entered
	shutdownStarted := make(chan struct{}, 2)
	server.RegisterOnShutdown(func() { shutdownStarted <- struct{}{} })
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(t.Context()) }()
	<-shutdownStarted
	// Keep the request active until Run has observed Serve exiting and joined
	// shutdown. Returning here would abandon the active request.
	select {
	case err := <-done:
		t.Fatalf("Run returned before request drained: %v", err)
	case <-shutdownStarted:
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-responseDone; err != nil {
		t.Fatal(err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServerShutdownDeadlineClosesConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := make(chan string, 1)
	entered := make(chan struct{})
	released := make(chan struct{})
	server := &api.Server{
		Server: http.Server{
			Addr: "127.0.0.1:0",
			BaseContext: func(listener net.Listener) context.Context {
				ready <- listener.Addr().
					String()
				return context.Background()
			},
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				<-r.Context().
					Done()
				close(released)
			}),
		},
		ShutdownTimeout: 20 * time.Millisecond,
	}
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	address := <-ready
	clientDone := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + address)
		if err != nil {
			clientDone <- err
			return
		}
		response.Body.Close()
		clientDone <- nil
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want shutdown deadline", err)
	}
	<-released
	if err := <-clientDone; err != nil {
		return
	}
	t.Fatal("request succeeded after its connection was forcibly closed")
}

func TestServerPreservesListenError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &api.Server{Server: http.Server{Addr: listener.Addr().
		String()}}
	if err := server.Run(t.Context()); !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("Run error = %v, want address in use", err)
	}
}

func TestServeContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := api.ServeContext(ctx, "127.0.0.1:0", http.NotFoundHandler()); err != nil {
		t.Fatalf("canceled ServeContext: %v", err)
	}
}
