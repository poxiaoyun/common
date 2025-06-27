package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"xiaoshiai.cn/common/log"
)

type DynamicTLSConfigOptions struct {
	CertFile              string `json:"certFile,omitempty"`
	KeyFile               string `json:"keyFile,omitempty"`
	CAFile                string `json:"caFile,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
}

func NewDynamicTLSConfig(ctx context.Context, options *DynamicTLSConfigOptions) (*tls.Config, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("failed to load system cert pool: %v", err)
	}
	dyn := &DynamicCertificate{
		options:  options,
		certPool: certPool,
	}
	if err := dyn.Reload(ctx); err != nil {
		return nil, err
	}
	go dyn.Watch(ctx)

	tlsConfig := &tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: options.InsecureSkipTLSVerify,
	}
	if options.CertFile != "" && options.KeyFile != "" {
		tlsConfig.GetClientCertificate = dyn.GetClientCertificate
		tlsConfig.GetCertificate = dyn.GetCertificate
	}
	return tlsConfig, nil
}

type DynamicCertificate struct {
	options *DynamicTLSConfigOptions

	mu          sync.Mutex
	certificate tls.Certificate
	certPool    *x509.CertPool
}

func (c *DynamicCertificate) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &c.certificate, nil
}

func (c *DynamicCertificate) GetClientCertificate(hello *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &c.certificate, nil
}

func (c *DynamicCertificate) Watch(ctx context.Context) error {
	log := log.FromContext(ctx)

	filesToWatch := []string{}
	if c.options.CAFile != "" {
		filesToWatch = append(filesToWatch, c.options.CAFile)
	}
	if c.options.CertFile != "" {
		filesToWatch = append(filesToWatch, c.options.CertFile)
	}
	if c.options.KeyFile != "" {
		filesToWatch = append(filesToWatch, c.options.KeyFile)
	}
	if len(filesToWatch) == 0 {
		return nil
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating fsnotify watcher: %v", err)
	}
	defer w.Close()

	for _, filename := range filesToWatch {
		if err := w.Add(filename); err != nil {
			return fmt.Errorf("error adding watch for file %s: %v", filename, err)
		}
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
				log.Error(err, "tls failed to reload")
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

	if c.options.CAFile != "" {
		caCert, err := os.ReadFile(c.options.CAFile)
		if err != nil {
			return fmt.Errorf("error reading CA file %s: %v", c.options.CAFile, err)
		}
		if len(caCert) == 0 {
			return fmt.Errorf("missing content for CA cert")
		}
		if !c.certPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to append CA cert from %s", c.options.CAFile)
		}
		log.Info("loaded CA cert", "caFile", c.options.CAFile)
	}
	if c.options.CertFile != "" && c.options.KeyFile != "" {
		cert, err := loadCertKeyPair(c.options.CertFile, c.options.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load cert/key pair: %v", err)
		}
		log.Info("(re)loaded a new cert/key pair", "cert", c.options.CertFile, "key", c.options.KeyFile)
		c.certificate = cert
	}
	return nil
}

func loadCertKeyPair(certFile, keyFile string) (tls.Certificate, error) {
	cert, err := os.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("error reading cert file %s: %v", certFile, err)
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("error reading key file %s: %v", keyFile, err)
	}
	if len(cert) == 0 || len(key) == 0 {
		return tls.Certificate{}, fmt.Errorf("missing content for serving cert")
	}
	return tls.X509KeyPair(cert, key)
}
