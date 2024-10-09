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
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"xiaoshiai.cn/common/log"
)

type ServerOption func(*http.Server) error

func WithTLSConfig(tls *tls.Config) ServerOption {
	return func(s *http.Server) error { s.TLSConfig = tls; return nil }
}

func WithDynamicTLSConfig(ctx context.Context, cert, key string) ServerOption {
	return func(s *http.Server) error {
		tlsconfig, err := NewDynamicTLSConfig(ctx, cert, key)
		if err != nil {
			return fmt.Errorf("failed to create dynamic tls config: %v", err)
		}
		s.TLSConfig = tlsconfig
		return nil
	}
}

func ServeContext(ctx context.Context, listen string, handler http.Handler, options ...ServerOption) error {
	log := log.FromContext(ctx)
	s := http.Server{
		Handler:     handler,
		Addr:        listen,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}
	for _, opt := range options {
		opt(&s)
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
		return s.ListenAndServeTLS("", "")
	} else {
		// http2 support without https
		s.Handler = h2c.NewHandler(s.Handler, &http2.Server{})
		log.Info("starting http server", "listen", listen)
		return s.ListenAndServe()
	}
}

func ServeContextTLS(ctx context.Context, listen string, handler http.Handler, cert, key string) error {
	log := log.FromContext(ctx)
	s := http.Server{
		Handler: handler,
		Addr:    listen,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	tlsconfig, err := NewDynamicTLSConfig(ctx, cert, key)
	if err != nil {
		return err
	}
	if tlsconfig != nil {
		s.TLSConfig = tlsconfig
	}
	go func() {
		<-ctx.Done()
		log.Info("closing http(s) server", "listen", listen)
		s.Close()
	}()
	if cert != "" && key != "" {
		// http2 support with tls enabled
		http2.ConfigureServer(&s, &http2.Server{})
		log.Info("starting https server", "listen", listen)
		return s.ListenAndServeTLS(cert, key)
	} else {
		// http2 support without https
		s.Handler = h2c.NewHandler(s.Handler, &http2.Server{})
		log.Info("starting http server", "listen", listen)
		return s.ListenAndServe()
	}
}

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

func TLSConfig(cafile, certfile, keyfile string) (*tls.Config, error) {
	config := &tls.Config{ClientCAs: x509.NewCertPool()}
	// ca
	if cafile != "" {
		capem, err := os.ReadFile(cafile)
		if err != nil {
			return nil, err
		}
		config.ClientCAs.AppendCertsFromPEM(capem)
	}
	// cert
	certificate, err := tls.LoadX509KeyPair(certfile, keyfile)
	if err != nil {
		return nil, err
	}
	config.Certificates = append(config.Certificates, certificate)
	return config, nil
}

func NewDynamicTLSConfig(ctx context.Context, certfile, keyfile string) (*tls.Config, error) {
	if certfile == "" || keyfile == "" {
		return nil, fmt.Errorf("missing cert or key file")
	}
	dyn := &DynamicCertificate{certFile: certfile, keyFile: keyfile}
	if err := dyn.Reload(ctx); err != nil {
		return nil, err
	}
	go dyn.Watch(ctx)
	return &tls.Config{
		GetConfigForClient: dyn.GetConfigForClient,
		GetCertificate:     dyn.GetCertificate,
	}, nil
}

type DynamicCertificate struct {
	certificate       tls.Certificate
	certFile, keyFile string
	mu                sync.Mutex
}

func (c *DynamicCertificate) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &c.certificate, nil
}

func (c *DynamicCertificate) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &tls.Config{Certificates: []tls.Certificate{c.certificate}}, nil
}

func (c *DynamicCertificate) Watch(ctx context.Context) error {
	log := log.FromContext(ctx)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating fsnotify watcher: %v", err)
	}
	defer w.Close()

	if err := w.Add(c.certFile); err != nil {
		return fmt.Errorf("error adding watch for file %s: %v", c.certFile, err)
	}
	if err := w.Add(c.keyFile); err != nil {
		return fmt.Errorf("error adding watch for file %s: %v", c.keyFile, err)
	}
	for {
		select {
		case e := <-w.Events:
			if e.Has(fsnotify.Remove) && !e.Has(fsnotify.Rename) {
				if err := w.Remove(e.Name); err != nil {
					log.Info("Failed to remove file watch, it may have been deleted", "file", e.Name, "err", err)
				}
				if err := w.Add(e.Name); err != nil {
					return fmt.Errorf("error adding watch for file %s: %v", e.Name, err)
				}
			}
			// reload cert
			if err := c.Reload(ctx); err != nil {
				// if we fail to reload the cert, use the old one, wait for the next event
				log.Error(err, "failed to reload cert", "cert", c.certFile, "key", c.keyFile)
			}
		case err := <-w.Errors:
			return fmt.Errorf("received fsnotify error: %v", err)
		case <-ctx.Done():
			return nil
		}
	}
}

func (c *DynamicCertificate) Reload(ctx context.Context) error {
	log := log.FromContext(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	cert, err := os.ReadFile(c.certFile)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(c.keyFile)
	if err != nil {
		return err
	}
	if len(cert) == 0 || len(key) == 0 {
		return fmt.Errorf("missing content for serving cert")
	}
	certificate, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return err
	}
	c.certificate = certificate
	log.Info("(re)loaded a new cert/key pair", "cert", c.certFile, "key", c.keyFile)
	return nil
}
