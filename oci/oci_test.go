package oci

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestRegistryArtifactRoundTrip(t *testing.T) {
	client, server := testRegistry(t)
	defer server.Close()
	ctx := context.Background()
	repository := "tenant/artifacts/demo"

	config, err := client.PushBlob(ctx, repository, BlobInput{
		MediaType: "application/vnd.example.config.v1+json",
		Content:   []byte(`{"name":"demo"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	layerData := []byte("layer content")
	layer, err := client.PushBlob(ctx, repository, BlobInput{
		MediaType: "application/vnd.example.layer.v1+octet-stream",
		Content:   layerData,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestData, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    []ocispec.Descriptor{layer},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := client.PutManifest(ctx, repository, "1.0.0", ocispec.MediaTypeImageManifest, manifestData)
	if err != nil {
		t.Fatal(err)
	}

	head, err := client.HeadManifest(ctx, repository, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if head.Digest != manifest.Digest || head.Size != manifest.Size || head.MediaType != manifest.MediaType {
		t.Fatalf("head descriptor = %#v, want %#v", head, manifest)
	}
	gotManifest, err := client.GetManifest(ctx, repository, manifest.Digest.String())
	if err != nil {
		t.Fatal(err)
	}
	if gotManifest.Descriptor.Digest != manifest.Digest || !bytes.Equal(gotManifest.Content, manifestData) {
		t.Fatalf("manifest = %#v %q", gotManifest.Descriptor, gotManifest.Content)
	}

	content, err := client.DownloadLayer(ctx, repository, layer)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	gotLayer, err := io.ReadAll(content)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotLayer, layerData) || content.Descriptor.Digest != layer.Digest ||
		content.Descriptor.Size != layer.Size || content.Descriptor.MediaType != layer.MediaType {
		t.Fatalf("layer = %q %#v, want %q %#v", gotLayer, content.Descriptor, layerData, layer)
	}

	tags, err := client.ListTags(ctx, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "1.0.0" {
		t.Fatalf("tags = %v", tags)
	}
	exists, err := client.ExistsTag(ctx, repository, "1.0.0")
	if err != nil || !exists {
		t.Fatalf("ExistsTag() = %v, %v", exists, err)
	}
}

func TestRegistryNotFound(t *testing.T) {
	client, server := testRegistry(t)
	defer server.Close()

	_, err := client.GetManifest(context.Background(), "missing/repository", "latest")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetManifest() error = %v, want ErrNotFound", err)
	}
	exists, err := client.ExistsTag(context.Background(), "missing/repository", "latest")
	if err != nil || exists {
		t.Fatalf("ExistsTag() = %v, %v", exists, err)
	}
	tags, err := client.ListTags(context.Background(), "missing/repository")
	if err != nil || tags != nil {
		t.Fatalf("ListTags() = %v, %v", tags, err)
	}
}

func TestDownloadLayerVerifiesDescriptor(t *testing.T) {
	client, server := testRegistry(t)
	defer server.Close()
	ctx := context.Background()
	repository := "verification/demo"
	descriptor, err := client.PushBlob(ctx, repository, BlobInput{
		MediaType: string(types.OCILayer),
		Content:   []byte("content"),
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor.Size++
	content, err := client.DownloadLayer(ctx, repository, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	if _, err := io.ReadAll(content); err == nil {
		t.Fatal("DownloadLayer() accepted a mismatched descriptor size")
	}
}

func TestRemoveTagPreservesSharedManifest(t *testing.T) {
	client, server := testRegistry(t)
	defer server.Close()
	ctx := context.Background()
	repository := "tags/demo"
	config, err := client.PushBlob(ctx, repository, BlobInput{MediaType: string(types.OCIConfigJSON), Content: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	manifestData, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    []ocispec.Descriptor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"keep", "remove"} {
		if _, err := client.PutManifest(ctx, repository, tag, ocispec.MediaTypeImageManifest, manifestData); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.RemoveTag(ctx, repository, "remove"); err != nil {
		t.Fatal(err)
	}
	if exists, err := client.ExistsTag(ctx, repository, "remove"); err != nil || exists {
		t.Fatalf("removed tag exists = %v, %v", exists, err)
	}
	if exists, err := client.ExistsTag(ctx, repository, "keep"); err != nil || !exists {
		t.Fatalf("shared tag exists = %v, %v", exists, err)
	}
}

func TestDescribeImageAndGetConfig(t *testing.T) {
	client, server := testRegistry(t)
	defer server.Close()
	ctx := context.Background()
	repository := "images/demo"
	created := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	configData, err := json.Marshal(map[string]any{
		"architecture": "amd64",
		"os":           "linux",
		"created":      created,
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := client.PushBlob(ctx, repository, BlobInput{MediaType: string(types.OCIConfigJSON), Content: configData})
	if err != nil {
		t.Fatal(err)
	}
	layer, err := client.PushBlob(ctx, repository, BlobInput{MediaType: string(types.OCILayer), Content: []byte("layer")})
	if err != nil {
		t.Fatal(err)
	}
	manifestData, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    []ocispec.Descriptor{layer},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutManifest(ctx, repository, "latest", ocispec.MediaTypeImageManifest, manifestData); err != nil {
		t.Fatal(err)
	}
	info, err := client.DescribeImage(ctx, repository, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Platforms) != 1 || info.Platforms[0].OS != "linux" || info.Platforms[0].Architecture != "amd64" || info.Platforms[0].Size != layer.Size {
		t.Fatalf("image info = %#v", info)
	}
	var decoded map[string]any
	if err := client.GetConfig(ctx, repository, "latest", &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["os"] != "linux" {
		t.Fatalf("config = %#v", decoded)
	}
}

func TestRegistryTransportOptions(t *testing.T) {
	httpServer := httptest.NewServer(gcrregistry.New())
	defer httpServer.Close()
	httpClient, err := NewClient(ClientOptions{Endpoint: httpServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := httpClient.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}

	tlsServer := httptest.NewTLSServer(gcrregistry.New())
	defer tlsServer.Close()
	if client, err := NewClient(ClientOptions{Endpoint: tlsServer.URL}); err != nil {
		t.Fatal(err)
	} else if err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() trusted an unknown TLS certificate")
	}
	insecureClient, err := NewClient(ClientOptions{Endpoint: tlsServer.URL, InsecureSkipTLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := insecureClient.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryCredentialValidation(t *testing.T) {
	for _, options := range []ClientOptions{
		{Endpoint: "registry.example.com", Username: "user"},
		{Endpoint: "registry.example.com", Password: "password"},
		{Endpoint: "registry.example.com", Username: "user", Password: "password", Token: "token"},
	} {
		if _, err := NewClient(options); err == nil {
			t.Fatalf("NewClient(%#v) accepted ambiguous credentials", options)
		}
	}
	if _, err := NewClient(ClientOptions{Endpoint: "registry.example.com", Token: "token"}); err != nil {
		t.Fatalf("NewClient(token): %v", err)
	}
	if _, err := NewClient(ClientOptions{Endpoint: "registry.example.com"}); err != nil {
		t.Fatalf("NewClient(anonymous): %v", err)
	}
}

func TestRegistryMapsAccessErrors(t *testing.T) {
	for _, test := range []struct {
		status int
		want   error
	}{
		{status: http.StatusUnauthorized, want: ErrUnauthorized},
		{status: http.StatusForbidden, want: ErrForbidden},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet && request.URL.Path == "/v2/" {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			client, err := NewClient(ClientOptions{Endpoint: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.HeadManifest(t.Context(), "team/app", "v1")
			if !errors.Is(err, test.want) {
				t.Fatalf("HeadManifest() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRegistryRejectsInvalidCertificate(t *testing.T) {
	filesystem := fstest.MapFS{
		"certs/registry.crt": &fstest.MapFile{Data: []byte("not PEM")},
	}
	if _, err := LoadCertificates(nil, filesystem, "certs/registry.crt", "", ""); err == nil {
		t.Fatal("LoadCertificates() accepted an invalid certificate")
	}
}

func TestLoadCertificatesLoadsCAAndClientPair(t *testing.T) {
	server := httptest.NewTLSServer(gcrregistry.New())
	defer server.Close()
	certificate := server.TLS.Certificates[0]
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
	keyData, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyData})
	filesystem := fstest.MapFS{
		"certs/ca.crt":      &fstest.MapFile{Data: certificatePEM},
		"certs/client.cert": &fstest.MapFile{Data: certificatePEM},
		"certs/client.key":  &fstest.MapFile{Data: keyPEM},
	}
	bundle, err := LoadCertificates(nil, filesystem, "certs/ca.crt", "certs/client.cert", "certs/client.key")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.RootCAs == nil || len(bundle.RootCAs.Subjects()) == 0 {
		t.Fatal("LoadCertificates() did not load the CA certificate")
	}
	if len(bundle.ClientCertificates) != 1 {
		t.Fatalf("client certificates = %d, want 1", len(bundle.ClientCertificates))
	}
}

func TestLoadCertificatesRejectsUnpairedClientCertificate(t *testing.T) {
	if _, err := LoadCertificates(nil, fstest.MapFS{}, "", "certs/client.cert", ""); err == nil {
		t.Fatal("LoadCertificates() accepted a client certificate without a key")
	}
}

func testRegistry(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(gcrregistry.New())
	client, err := NewClient(ClientOptions{Endpoint: server.URL})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return client, server
}
