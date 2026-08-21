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
	"fmt"
	"net"
	"net/http"
	"strings"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
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

// ServeContext runs an HTTP or HTTPS server until ctx is canceled.
func ServeContext(ctx context.Context, listen string, handler http.Handler, options ...ServerOption) error {
	log := log.FromContext(ctx)
	s := http.Server{
		Handler:     handler,
		Addr:        listen,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}
	for _, opt := range options {
		if err := opt(&s); err != nil {
			return fmt.Errorf("configure http server: %w", err)
		}
	}
	go func() {
		<-ctx.Done()
		log.Info("closing http(s) server", "listen", listen)
		s.Close()
	}()
	if s.TLSConfig != nil {
		// http2 support with tls enabled
		http2.ConfigureServer(&s, &http2.Server{})
		log.Info("starting https server", "listen", listen)
		if err := s.ListenAndServeTLS("", ""); err != nil {
			return fmt.Errorf("listen and serve https: %v", err)
		}
		return nil
	} else {
		// http2 support without https
		s.Handler = h2c.NewHandler(s.Handler, &http2.Server{})
		log.Info("starting http server", "listen", listen)
		if err := s.ListenAndServe(); err != nil {
			return fmt.Errorf("listen and serve http: %v", err)
		}
		return nil
	}
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
