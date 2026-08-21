package tls

import (
	cryptotls "crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"
)

const certificateReloadInterval = time.Minute

// ClientOptions configures a TLS client.
type ClientOptions struct {
	CAFile             string
	CertFile           string
	KeyFile            string
	InsecureSkipVerify bool
}

// NewClientConfig builds a client TLS configuration. Client certificates are
// reloaded from disk for new handshakes at most once per minute.
func NewClientConfig(options ClientOptions) (*cryptotls.Config, error) {
	config := &cryptotls.Config{InsecureSkipVerify: options.InsecureSkipVerify}

	if options.CAFile != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate pool: %w", err)
		}
		ca, err := os.ReadFile(options.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file %s: %w", options.CAFile, err)
		}
		if !roots.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("append CA certificates from %s", options.CAFile)
		}
		config.RootCAs = roots
	}

	if options.CertFile != "" || options.KeyFile != "" {
		loader, err := newCertificateLoader(options.CertFile, options.KeyFile, certificateReloadInterval)
		if err != nil {
			return nil, err
		}
		config.GetClientCertificate = func(*cryptotls.CertificateRequestInfo) (*cryptotls.Certificate, error) {
			return loader.Load()
		}
	}

	return config, nil
}

// NewServingConfig builds a server TLS configuration. The serving certificate
// is reloaded from disk for new handshakes at most once per minute.
func NewServingConfig(certFile, keyFile string) (*cryptotls.Config, error) {
	loader, err := newCertificateLoader(certFile, keyFile, certificateReloadInterval)
	if err != nil {
		return nil, err
	}
	return &cryptotls.Config{
		GetCertificate: func(*cryptotls.ClientHelloInfo) (*cryptotls.Certificate, error) {
			return loader.Load()
		},
	}, nil
}

type certificateLoader struct {
	mu             sync.Mutex
	certFile       string
	keyFile        string
	reloadInterval time.Duration
	loadedAt       time.Time
	certificate    *cryptotls.Certificate
	err            error
}

func newCertificateLoader(certFile, keyFile string, reloadInterval time.Duration) (*certificateLoader, error) {
	certificate, err := loadCertificatePair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &certificateLoader{
		certFile:       certFile,
		keyFile:        keyFile,
		reloadInterval: reloadInterval,
		loadedAt:       time.Now(),
		certificate:    certificate,
	}, nil
}

func (l *certificateLoader) Load() (*cryptotls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if time.Since(l.loadedAt) >= l.reloadInterval {
		l.certificate, l.err = loadCertificatePair(l.certFile, l.keyFile)
		l.loadedAt = time.Now()
	}
	return l.certificate, l.err
}

func loadCertificatePair(certFile, keyFile string) (*cryptotls.Certificate, error) {
	certificate, err := cryptotls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load certificate %s and key %s: %w", certFile, keyFile, err)
	}
	return &certificate, nil
}
