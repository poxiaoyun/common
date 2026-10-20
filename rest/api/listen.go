// Copyright 2022 The kubegems.io Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"xiaoshiai.cn/common/log"
	libtls "xiaoshiai.cn/common/tls"
)

// ServerOption configures the HTTP server before it starts listening.
type ServerOption func(*http.Server) error

// WithTLSConfig installs an existing TLS configuration.
func WithTLSConfig(tls *tls.Config) ServerOption {
	return func(s *http.Server) error { s.TLSConfig = tls; return nil }
}

// WithDynamicTLSConfig reloads the serving certificate for new TLS handshakes.
func WithDynamicTLSConfig(cert, key string) ServerOption {
	return func(s *http.Server) error {
		tlsconfig, err := libtls.NewServingConfig(cert, key)
		if err != nil {
			return fmt.Errorf("create dynamic TLS config: %w", err)
		}
		s.TLSConfig = tlsconfig
		return nil
	}
}

// Server owns an HTTP server's listening and graceful shutdown lifecycle.
// Configure it before calling Run, and run each instance only once.
type Server struct {
	http.Server
	// ShutdownTimeout bounds draining active requests; zero selects 30 seconds.
	ShutdownTimeout time.Duration
}

// Run serves HTTP or HTTPS until cancellation or a serving failure. It waits for
// shutdown, returns nil on normal cancellation, and force-closes connections if
// the shutdown deadline expires. Hijacked connections remain caller-owned.
func (s *Server) Run(ctx context.Context) error {
	timeout := s.ShutdownTimeout
	if timeout < 0 {
		return fmt.Errorf("shutdown timeout must not be negative")
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if ctx.Err() != nil {
		return nil
	}

	requests, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	if s.BaseContext == nil {
		s.BaseContext = func(net.Listener) context.Context { return requests }
	}
	if s.Protocols == nil {
		s.Protocols = &http.Protocols{}
		s.Protocols.SetHTTP1(true)
		s.Protocols.SetHTTP2(true)
		s.Protocols.SetUnencryptedHTTP2(true)
	}
	listen := s.Addr
	if listen == "" {
		listen = ":http"
		if s.TLSConfig != nil {
			listen = ":https"
		}
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", listen)
	if err != nil {
		return fmt.Errorf("listen HTTP server: %w", err)
	}
	defer listener.Close()

	servingDone := make(chan error, 1)
	shutdownDone := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
		case err := <-servingDone:
			if !errors.Is(err, http.ErrServerClosed) {
				shutdownDone <- s.Close()
				return
			}
		}
		// Serve also returns ErrServerClosed when a caller starts Shutdown.
		// Wait for active requests through the same bounded shutdown path.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		err := s.Shutdown(shutdown)
		if err != nil {
			err = errors.Join(err, s.Close())
		}
		shutdownDone <- err
	}()
	log.FromContext(ctx).
		Info("starting HTTP server", "listen", listener.Addr().
			String(), "tls", s.TLSConfig != nil)
	if s.TLSConfig != nil {
		err = s.ServeTLS(listener, "", "")
	} else {
		err = s.Server.Serve(listener)
	}
	servingDone <- err
	shutdownErr := <-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, shutdownErr)
}

// ServeContext configures and runs a server with the default shutdown timeout.
func ServeContext(ctx context.Context, listen string, handler http.Handler, options ...ServerOption) error {
	server := &Server{Server: http.Server{Handler: handler, Addr: listen}}
	for _, option := range options {
		if err := option(&server.Server); err != nil {
			return fmt.Errorf("configure HTTP server: %w", err)
		}
	}
	return server.Run(ctx)
}

// GRPCHTTPMux routes gRPC requests to grpchandler and all others to httphandler.
func GRPCHTTPMux(httphandler http.Handler, grpchandler http.Handler) http.Handler {
	httphandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpchandler.ServeHTTP(w, r)
		} else {
			httphandler.ServeHTTP(w, r)
		}
	})
	return httphandler
}
