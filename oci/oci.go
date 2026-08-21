// Package oci provides registry operations without exposing the underlying
// registry client implementation to callers.
package oci

import (
	"context"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	// ErrNotFound identifies a missing repository, manifest, tag, or blob.
	ErrNotFound = errors.New("OCI content not found")
	// ErrUnauthorized identifies Registry authentication failure.
	ErrUnauthorized = errors.New("OCI Registry authentication failed")
	// ErrForbidden identifies Registry authorization failure.
	ErrForbidden = errors.New("OCI Registry access forbidden")
)

// ClientOptions configures a client for one registry endpoint.
type ClientOptions struct {
	Endpoint              string
	Username              string
	Password              string
	Token                 string
	InsecureSkipTLSVerify bool
	CAFile                string
	CertFile              string
	KeyFile               string
}

// BlobInput is content to upload into a repository blob store.
type BlobInput struct {
	MediaType string
	Content   []byte
}

// ManifestContent is a manifest and the descriptor of its exact encoded bytes.
type ManifestContent struct {
	Descriptor ocispec.Descriptor
	Content    []byte
}

// LayerContent streams a blob and carries its OCI descriptor. The caller must
// close the stream. Size and digest verification errors can be returned while
// reading it.
type LayerContent struct {
	io.ReadCloser
	Descriptor ocispec.Descriptor
}

// CertificateBundle contains registry trust roots and mTLS client identities.
type CertificateBundle struct {
	RootCAs            *x509.CertPool
	ClientCertificates []tls.Certificate
}

// ImageInfo describes the platform variants selected by an image reference.
type ImageInfo struct {
	Image     string     `json:"image"`
	Version   string     `json:"version"`
	Platforms []Platform `json:"platforms"`
}

