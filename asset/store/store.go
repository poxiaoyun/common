// Package store implements asset.Service with a common Store.
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/asset"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
	commonstore "xiaoshiai.cn/common/store"
)

var _ asset.Service = (*Service)(nil)

// DefaultMaxBytes is the default Store-backed upload size limit.
const DefaultMaxBytes = 8 * 1024 * 1024

// Policy controls content accepted by the Store-backed service.
type Policy struct {
	MaxBytes int64
	// AllowedMediaTypes accepts exact media types and wildcard media ranges such
	// as image/*. An empty list accepts every media type.
	AllowedMediaTypes []string
}

// PolicyKey selects the global policy when empty, a kind policy when only Kind
// is set, or a named asset policy when both Kind and Name are set.
type PolicyKey struct {
	Kind string
	Name string
}

// Policies contains Store-backed upload policies from least to most specific.
type Policies map[PolicyKey]Policy

// Asset is the Store persistence representation of an asset.
type Asset struct {
	commonstore.ObjectMeta `json:",inline"`
	Kind                   string            `json:"kind,omitempty"`
	Owner                  string            `json:"owner,omitempty"`
	UpdationTimestamp      meta.Time         `json:"updationTimestamp,omitempty"`
	ContentType            string            `json:"contentType,omitempty"`
	FileName               string            `json:"fileName,omitempty"`
	ModTime                meta.Time         `json:"modTime,omitempty"`
	ContentLength          int64             `json:"contentLength,omitempty"`
	ETag                   string            `json:"etag,omitempty"`
	Digest                 string            `json:"digest,omitempty"`
	Content                string            `json:"content,omitempty"`
	Link                   *asset.Link       `json:"link,omitempty"`
	Version                int64             `json:"version,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
}

// AddToSchema registers the Asset persistence model.
func AddToSchema(schema *commonstore.Schema) error {
	return schema.Register(&Asset{}, commonstore.ResourceSchema{
		ScopeKeys: []string{"kind", "owner"},
	})
}

// Options configures Store-backed asset serving and upload policies.
type Options struct {
	Policies Policies `json:"-"`
}

// Service stores Assets through a common Store.
type Service struct {
	storage commonstore.Store
	options Options
}

// New returns a Store-backed asset Service.
func New(storage commonstore.Store, options Options) *Service {
	return &Service{storage: storage, options: optionsWithDefaults(options)}
}

// Put creates or replaces content and increments its version.
func (s *Service) Put(ctx context.Context, target asset.Target, blob asset.Blob, options asset.PutOptions) (*asset.Asset, error) {
	name := options.Name
	if name == "" {
		name = uuid.NewString()
	}
	prepared, err := prepareBlob(target, name, blob, s.options.Policies)
	if err != nil {
		return nil, err
	}
	record := &Asset{ObjectMeta: commonstore.ObjectMeta{ID: name}}
	err = commonstore.CreateOrUpdate(ctx, s.targetStore(target), record, func() error {
		record.Kind = target.Kind
		record.Owner = target.Name
		record.UpdationTimestamp = meta.Now()
		record.ContentType = blob.ContentType
		record.FileName = blob.FileName
		record.ModTime = meta.Time{Time: blob.ModTime}
		record.ContentLength = int64(len(prepared.data))
		if blob.Link != nil {
			record.ContentLength = blob.ContentLength
		}
		record.Digest = prepared.digest
		record.Content = base64.StdEncoding.EncodeToString(prepared.data)
		record.Link = cloneLink(blob.Link)
		record.ETag = ""
		if blob.Content != nil {
			record.ETag = fmt.Sprintf(`W/"%s"`, prepared.digest)
		}
		if options.Metadata != nil {
			record.Metadata = maps.Clone(options.Metadata)
		}
		record.Version++
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := s.asset(target, record)
	return &result, nil
}

// Get returns stored asset information.
func (s *Service) Get(ctx context.Context, target asset.Target, name string) (*asset.Asset, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	record, err := s.get(ctx, target, name)
	if err != nil {
		return nil, err
	}
	result := s.asset(target, record)
	return &result, nil
}

// List returns one page of assets in a target scope.
func (s *Service) List(ctx context.Context, target asset.Target, options meta.ListOptions) (meta.Page[asset.Asset], error) {
	if err := asset.ValidateTarget(target); err != nil {
		return meta.Page[asset.Asset]{}, err
	}
	storeSort, err := assetStoreSort(options.Sort)
	if err != nil {
		return meta.Page[asset.Asset]{}, err
	}
	options.Sort = storeSort
	listOptions, err := commonstore.ListOptionsFromMeta(options)
	if err != nil {
		return meta.Page[asset.Asset]{}, commonerrors.NewBadRequest(err.Error())
	}
	list := &commonstore.List[Asset]{}
	if err := s.targetStore(target).List(ctx, list, listOptions...); err != nil {
		return meta.Page[asset.Asset]{}, err
	}
	page := commonstore.PageFromList(*list)
	return meta.ConvertPage(page, func(record Asset) asset.Asset {
		return s.asset(target, &record)
	}), nil
}

// ReplaceMetadata replaces metadata without changing content identity.
func (s *Service) ReplaceMetadata(ctx context.Context, target asset.Target, name string, metadata map[string]string) (*asset.Asset, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	record := &Asset{ObjectMeta: commonstore.ObjectMeta{ID: name}}
	patch, err := json.Marshal([]map[string]any{
		{"op": "add", "path": "/metadata", "value": metadata},
		{"op": "add", "path": "/updationTimestamp", "value": meta.Now()},
	})
	if err != nil {
		return nil, err
	}
	if err := s.targetStore(target).Patch(
		ctx,
		record,
		commonstore.RawPatch(commonstore.PatchTypeJSONPatch, patch),
	); err != nil {
		return nil, err
	}
	result := s.asset(target, record)
	return &result, nil
}

// Resolve loads asset content.
func (s *Service) Resolve(ctx context.Context, target asset.Target, name string, options asset.ResolveOptions) (*asset.Resolved, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	record, err := s.get(ctx, target, name)
	if err != nil {
		return nil, err
	}
	if options.Version > 0 && options.Version != record.Version {
		return nil, commonerrors.NewNotFound("asset", name)
	}
	if record.Link != nil {
		return &asset.Resolved{
			Asset: s.asset(target, record),
			Link:  cloneLink(record.Link),
		}, nil
	}
	content, err := base64.StdEncoding.DecodeString(record.Content)
	if err != nil {
		return nil, fmt.Errorf(
			"decode stored asset %s/%s/%s: %w",
			target.Kind,
			target.Name,
			name,
			err,
		)
	}
	contentRange := ""
	if options.Range != "" {
		ranges, err := api.ParseRange(options.Range, int64(len(content)))
		if err != nil {
			return nil, commonerrors.NewCustomError(
				http.StatusRequestedRangeNotSatisfiable,
				commonerrors.StatusReasonBadRequest,
				err.Error(),
			)
		}
		if len(ranges) > 1 {
			return nil, commonerrors.NewCustomError(
				http.StatusRequestedRangeNotSatisfiable,
				commonerrors.StatusReasonBadRequest,
				"multiple ranges are not supported",
			)
		}
		if len(ranges) == 1 {
			selected := ranges[0]
			content = content[selected.Start : selected.Start+selected.Length]
			contentRange = selected.ContentRange(record.ContentLength)
		}
	}
	return &asset.Resolved{
		Asset:         s.asset(target, record),
		Content:       io.NopCloser(bytes.NewReader(content)),
		ContentLength: int64(len(content)),
		ContentRange:  contentRange,
	}, nil
}

// Delete removes one asset.
func (s *Service) Delete(ctx context.Context, target asset.Target, name string) error {
	if err := asset.Validate(target, name); err != nil {
		return err
	}
	return s.targetStore(target).Delete(
		ctx,
		&Asset{ObjectMeta: commonstore.ObjectMeta{ID: name}},
	)
}

// DeleteAll removes every asset in a target scope.
func (s *Service) DeleteAll(ctx context.Context, target asset.Target) error {
	if err := asset.ValidateTarget(target); err != nil {
		return err
	}
	return s.targetStore(target).DeleteBatch(ctx, &commonstore.List[Asset]{})
}

func (s *Service) targetStore(target asset.Target) commonstore.Store {
	return s.storage.Scope(
		commonstore.Scope{Resource: "kinds", Name: target.Kind},
		commonstore.Scope{Resource: "owners", Name: target.Name},
	)
}

func (s *Service) get(ctx context.Context, target asset.Target, name string) (*Asset, error) {
	record := &Asset{}
	if err := s.targetStore(target).Get(ctx, name, record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) asset(target asset.Target, record *Asset) asset.Asset {
	return asset.Asset{
		Target:            target,
		Name:              record.ID,
		Version:           record.Version,
		Metadata:          maps.Clone(record.Metadata),
		ContentType:       record.ContentType,
		FileName:          record.FileName,
		ModTime:           record.ModTime,
		Size:              record.ContentLength,
		Digest:            record.Digest,
		ETag:              record.ETag,
		CreationTimestamp: record.CreationTimestamp,
		UpdationTimestamp: record.UpdationTimestamp,
	}
}

func optionsWithDefaults(options Options) Options {
	if options.Policies == nil {
		options.Policies = Policies{{}: {MaxBytes: DefaultMaxBytes}}
	}
	return options
}

type preparedBlob struct {
	data   []byte
	digest string
}

func prepareBlob(target asset.Target, name string, blob asset.Blob, policies Policies) (preparedBlob, error) {
	if err := asset.Validate(target, name); err != nil {
		return preparedBlob{}, err
	}
	if err := asset.ValidateBlob(blob); err != nil {
		return preparedBlob{}, err
	}
	policy := policies[PolicyKey{}]
	if kind, ok := policies[PolicyKey{Kind: target.Kind}]; ok {
		policy = kind
	}
	if exact, ok := policies[PolicyKey{Kind: target.Kind, Name: name}]; ok {
		policy = exact
	}
	if policy.MaxBytes <= 0 {
		policy.MaxBytes = DefaultMaxBytes
	}
	if !asset.IsMediaTypeAllowed(blob.ContentType, policy.AllowedMediaTypes) {
		return preparedBlob{}, commonerrors.NewBadRequest(
			fmt.Sprintf("asset content type %q is not allowed", blob.ContentType),
		)
	}
	if blob.ContentLength > policy.MaxBytes {
		return preparedBlob{}, commonerrors.NewRequestEntityTooLarge(
			fmt.Sprintf("asset exceeds %d bytes", policy.MaxBytes),
		)
	}
	if blob.Link != nil {
		return preparedBlob{}, nil
	}
	data, err := io.ReadAll(io.LimitReader(blob.Content, policy.MaxBytes+1))
	if err != nil {
		return preparedBlob{}, commonerrors.NewBadRequest(fmt.Sprintf("read asset content: %v", err))
	}
	if len(data) == 0 {
		return preparedBlob{}, commonerrors.NewBadRequest("asset content is empty")
	}
	if int64(len(data)) > policy.MaxBytes {
		return preparedBlob{}, commonerrors.NewRequestEntityTooLarge(
			fmt.Sprintf("asset exceeds %d bytes", policy.MaxBytes),
		)
	}
	if blob.ContentLength > 0 && blob.ContentLength != int64(len(data)) {
		return preparedBlob{}, commonerrors.NewBadRequest(
			fmt.Sprintf("asset content length is %d bytes, declared %d", len(data), blob.ContentLength),
		)
	}
	return preparedBlob{
		data:   data,
		digest: fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
	}, nil
}

func cloneLink(link *asset.Link) *asset.Link {
	if link == nil {
		return nil
	}
	result := *link
	return &result
}

func assetStoreSort(sort string) (string, error) {
	sorts := meta.ParseSort(sort)
	if len(sorts) == 0 {
		return "id+", nil
	}
	fields := make([]string, 0, len(sorts)+1)
	hasName := false
	for _, sorting := range sorts {
		switch sorting.Field {
		case "name":
			sorting.Field = "id"
			hasName = true
		case "creationTimestamp", "updationTimestamp":
		default:
			return "", commonerrors.NewBadRequest(
				fmt.Sprintf("unsupported asset sort field %q", sorting.Field),
			)
		}
		direction := "+"
		if sorting.Direction == meta.SortDirectionDesc {
			direction = "-"
		}
		fields = append(fields, sorting.Field+direction)
	}
	if !hasName {
		fields = append(fields, "id+")
	}
	return strings.Join(fields, ","), nil
}
