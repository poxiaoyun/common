package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	ociv1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
)

type OCICredential struct {
	Host     string `json:"host,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type OCIArtifacts struct {
	Client *regclient.RegClient
}

func NewOCIArtifacts(credentials []OCICredential) (*OCIArtifacts, error) {
	refhosts := make([]*config.Host, 0, len(credentials))
	for _, credential := range credentials {
		host, username, password := credential.Host, credential.Username, credential.Password
		reghost := config.HostNewName(host)
		reghost.User, reghost.Pass = username, password
		reghost.TLS = config.TLSInsecure
		refhosts = append(refhosts, reghost)
	}
	regopts := []reg.Opts{
		reg.WithCertDirs([]string{"/etc/docker/certs.d"}),
	}
	regopts = append(regopts, reg.WithConfigHosts(refhosts))
	regcli := regclient.New(
		regclient.WithDockerCreds(),
		regclient.WithRegOpts(regopts...),
	)
	return &OCIArtifacts{Client: regcli}, nil
}

func (o *OCIArtifacts) Ping(ctx context.Context, image string) error {
	ref, err := ref.New(image)
	if err != nil {
		return err
	}
	_, err = o.Client.Ping(ctx, ref)
	return err
}

func (o *OCIArtifacts) RemoveTag(ctx context.Context, image string, version string) error {
	ref, err := mergeImageVersion(image, version)
	if err != nil {
		return err
	}
	return o.Client.TagDelete(ctx, ref)
}

func (o *OCIArtifacts) DownloadChart(ctx context.Context, image string, version string) ([]*loader.BufferedFile, error) {
	ref, err := mergeImageVersion(image, version)
	if err != nil {
		return nil, err
	}
	mani, err := o.Client.ManifestGet(ctx, ref)
	if err != nil {
		return nil, err
	}
	imager, ok := mani.(manifest.Imager)
	if !ok {
		return nil, fmt.Errorf("not a imager manifest: %s", mani.GetDescriptor().MediaType)
	}
	layers, err := imager.GetLayers()
	if err != nil {
		return nil, err
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("no layers found")
	}
	br, err := o.Client.BlobGet(ctx, ref, layers[0])
	if err != nil {
		return nil, err
	}
	defer br.Close()
	return loader.LoadArchiveFiles(br)
}

func (o *OCIArtifacts) ListTags(ctx context.Context, image string) ([]string, error) {
	ref, err := ref.New(image)
	if err != nil {
		return nil, err
	}
	list, err := o.Client.TagList(ctx, ref)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return list.Tags, nil
}

func (o *OCIArtifacts) ExistsTag(ctx context.Context, image string, version string) (bool, error) {
	ref, err := mergeImageVersion(image, version)
	if err != nil {
		return false, err
	}
	_, err = o.Client.ManifestGet(ctx, ref)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (o *OCIArtifacts) GetChartConfig(ctx context.Context, image string, version string) (*chart.Metadata, error) {
	ref, err := mergeImageVersion(image, version)
	if err != nil {
		return nil, err
	}
	mani, err := o.Client.ManifestGet(ctx, ref)
	if err != nil {
		return nil, err
	}
	imager, ok := mani.(manifest.Imager)
	if !ok {
		return nil, fmt.Errorf("not a imager manifest: %s", mani.GetDescriptor().MediaType)
	}
	config, err := imager.GetConfig()
	if err != nil {
		return nil, err
	}
	metadata := &chart.Metadata{}
	if err := o.DecodeBlob(ctx, ref, config, &chart.Metadata{}); err != nil {
		return nil, err
	}
	return metadata, nil
}

type ImageInfo struct {
	Image     string     `json:"image"`
	Version   string     `json:"version"`
	Platforms []Platform `json:"platforms"`
}

type Platform struct {
	platform.Platform `json:",inline"`
	Annotations       map[string]string `json:"annotations"`
	Size              int64             `json:"size"`
	CreationTime      time.Time         `json:"creationTime"`
}

func (o *OCIArtifacts) DescribeImage(ctx context.Context, image string, version string) (*ImageInfo, error) {
	ref, err := mergeImageVersion(image, version)
	if err != nil {
		return nil, err
	}
	mani, err := o.Client.ManifestGet(ctx, ref)
	if err != nil {
		return nil, err
	}
	imagerToPlatform := func(imager manifest.Imager, desc descriptor.Descriptor) (*Platform, error) {
		p := Platform{
			Annotations: desc.Annotations,
		}
		// get os and arch from config
		if desc.Platform == nil {
			configDesc, err := imager.GetConfig()
			if err != nil {
				return nil, err
			}
			configData := map[string]any{}
			if err := o.DecodeBlob(ctx, ref, configDesc, &configData); err != nil {
				return nil, err
			}
			configos, _ := configData["os"].(string)
			configarch, _ := configData["architecture"].(string)
			desc.Platform = &platform.Platform{OS: configos, Architecture: configarch}

			created, ok := configData["created"].(string)
			if ok {
				p.CreationTime, _ = time.Parse(time.RFC3339, created)
			}
		}
		// calc all layers size
		layers, err := imager.GetLayers()
		if err != nil {
			return nil, err
		}
		allLayersSize := int64(0)
		for _, layer := range layers {
			allLayersSize += layer.Size
		}
		p.Size = allLayersSize

		if p.CreationTime.IsZero() && desc.Annotations != nil && desc.Annotations["org.opencontainers.image.created"] != "" {
			p.CreationTime, _ = time.Parse(time.RFC3339, desc.Annotations["org.opencontainers.image.created"])
		}

		if desc.Platform != nil {
			p.Platform = *desc.Platform
		}
		return &p, nil
	}
	platforms := []Platform{}
	switch manifestval := mani.(type) {
	case manifest.Imager:
		platform, err := imagerToPlatform(manifestval, mani.GetDescriptor())
		if err != nil {
			return nil, err
		}
		platforms = append(platforms, *platform)
	case manifest.Indexer:
		manifests, err := manifestval.GetManifestList()
		if err != nil {
			return nil, err
		}
		for _, manidesc := range manifests {
			maniref := ref
			maniref.Digest = manidesc.Digest.String()
			val, err := o.Client.ManifestGet(ctx, maniref)
			if err != nil {
				return nil, err
			}
			if imager, ok := val.(manifest.Imager); ok {
				platform, err := imagerToPlatform(imager, manidesc)
				if err != nil {
					return nil, err
				}
				platforms = append(platforms, *platform)
			}
		}
	default:
		return nil, fmt.Errorf("not a imager or indexer manifest: %s", mani.GetDescriptor().MediaType)
	}
	return &ImageInfo{Image: image, Version: version, Platforms: platforms}, nil
}

func (o *OCIArtifacts) GetConfig(ctx context.Context, image string, version string, into any) error {
	ref, err := mergeImageVersion(image, version)
	if err != nil {
		return err
	}
	mani, err := o.Client.ManifestGet(ctx, ref)
	if err != nil {
		return err
	}
	imager, ok := mani.(manifest.Imager)
	if ok {
		config, err := imager.GetConfig()
		if err != nil {
			return err
		}
		return o.DecodeBlob(ctx, ref, config, into)
	}
	return fmt.Errorf("not a imager manifest: %s", mani.GetDescriptor().MediaType)
}

func mergeImageVersion(image, version string) (ref.Ref, error) {
	if strings.HasPrefix(version, "sha256:") {
		image += "@" + version
	} else {
		image += ":" + version
	}
	return ref.New(image)
}

func (o *OCIArtifacts) UploadHelmChart(ctx context.Context, image string, expectedChartName string, data io.Reader) error {
	// 20MB limit
	// Chart
	data = io.LimitReader(data, 20*1024*1024)
	chartContent := NewContentDescritor(registry.ChartLayerMediaType)
	chart, err := loader.LoadArchive(io.TeeReader(data, chartContent))
	if err != nil {
		return err
	}
	if expectedChartName != chart.Name() {
		return fmt.Errorf("chart name must be %s", expectedChartName)
	}
	if chart.Metadata.Version == "" {
		return fmt.Errorf("chart version is empty")
	}
	// check must be semver version
	if _, err := semver.Parse(chart.Metadata.Version); err != nil {
		return fmt.Errorf("chart version must be semver version: %w", err)
	}
	// upload
	ref, err := ref.New(image + ":" + chart.Metadata.Version)
	if err != nil {
		return err
	}
	// config
	configContent := NewContentDescritor(registry.ConfigMediaType)
	if err := json.NewEncoder(configContent).Encode(chart.Metadata); err != nil {
		return err
	}
	// manifest
	ocimanifest := ociv1.Manifest{
		Versioned:   ociv1.ManifestSchemaVersion,
		MediaType:   mediatype.OCI1Manifest,
		Config:      configContent.Descriptor(),
		Layers:      []descriptor.Descriptor{chartContent.Descriptor()},
		Annotations: GenerateChartOCIAnnotations(chart.Metadata),
	}
	manifestContent := NewContentDescritor(mediatype.OCI1Manifest)
	if err := json.NewEncoder(manifestContent).Encode(ocimanifest); err != nil {
		return err
	}
	if err := o.PushBlob(ctx, ref, chartContent); err != nil {
		return err
	}
	if err := o.PushBlob(ctx, ref, configContent); err != nil {
		return err
	}
	if err := o.PushManifest(ctx, ref, manifestContent); err != nil {
		return err
	}
	return nil
}

func (c *OCIArtifacts) PushManifest(ctx context.Context, reference ref.Ref, r *ContentDescritor) error {
	manifest, err := manifest.New(manifest.WithDesc(r.Descriptor()), manifest.WithRaw(r.Content()))
	if err != nil {
		return err
	}
	return c.Client.ManifestPut(ctx, reference, manifest)
}

func (c *OCIArtifacts) PushBlob(ctx context.Context, reference ref.Ref, r *ContentDescritor) error {
	reference.Digest = r.Descriptor().Digest.String()
	desc := r.Descriptor()
	if _, err := c.Client.BlobHead(ctx, reference, desc); err != nil {
		if !errors.Is(err, errs.ErrNotFound) {
			return err
		}
	} else {
		return nil
	}
	if _, err := c.Client.BlobPut(ctx, reference, desc, r); err != nil {
		return err
	}
	return nil
}

func (c *OCIArtifacts) DecodeBlob(ctx context.Context, reference ref.Ref, d descriptor.Descriptor, into any) error {
	br, err := c.Client.BlobGet(ctx, reference, d)
	if err != nil {
		return err
	}
	defer br.Close()
	return json.NewDecoder(br).Decode(into)
}

func NewContentDescritor(mediaType string) *ContentDescritor {
	return &ContentDescritor{
		data:      bytes.NewBuffer(nil),
		digester:  digest.Canonical.Digester(),
		mediaType: mediaType,
	}
}

type ContentDescritor struct {
	size      int64
	data      *bytes.Buffer
	digester  digest.Digester
	mediaType string
}

func (c *ContentDescritor) Write(p []byte) (int, error) {
	n, err := c.data.Write(p)
	if err != nil {
		return n, err
	}
	c.size += int64(n)
	c.digester.Hash().Write(p[:n])
	return n, nil
}

func (c *ContentDescritor) Read(p []byte) (int, error) {
	return c.data.Read(p)
}

func (c *ContentDescritor) Content() []byte {
	return c.data.Bytes()
}

func (c *ContentDescritor) Descriptor() descriptor.Descriptor {
	return descriptor.Descriptor{
		MediaType: c.mediaType,
		Digest:    c.digester.Digest(),
		Size:      c.size,
	}
}

func GenerateChartOCIAnnotations(meta *chart.Metadata) map[string]string {
	addToMap := func(inputMap map[string]string, newKey string, newValue string) map[string]string {
		if len(strings.TrimSpace(newValue)) > 0 {
			inputMap[newKey] = newValue
		}
		return inputMap
	}
	chartOCIAnnotations := map[string]string{}
	chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationDescription, meta.Description)
	chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationTitle, meta.Name)
	chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationVersion, meta.Version)
	chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationURL, meta.Home)
	chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationCreated, time.Now().Format(time.RFC3339))
	if len(meta.Sources) > 0 {
		chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationSource, meta.Sources[0])
	}
	if meta.Maintainers != nil && len(meta.Maintainers) > 0 {
		var maintainerSb strings.Builder
		for maintainerIdx, maintainer := range meta.Maintainers {
			if len(maintainer.Name) > 0 {
				maintainerSb.WriteString(maintainer.Name)
			}
			if len(maintainer.Email) > 0 {
				maintainerSb.WriteString(" (")
				maintainerSb.WriteString(maintainer.Email)
				maintainerSb.WriteString(")")
			}
			if maintainerIdx < len(meta.Maintainers)-1 {
				maintainerSb.WriteString(", ")
			}
		}
		chartOCIAnnotations = addToMap(chartOCIAnnotations, ocispec.AnnotationAuthors, maintainerSb.String())
	}
	for chartAnnotationKey, chartAnnotationValue := range meta.Annotations {
		if slices.Contains([]string{ocispec.AnnotationVersion, ocispec.AnnotationTitle}, chartAnnotationKey) {
			continue
		}
		chartOCIAnnotations[chartAnnotationKey] = chartAnnotationValue
	}
	return chartOCIAnnotations
}