// Platform is the Common-owned projection of an OCI image platform.
type Platform struct {
	Architecture string            `json:"architecture"`
	OS           string            `json:"os"`
	OSVersion    string            `json:"osVersion,omitempty"`
	OSFeatures   []string          `json:"osFeatures,omitempty"`
	Variant      string            `json:"variant,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Size         int64             `json:"size"`
	CreationTime time.Time         `json:"creationTime"`
}

// Client hides authentication, transport, reference parsing, and registry
// compatibility details behind repository/reference strings.
type Client struct {
	registry      name.Registry
	nameOptions   []name.Option
	remoteOptions []remote.Option
	transport     http.RoundTripper
}

// NewClient constructs a registry client. Endpoints without a scheme are
// secure by default; plain HTTP requires an explicit http:// endpoint.
func NewClient(options ClientOptions) (*Client, error) {
	host, plainHTTP, err := normalizeEndpoint(options.Endpoint)
	if err != nil {
		return nil, err
	}
	if (options.Username == "") != (options.Password == "") {
		return nil, errors.New("registry username and password must be configured together")
	}
	if options.Token != "" && options.Username != "" {
		return nil, errors.New("registry token cannot be combined with username and password")
	}
	if (options.CertFile == "") != (options.KeyFile == "") {
		return nil, errors.New("registry client certificate and key must be configured together")
	}

	nameOptions := []name.Option{name.StrictValidation}
	if plainHTTP {
		nameOptions = append(nameOptions, name.Insecure)
	}
	registry, err := name.NewRegistry(host, nameOptions...)
	if err != nil {
		return nil, fmt.Errorf("invalid registry endpoint: %w", err)
	}

	httpTransport, err := cloneHTTPTransport(remote.DefaultTransport)
	if err != nil {
		return nil, err
	}
	httpTransport.TLSClientConfig = cloneTLSConfig(httpTransport.TLSClientConfig)
	httpTransport.TLSClientConfig.InsecureSkipVerify = options.InsecureSkipTLSVerify // #nosec G402 -- explicit caller option.
	if options.CAFile != "" || options.CertFile != "" {
		certificatesFS, caFile, certFile, keyFile, err := certificateFilesystem(options.CAFile, options.CertFile, options.KeyFile)
		if err != nil {
			return nil, err
		}
		bundle, err := LoadCertificates(httpTransport.TLSClientConfig.RootCAs, certificatesFS, caFile, certFile, keyFile)
		if err != nil {
			return nil, err
		}
		httpTransport.TLSClientConfig.RootCAs = bundle.RootCAs
		httpTransport.TLSClientConfig.Certificates = append(
			httpTransport.TLSClientConfig.Certificates,
			bundle.ClientCertificates...,
		)
	}
	remoteOptions := []remote.Option{remote.WithTransport(httpTransport)}
	if options.Token != "" {
		remoteOptions = append(remoteOptions, remote.WithAuth(&authn.Bearer{Token: options.Token}))
	} else if options.Username != "" {
		remoteOptions = append(remoteOptions, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
			Username: options.Username,
			Password: options.Password,
		})))
	} else {
		remoteOptions = append(remoteOptions, remote.WithAuth(authn.Anonymous))
	}
	return &Client{
		registry:      registry,
		nameOptions:   nameOptions,
		remoteOptions: remoteOptions,
		transport:     httpTransport,
	}, nil
}

// Ping checks that the configured endpoint implements the registry v2 API.
func (r *Client) Ping(ctx context.Context) error {
	_, err := transport.Ping(ctx, r.registry, r.transport)
	return mapError(err)
}

// PushBlob uploads content if it is not already present and returns its OCI descriptor.
func (r *Client) PushBlob(ctx context.Context, repository string, input BlobInput) (ocispec.Descriptor, error) {
	if input.MediaType == "" {
		return ocispec.Descriptor{}, errors.New("blob media type is required")
	}
	repo, err := r.repository(repository)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	layer := static.NewLayer(input.Content, types.MediaType(input.MediaType))
	pusher, err := remote.NewPusher(r.options()...)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	if err := pusher.Upload(ctx, repo, layer); err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}
	return descriptorFor(input.MediaType, input.Content), nil
}

// PutManifest assigns exact manifest bytes to a tag or digest reference.
func (r *Client) PutManifest(ctx context.Context, repository, reference, mediaType string, content []byte) (ocispec.Descriptor, error) {
	if mediaType == "" {
		return ocispec.Descriptor{}, errors.New("manifest media type is required")
	}
	ref, err := r.reference(repository, reference)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	pusher, err := remote.NewPusher(r.options()...)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	manifest := rawManifest{content: content, mediaType: types.MediaType(mediaType)}
	if err := pusher.Put(ctx, ref, manifest); err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}
	return descriptorFor(mediaType, content), nil
}

// HeadManifest returns manifest metadata without downloading the manifest body.
func (r *Client) HeadManifest(ctx context.Context, repository, reference string) (ocispec.Descriptor, error) {
	ref, err := r.reference(repository, reference)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	descriptor, err := remote.Head(ref, r.options(remote.WithContext(ctx))...)
	if err != nil {
		return ocispec.Descriptor{}, mapError(err)
	}
	return fromGoogleDescriptor(*descriptor), nil
}

// GetManifest returns exact manifest bytes and their descriptor.
func (r *Client) GetManifest(ctx context.Context, repository, reference string) (ManifestContent, error) {
	ref, err := r.reference(repository, reference)
	if err != nil {
		return ManifestContent{}, err
	}
	descriptor, err := remote.Get(ref, r.options(remote.WithContext(ctx))...)
	if err != nil {
		return ManifestContent{}, mapError(err)
	}
	return ManifestContent{
		Descriptor: fromGoogleDescriptor(descriptor.Descriptor),
		Content:    append([]byte(nil), descriptor.Manifest...),
	}, nil
}

// DownloadLayer opens the blob identified by descriptor from repository.
func (r *Client) DownloadLayer(ctx context.Context, repository string, descriptor ocispec.Descriptor) (*LayerContent, error) {
	if err := descriptor.Digest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid layer digest: %w", err)
	}
	if descriptor.Size < 0 {
		return nil, errors.New("layer size must not be negative")
	}
	repo, err := r.repository(repository)
	if err != nil {
		return nil, err
	}
	layer, err := remote.Layer(repo.Digest(descriptor.Digest.String()), r.options(remote.WithContext(ctx))...)
	if err != nil {
		return nil, mapError(err)
	}
	reader, err := layer.Compressed()
	if err != nil {
		return nil, mapError(err)
	}
	verified, err := newVerifiedReadCloser(reader, descriptor)
	if err != nil {
		reader.Close()
		return nil, err
	}
	return &LayerContent{
		ReadCloser: verified,
		Descriptor: cloneDescriptor(descriptor),
	}, nil
}

// ListTags returns all tags in repository. A missing repository has no tags.
func (r *Client) ListTags(ctx context.Context, repository string) ([]string, error) {
	repo, err := r.repository(repository)
	if err != nil {
		return nil, err
	}
	tags, err := remote.List(repo, r.options(remote.WithContext(ctx))...)
	if errors.Is(mapError(err), ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, mapError(err)
	}
	return tags, nil
}

// ExistsTag reports whether reference exists in repository.
func (r *Client) ExistsTag(ctx context.Context, repository, reference string) (bool, error) {
	_, err := r.HeadManifest(ctx, repository, reference)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// RemoveTag removes a tag without deleting a manifest shared by other tags.
func (r *Client) RemoveTag(ctx context.Context, repository, tag string) error {
	repo, err := r.repository(repository)
	if err != nil {
		return err
	}
	ref, err := r.tagReference(repo, tag)
	if err != nil {
		return err
	}
	pusher, err := remote.NewPusher(r.options()...)
	if err != nil {
		return err
	}
	if err := pusher.Delete(ctx, ref); err == nil {
		return nil
	}

	current, err := r.GetManifest(ctx, repository, tag)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	empty := []byte("{}")
	emptyDigest := digest.FromBytes(empty)
	config, err := json.Marshal(struct {
		Created      time.Time `json:"created"`
		Architecture string    `json:"architecture"`
		OS           string    `json:"os"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
		RootFS struct {
			Type    string          `json:"type"`
			DiffIDs []digest.Digest `json:"diff_ids"`
		} `json:"rootfs"`
	}{
		Created:      now,
		Architecture: "amd64",
		OS:           "linux",
		Config: struct {
			Labels map[string]string `json:"Labels"`
		}{Labels: map[string]string{"delete-tag": tag, "delete-date": now.Format(time.RFC3339Nano)}},
		RootFS: struct {
			Type    string          `json:"type"`
			DiffIDs []digest.Digest `json:"diff_ids"`
		}{Type: "layers", DiffIDs: []digest.Digest{emptyDigest}},
	})
	if err != nil {
		return err
	}

	manifestMediaType := string(types.OCIManifestSchema1)
	configMediaType := string(types.OCIConfigJSON)
	layerMediaType := string(types.OCILayer)
	if types.MediaType(current.Descriptor.MediaType) == types.DockerManifestSchema2 ||
		types.MediaType(current.Descriptor.MediaType) == types.DockerManifestList {
		manifestMediaType = string(types.DockerManifestSchema2)
		configMediaType = string(types.DockerConfigJSON)
		layerMediaType = string(types.DockerLayer)
	}
	emptyDescriptor, err := r.PushBlob(ctx, repository, BlobInput{MediaType: layerMediaType, Content: empty})
	if err != nil {
		return err
	}
	configDescriptor, err := r.PushBlob(ctx, repository, BlobInput{MediaType: configMediaType, Content: config})
	if err != nil {
		return err
	}
	dummy, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: manifestMediaType,
		Config:    configDescriptor,
		Layers:    []ocispec.Descriptor{emptyDescriptor},
	})
	if err != nil {
		return err
	}
	dummyDescriptor, err := r.PutManifest(ctx, repository, tag, manifestMediaType, dummy)
	if err != nil {
		return fmt.Errorf("replace tag %q with deletion manifest: %w", tag, err)
	}
	if err := pusher.Delete(ctx, repo.Digest(dummyDescriptor.Digest.String())); err != nil {
		return fmt.Errorf("delete replacement manifest for tag %q: %w", tag, mapError(err))
	}
	return nil
}

