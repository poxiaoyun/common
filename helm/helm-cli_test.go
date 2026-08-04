package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	helmregistry "helm.sh/helm/v3/pkg/registry"
)

const (
	testChartName    = "example"
	testChartVersion = "1.2.3"
)

func TestDownloadChartFromHTTPRepository(t *testing.T) {
	metadata, chartData := buildTestChart(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yaml":
			w.Header().Set("Content-Type", "application/x-yaml")
			fmt.Fprintf(w, `apiVersion: v1
entries:
  %s:
    - apiVersion: v2
      name: %s
      version: %s
      type: application
      urls:
        - %s/%s-%s.tgz
generated: "2024-01-01T00:00:00Z"
`, metadata.Name, metadata.Name, metadata.Version, server.URL, metadata.Name, metadata.Version)
		case "/" + metadata.Name + "-" + metadata.Version + ".tgz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(chartData)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	destination := t.TempDir()
	got, err := DownloadChart(context.Background(), server.URL, metadata.Name, metadata.Version, destination, DownloadOptions{})
	if err != nil {
		t.Fatalf("DownloadChart() error = %v", err)
	}
	assertDownloadedChart(t, got, filepath.Join(destination, metadata.Name+"-"+metadata.Version+".tgz"), metadata)
}

func TestDownloadChartFromOCIRegistry(t *testing.T) {
	metadata, chartData := buildTestChart(t)
	server := newOCIRegistry(t, metadata, chartData)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	repository := "oci://" + serverURL.Host + "/charts/" + metadata.Name

	for _, version := range []string{metadata.Version, ""} {
		name := "explicit version"
		if version == "" {
			name = "latest version"
		}
		t.Run(name, func(t *testing.T) {
			destination := t.TempDir()
			got, err := DownloadChart(context.Background(), repository, metadata.Name, version, destination, DownloadOptions{PlainHTTP: true})
			if err != nil {
				t.Fatalf("DownloadChart() error = %v", err)
			}
			assertDownloadedChart(t, got, filepath.Join(destination, metadata.Name+"-"+metadata.Version+".tgz"), metadata)
		})
	}
}

func buildTestChart(t *testing.T) (*chart.Metadata, []byte) {
	t.Helper()
	metadata := &chart.Metadata{
		APIVersion: chart.APIVersionV2,
		Name:       testChartName,
		Version:    testChartVersion,
		Type:       "application",
	}
	archive, err := chartutil.Save(&chart.Chart{Metadata: metadata}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	return metadata, data
}

func newOCIRegistry(t *testing.T, metadata *chart.Metadata, chartData []byte) *httptest.Server {
	t.Helper()
	configData, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	configDescriptor := descriptor(helmregistry.ConfigMediaType, configData)
	chartDescriptor := descriptor(helmregistry.ChartLayerMediaType, chartData)
	manifestData, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDescriptor,
		Layers:    []ocispec.Descriptor{chartDescriptor},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestDescriptor := descriptor(ocispec.MediaTypeImageManifest, manifestData)
	repositoryPath := "/v2/charts/" + metadata.Name

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
			w.WriteHeader(http.StatusOK)
		case repositoryPath + "/tags/list":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "charts/" + metadata.Name,
				"tags": []string{"1.0.0", metadata.Version},
			})
		case repositoryPath + "/manifests/" + metadata.Version,
			repositoryPath + "/manifests/" + manifestDescriptor.Digest.String():
			serveOCIContent(w, r, manifestDescriptor, manifestData)
		case repositoryPath + "/blobs/" + configDescriptor.Digest.String():
			serveOCIContent(w, r, configDescriptor, configData)
		case repositoryPath + "/blobs/" + chartDescriptor.Digest.String():
			serveOCIContent(w, r, chartDescriptor, chartData)
		default:
			http.NotFound(w, r)
		}
	}))
}

func descriptor(mediaType string, data []byte) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}
}

func serveOCIContent(w http.ResponseWriter, r *http.Request, descriptor ocispec.Descriptor, data []byte) {
	w.Header().Set("Content-Type", descriptor.MediaType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", descriptor.Size))
	w.Header().Set("Docker-Content-Digest", descriptor.Digest.String())
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func assertDownloadedChart(t *testing.T, got, want string, metadata *chart.Metadata) {
	t.Helper()
	if got != want {
		t.Fatalf("DownloadChart() = %q, want %q", got, want)
	}
	downloaded, err := loader.Load(got)
	if err != nil {
		t.Fatalf("load downloaded chart: %v", err)
	}
	if downloaded.Metadata.Name != metadata.Name || downloaded.Metadata.Version != metadata.Version {
		t.Fatalf("downloaded chart metadata = %s@%s, want %s@%s", downloaded.Metadata.Name, downloaded.Metadata.Version, metadata.Name, metadata.Version)
	}
}
