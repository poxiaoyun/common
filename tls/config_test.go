package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestNewClientConfigUsesSystemRootsByDefault(t *testing.T) {
	config, err := NewClientConfig(ClientOptions{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if config.RootCAs != nil {
		t.Fatal("RootCAs is non-nil, want the system roots")
	}
	if !config.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify is false")
	}
}

func TestNewClientConfigRequiresCertificateAndKeyTogether(t *testing.T) {
	_, err := NewClientConfig(ClientOptions{CertFile: "client.crt"})
	if err == nil {
		t.Fatal("NewClientConfig returned no error")
	}
}

func TestNewClientConfigAppendsCAFileToSystemRoots(t *testing.T) {
	certPEM, _, certificate := newCertificatePair(t, 1)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := NewClientConfig(ClientOptions{CAFile: caFile})
	if err != nil {
		t.Fatal(err)
	}
	if config.RootCAs == nil {
		t.Fatal("RootCAs is nil")
	}
	if !slices.ContainsFunc(config.RootCAs.Subjects(), func(subject []byte) bool {
		return slices.Equal(subject, certificate.RawSubject)
	}) {
		t.Fatal("RootCAs does not contain the configured CA")
	}
}

func TestCertificateLoaderReloadsAndRecovers(t *testing.T) {
	directory := t.TempDir()
	certFile := filepath.Join(directory, "client.crt")
	keyFile := filepath.Join(directory, "client.key")
	writeCertificatePair(t, certFile, keyFile, 1)

	loader, err := newCertificateLoader(certFile, keyFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	first, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if serialNumber(t, first) != 1 {
		t.Fatalf("certificate serial = %d, want 1", serialNumber(t, first))
	}

	writeCertificatePair(t, certFile, keyFile, 2)
	second, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if serialNumber(t, second) != 2 {
		t.Fatalf("certificate serial = %d, want 2", serialNumber(t, second))
	}

	if err := os.WriteFile(certFile, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(); err == nil {
		t.Fatal("Load returned no error for an invalid certificate")
	}

	writeCertificatePair(t, certFile, keyFile, 3)
	third, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if serialNumber(t, third) != 3 {
		t.Fatalf("certificate serial = %d, want 3", serialNumber(t, third))
	}
}

func TestCertificateLoaderCachesCertificateUntilReloadInterval(t *testing.T) {
	directory := t.TempDir()
	certFile := filepath.Join(directory, "client.crt")
	keyFile := filepath.Join(directory, "client.key")
	writeCertificatePair(t, certFile, keyFile, 1)

	loader, err := newCertificateLoader(certFile, keyFile, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	writeCertificatePair(t, certFile, keyFile, 2)
	certificate, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if serialNumber(t, certificate) != 1 {
		t.Fatalf("certificate serial = %d, want cached serial 1", serialNumber(t, certificate))
	}
}

func TestClientAndServingConfigsUseCertificateLoader(t *testing.T) {
	directory := t.TempDir()
	certFile := filepath.Join(directory, "tls.crt")
	keyFile := filepath.Join(directory, "tls.key")
	writeCertificatePair(t, certFile, keyFile, 1)

	clientConfig, err := NewClientConfig(ClientOptions{CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatal(err)
	}
	if clientConfig.GetClientCertificate == nil {
		t.Fatal("GetClientCertificate is nil")
	}
	if _, err := clientConfig.GetClientCertificate(nil); err != nil {
		t.Fatal(err)
	}

	servingConfig, err := NewServingConfig(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if servingConfig.GetCertificate == nil {
		t.Fatal("GetCertificate is nil")
	}
	if _, err := servingConfig.GetCertificate(nil); err != nil {
		t.Fatal(err)
	}
}

func writeCertificatePair(t *testing.T, certFile, keyFile string, serial int64) {
	t.Helper()
	certPEM, keyPEM, _ := newCertificatePair(t, serial)
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newCertificatePair(t *testing.T, serial int64) ([]byte, []byte, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyDER}), certificate
}

func serialNumber(t *testing.T, certificate *cryptotls.Certificate) int64 {
	t.Helper()
	parsed, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return parsed.SerialNumber.Int64()
}