// GetConfig decodes the image manifest's config blob into into.
func (r *Client) GetConfig(ctx context.Context, repository, reference string, into any) error {
	manifest, err := r.GetManifest(ctx, repository, reference)
	if err != nil {
		return err
	}
	parsed, err := parseImageManifest(manifest)
	if err != nil {
		return err
	}
	content, err := r.DownloadLayer(ctx, repository, parsed.Config)
	if err != nil {
		return err
	}
	defer content.Close()
	return json.NewDecoder(content).Decode(into)
}

// DescribeImage resolves an image manifest or index into platform summaries.
func (r *Client) DescribeImage(ctx context.Context, repository, reference string) (*ImageInfo, error) {
	manifest, err := r.GetManifest(ctx, repository, reference)
	if err != nil {
		return nil, err
	}
	platforms := []Platform{}
	switch types.MediaType(manifest.Descriptor.MediaType) {
	case types.OCIManifestSchema1, types.DockerManifestSchema2:
		platform, err := r.describePlatform(ctx, repository, manifest, nil)
		if err != nil {
			return nil, err
		}
		platforms = append(platforms, platform)
	case types.OCIImageIndex, types.DockerManifestList:
		var index ocispec.Index
		if err := json.Unmarshal(manifest.Content, &index); err != nil {
			return nil, fmt.Errorf("decode image index: %w", err)
		}
		for _, childDescriptor := range index.Manifests {
			child, err := r.GetManifest(ctx, repository, childDescriptor.Digest.String())
			if err != nil {
				return nil, err
			}
			if !types.MediaType(child.Descriptor.MediaType).IsImage() {
				continue
			}
			platform, err := r.describePlatform(ctx, repository, child, &childDescriptor)
			if err != nil {
				return nil, err
			}
			platforms = append(platforms, platform)
		}
	default:
		return nil, fmt.Errorf("manifest has unsupported image media type %q", manifest.Descriptor.MediaType)
	}
	return &ImageInfo{Image: repository, Version: reference, Platforms: platforms}, nil
}

func (r *Client) describePlatform(ctx context.Context, repository string, content ManifestContent, indexDescriptor *ocispec.Descriptor) (Platform, error) {
	manifest, err := parseImageManifest(content)
	if err != nil {
		return Platform{}, err
	}
	configContent, err := r.DownloadLayer(ctx, repository, manifest.Config)
	if err != nil {
		return Platform{}, err
	}
	defer configContent.Close()
	var config struct {
		Architecture string    `json:"architecture"`
		OS           string    `json:"os"`
		OSVersion    string    `json:"os.version"`
		OSFeatures   []string  `json:"os.features"`
		Variant      string    `json:"variant"`
		Created      time.Time `json:"created"`
	}
	if err := json.NewDecoder(configContent).Decode(&config); err != nil {
		return Platform{}, fmt.Errorf("decode image config: %w", err)
	}
	platform := Platform{
		Architecture: config.Architecture,
		OS:           config.OS,
		OSVersion:    config.OSVersion,
		OSFeatures:   append([]string(nil), config.OSFeatures...),
		Variant:      config.Variant,
		Annotations:  cloneMap(content.Descriptor.Annotations),
		CreationTime: config.Created,
	}
	if indexDescriptor != nil {
		platform.Annotations = cloneMap(indexDescriptor.Annotations)
		if indexDescriptor.Platform != nil {
			platform.Architecture = indexDescriptor.Platform.Architecture
			platform.OS = indexDescriptor.Platform.OS
			platform.OSVersion = indexDescriptor.Platform.OSVersion
			platform.OSFeatures = append([]string(nil), indexDescriptor.Platform.OSFeatures...)
			platform.Variant = indexDescriptor.Platform.Variant
		}
	}
	for _, layer := range manifest.Layers {
		platform.Size += layer.Size
	}
	if platform.CreationTime.IsZero() {
		if value := platform.Annotations[ocispec.AnnotationCreated]; value != "" {
			platform.CreationTime, _ = time.Parse(time.RFC3339, value)
		}
	}
	return platform, nil
}

func parseImageManifest(content ManifestContent) (ocispec.Manifest, error) {
	if !types.MediaType(content.Descriptor.MediaType).IsImage() {
		return ocispec.Manifest{}, fmt.Errorf("manifest has unsupported image media type %q", content.Descriptor.MediaType)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(content.Content, &manifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("decode image manifest: %w", err)
	}
	return manifest, nil
}

func (r *Client) repository(repository string) (name.Repository, error) {
	repository = strings.Trim(repository, "/")
	if repository == "" {
		return name.Repository{}, errors.New("repository is required")
	}
	return name.NewRepository(r.registry.RegistryStr()+"/"+repository, r.nameOptions...)
}

func (r *Client) reference(repository, reference string) (name.Reference, error) {
	repo, err := r.repository(repository)
	if err != nil {
		return nil, err
	}
	if parsed, err := digest.Parse(reference); err == nil {
		return repo.Digest(parsed.String()), nil
	}
	return r.tagReference(repo, reference)
}

func (r *Client) tagReference(repo name.Repository, tag string) (name.Tag, error) {
	if tag == "" {
		return name.Tag{}, errors.New("tag is required")
	}
	return name.NewTag(repo.Name()+":"+tag, r.nameOptions...)
}

func (r *Client) options(extra ...remote.Option) []remote.Option {
	options := make([]remote.Option, 0, len(r.remoteOptions)+len(extra))
	options = append(options, r.remoteOptions...)
	return append(options, extra...)
}

func normalizeEndpoint(endpoint string) (host string, plainHTTP bool, err error) {
	endpoint = strings.TrimSpace(strings.TrimRight(endpoint, "/"))
	if endpoint == "" {
		return "", false, errors.New("registry endpoint is required")
	}
	if !strings.Contains(endpoint, "://") {
		if strings.Contains(endpoint, "/") {
			return "", false, errors.New("registry endpoint must not contain a path")
		}
		return endpoint, false, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return "", false, errors.New("invalid registry endpoint")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false, errors.New("registry endpoint scheme must be http or https")
	}
	return parsed.Host, parsed.Scheme == "http", nil
}

func cloneTLSConfig(config *tls.Config) *tls.Config {
	if config == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return config.Clone()
}

func cloneHTTPTransport(roundTripper http.RoundTripper) (*http.Transport, error) {
	httpTransport, ok := roundTripper.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("registry transport %T cannot be cloned", roundTripper)
	}
	return httpTransport.Clone(), nil
}

func certificateFilesystem(caFile, certFile, keyFile string) (fs.FS, string, string, string, error) {
	files := []*string{&caFile, &certFile, &keyFile}
	for _, file := range files {
		if *file == "" {
			continue
		}
		absolute, err := filepath.Abs(*file)
		if err != nil {
			return nil, "", "", "", fmt.Errorf("resolve registry certificate path %q: %w", *file, err)
		}
		*file = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(absolute)), "/")
		if !fs.ValidPath(*file) {
			return nil, "", "", "", fmt.Errorf("invalid registry certificate path %q", absolute)
		}
	}
	return os.DirFS("/"), caFile, certFile, keyFile, nil
}

// LoadCertificates loads an optional CA bundle and optional mTLS client
// certificate/key pair from filesystem. File names are explicit because OCI
// Distribution does not define a client trust-store discovery layout.
func LoadCertificates(existing *x509.CertPool, filesystem fs.FS, caFile, certFile, keyFile string) (CertificateBundle, error) {
	if filesystem == nil {
		return CertificateBundle{}, errors.New("certificate filesystem is required")
	}
	if (certFile == "") != (keyFile == "") {
		return CertificateBundle{}, errors.New("registry client certificate and key must be configured together")
	}
	for _, filename := range []string{caFile, certFile, keyFile} {
		if filename != "" && !fs.ValidPath(filename) {
			return CertificateBundle{}, fmt.Errorf("certificate path %q must be relative to the filesystem", filename)
		}
	}
	var pool *x509.CertPool
	if existing != nil {
		pool = existing.Clone()
	} else {
		var err error
		pool, err = x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
	}
	bundle := CertificateBundle{RootCAs: pool}
	if caFile != "" {
		caPEM, err := fs.ReadFile(filesystem, caFile)
		if err != nil {
			return CertificateBundle{}, fmt.Errorf("read registry CA certificate %q: %w", caFile, err)
		}
		if !pool.AppendCertsFromPEM(caPEM) {
			return CertificateBundle{}, fmt.Errorf("registry CA certificate %q contains no valid PEM certificate", caFile)
		}
	}
	if certFile != "" {
		certificatePEM, err := fs.ReadFile(filesystem, certFile)
		if err != nil {
			return CertificateBundle{}, fmt.Errorf("read registry client certificate %q: %w", certFile, err)
		}
		keyPEM, err := fs.ReadFile(filesystem, keyFile)
		if err != nil {
			return CertificateBundle{}, fmt.Errorf("read registry client key %q: %w", keyFile, err)
		}
		certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
		if err != nil {
			return CertificateBundle{}, fmt.Errorf("load registry client certificate %q: %w", certFile, err)
		}
		bundle.ClientCertificates = append(bundle.ClientCertificates, certificate)
	}
	return bundle, nil
}

func descriptorFor(mediaType string, content []byte) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(content),
		Size:      int64(len(content)),
	}
}

func fromGoogleDescriptor(descriptor v1.Descriptor) ocispec.Descriptor {
	result := ocispec.Descriptor{
		MediaType:    string(descriptor.MediaType),
		Digest:       digest.Digest(descriptor.Digest.String()),
		Size:         descriptor.Size,
		URLs:         append([]string(nil), descriptor.URLs...),
		Annotations:  cloneMap(descriptor.Annotations),
		Data:         append([]byte(nil), descriptor.Data...),
		ArtifactType: descriptor.ArtifactType,
	}
	if descriptor.Platform != nil {
		result.Platform = &ocispec.Platform{
			Architecture: descriptor.Platform.Architecture,
			OS:           descriptor.Platform.OS,
			OSVersion:    descriptor.Platform.OSVersion,
			OSFeatures:   append([]string(nil), descriptor.Platform.OSFeatures...),
			Variant:      descriptor.Platform.Variant,
		}
	}
	return result
}

func cloneDescriptor(descriptor ocispec.Descriptor) ocispec.Descriptor {
	descriptor.URLs = append([]string(nil), descriptor.URLs...)
	descriptor.Annotations = cloneMap(descriptor.Annotations)
	descriptor.Data = append([]byte(nil), descriptor.Data...)
	if descriptor.Platform != nil {
		platform := *descriptor.Platform
		platform.OSFeatures = append([]string(nil), descriptor.Platform.OSFeatures...)
		descriptor.Platform = &platform
	}
	return descriptor
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	maps.Copy(cloned, values)
	return cloned
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var transportError *transport.Error
	if errors.As(err, &transportError) {
		switch transportError.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %w", ErrNotFound, err)
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %w", ErrUnauthorized, err)
		case http.StatusForbidden:
			return fmt.Errorf("%w: %w", ErrForbidden, err)
		}
	}
	return err
}

type rawManifest struct {
	content   []byte
	mediaType types.MediaType
}

func (m rawManifest) RawManifest() ([]byte, error) {
	return m.content, nil
}

func (m rawManifest) MediaType() (types.MediaType, error) {
	return m.mediaType, nil
}

type verifiedReadCloser struct {
	reader     io.ReadCloser
	descriptor ocispec.Descriptor
	digester   digest.Digester
	size       int64
	done       bool
}

func newVerifiedReadCloser(reader io.ReadCloser, descriptor ocispec.Descriptor) (*verifiedReadCloser, error) {
	algorithm := descriptor.Digest.Algorithm()
	if !algorithm.Available() {
		return nil, fmt.Errorf("unsupported layer digest algorithm %q", algorithm)
	}
	return &verifiedReadCloser{
		reader:     reader,
		descriptor: descriptor,
		digester:   algorithm.Digester(),
	}, nil
}

func (r *verifiedReadCloser) Read(buffer []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n, err := r.reader.Read(buffer)
	if n > 0 {
		r.size += int64(n)
		_, _ = r.digester.Hash().Write(buffer[:n])
	}
	if err == nil {
		return n, nil
	}
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	r.done = true
	if r.size != r.descriptor.Size {
		return n, fmt.Errorf("downloaded layer size mismatch: expected %d, got %d", r.descriptor.Size, r.size)
	}
	if actual := r.digester.Digest(); actual != r.descriptor.Digest {
		return n, fmt.Errorf("downloaded layer digest mismatch: expected %s, got %s", r.descriptor.Digest, actual)
	}
	return n, io.EOF
}

func (r *verifiedReadCloser) Close() error {
	return r.reader.Close()
}
