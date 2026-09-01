// Package inmemory implements asset.Service in memory.
package inmemory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/asset"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
)

var _ asset.Service = (*Service)(nil)

// DefaultMaxBytes is the default in-memory upload size limit.
const DefaultMaxBytes = 8 * 1024 * 1024

// Policy controls content accepted by the in-memory service.
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

// Policies contains in-memory upload policies from least to most specific.
type Policies map[PolicyKey]Policy

// Options configures in-memory asset serving and upload policies.
type Options struct {
	Policies Policies `json:"-"`
}

type storedAsset struct {
	asset   asset.Asset
	content []byte
	link    *asset.Link
}

// Service stores assets in memory.
type Service struct {
	mu      sync.RWMutex
	items   map[string]storedAsset
	options Options
}

// New creates an empty in-memory asset Service.
func New(options Options) *Service {
	return &Service{
		items:   map[string]storedAsset{},
		options: optionsWithDefaults(options),
	}
}

// Put creates or replaces content and increments its version.
func (s *Service) Put(_ context.Context, target asset.Target, blob asset.Blob, options asset.PutOptions) (*asset.Asset, error) {
	name := options.Name
	if name == "" {
		name = uuid.NewString()
	}
	prepared, err := prepareBlob(target, name, blob, s.options.Policies)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := itemKey(target, name)
	item, exists := s.items[key]
	now := meta.Now()
	if !exists {
		item.asset.CreationTimestamp = now
	}
	item.asset.Target = target
	item.asset.Name = name
	item.asset.Version++
	item.asset.ContentType = blob.ContentType
	item.asset.FileName = blob.FileName
	item.asset.ModTime = meta.Time{Time: blob.ModTime}
	item.asset.Size = int64(len(prepared.data))
	if blob.Link != nil {
		item.asset.Size = blob.ContentLength
	}
	item.asset.Digest = prepared.digest
	item.asset.ETag = ""
	if blob.Content != nil {
		item.asset.ETag = fmt.Sprintf(`W/"%s"`, prepared.digest)
	}
	if options.Metadata != nil {
		item.asset.Metadata = maps.Clone(options.Metadata)
	}
	item.asset.UpdationTimestamp = now
	item.content = bytes.Clone(prepared.data)
	item.link = cloneLink(blob.Link)
	s.items[key] = item
	result := cloneAsset(item.asset)
	return &result, nil
}

// Get returns stored asset information.
func (s *Service) Get(_ context.Context, target asset.Target, name string) (*asset.Asset, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[itemKey(target, name)]
	if !ok {
		return nil, errors.NewNotFound("asset", name)
	}
	result := cloneAsset(item.asset)
	return &result, nil
}

// List returns one page of assets owned by a target.
func (s *Service) List(_ context.Context, target asset.Target, options meta.ListOptions) (meta.Page[asset.Asset], error) {
	if err := asset.ValidateTarget(target); err != nil {
		return meta.Page[asset.Asset]{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := itemKey(target, "")
	items := []asset.Asset{}
	for key, item := range s.items {
		if strings.HasPrefix(key, prefix) {
			items = append(items, cloneAsset(item.asset))
		}
	}
	return paginate(items, options)
}

// ReplaceMetadata replaces metadata without changing content identity.
func (s *Service) ReplaceMetadata(_ context.Context, target asset.Target, name string, metadata map[string]string) (*asset.Asset, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	key := itemKey(target, name)
	item, ok := s.items[key]
	if !ok {
		return nil, errors.NewNotFound("asset", name)
	}
	item.asset.Metadata = maps.Clone(metadata)
	item.asset.UpdationTimestamp = meta.Now()
	s.items[key] = item
	result := cloneAsset(item.asset)
	return &result, nil
}

// Resolve loads asset content.
func (s *Service) Resolve(_ context.Context, target asset.Target, name string, options asset.ResolveOptions) (*asset.Resolved, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[itemKey(target, name)]
	if !ok || options.Version > 0 && options.Version != item.asset.Version {
		return nil, errors.NewNotFound("asset", name)
	}
	if item.link != nil {
		return &asset.Resolved{Asset: cloneAsset(item.asset), Link: cloneLink(item.link)}, nil
	}
	content := item.content
	contentRange := ""
	if options.Range != "" {
		ranges, err := api.ParseRange(options.Range, int64(len(content)))
		if err != nil {
			return nil, errors.NewCustomError(
				http.StatusRequestedRangeNotSatisfiable,
				errors.StatusReasonBadRequest,
				err.Error(),
			)
		}
		if len(ranges) > 1 {
			return nil, errors.NewCustomError(
				http.StatusRequestedRangeNotSatisfiable,
				errors.StatusReasonBadRequest,
				"multiple ranges are not supported",
			)
		}
		if len(ranges) == 1 {
			selected := ranges[0]
			content = content[selected.Start : selected.Start+selected.Length]
			contentRange = selected.ContentRange(item.asset.Size)
		}
	}
	return &asset.Resolved{
		Asset:         cloneAsset(item.asset),
		Content:       io.NopCloser(bytes.NewReader(content)),
		ContentLength: int64(len(content)),
		ContentRange:  contentRange,
	}, nil
}

// Delete removes one asset.
func (s *Service) Delete(_ context.Context, target asset.Target, name string) error {
	if err := asset.Validate(target, name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	key := itemKey(target, name)
	if _, ok := s.items[key]; !ok {
		return errors.NewNotFound("asset", name)
	}
	delete(s.items, key)
	return nil
}

// DeleteAll removes every asset owned by a target.
func (s *Service) DeleteAll(_ context.Context, target asset.Target) error {
	if err := asset.ValidateTarget(target); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := itemKey(target, "")
	for key := range s.items {
		if strings.HasPrefix(key, prefix) {
			delete(s.items, key)
		}
	}
	return nil
}

func optionsWithDefaults(options Options) Options {
	if options.Policies == nil {
		options.Policies = Policies{{}: {MaxBytes: DefaultMaxBytes}}
	}
	return options
}

func itemKey(target asset.Target, name string) string {
	return target.Kind + "\x00" + target.Name + "\x00" + name
}

func cloneAsset(value asset.Asset) asset.Asset {
	value.Metadata = maps.Clone(value.Metadata)
	return value
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
		return preparedBlob{}, errors.NewBadRequest(
			fmt.Sprintf("asset content type %q is not allowed", blob.ContentType),
		)
	}
	if blob.ContentLength > policy.MaxBytes {
		return preparedBlob{}, errors.NewRequestEntityTooLarge(
			fmt.Sprintf("asset exceeds %d bytes", policy.MaxBytes),
		)
	}
	if blob.Link != nil {
		return preparedBlob{}, nil
	}
	data, err := io.ReadAll(io.LimitReader(blob.Content, policy.MaxBytes+1))
	if err != nil {
		return preparedBlob{}, errors.NewBadRequest(fmt.Sprintf("read asset content: %v", err))
	}
	if len(data) == 0 {
		return preparedBlob{}, errors.NewBadRequest("asset content is empty")
	}
	if int64(len(data)) > policy.MaxBytes {
		return preparedBlob{}, errors.NewRequestEntityTooLarge(
			fmt.Sprintf("asset exceeds %d bytes", policy.MaxBytes),
		)
	}
	if blob.ContentLength > 0 && blob.ContentLength != int64(len(data)) {
		return preparedBlob{}, errors.NewBadRequest(
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

func paginate(items []asset.Asset, options meta.ListOptions) (meta.Page[asset.Asset], error) {
	sorts := meta.ParseSort(options.Sort)
	if len(sorts) == 0 {
		sorts = []meta.SortField{{Field: "name", Direction: meta.SortDirectionAsc}}
	}
	for _, sorting := range sorts {
		switch sorting.Field {
		case "name", "creationTimestamp", "updationTimestamp":
		default:
			return meta.Page[asset.Asset]{}, errors.NewBadRequest(
				fmt.Sprintf("unsupported asset sort field %q", sorting.Field),
			)
		}
	}
	slices.SortFunc(items, func(left, right asset.Asset) int {
		for _, sorting := range sorts {
			comparison := compareAsset(left, right, sorting.Field)
			if sorting.Direction == meta.SortDirectionDesc {
				comparison = -comparison
			}
			if comparison != 0 {
				return comparison
			}
		}
		return strings.Compare(left.Name, right.Name)
	})
	options.Sort = ""
	return api.PageFromListOptions(
		items,
		options,
		func(item asset.Asset) string { return item.Name },
		nil,
		nil,
	)
}

func compareAsset(left, right asset.Asset, field string) int {
	switch field {
	case "name":
		return strings.Compare(left.Name, right.Name)
	case "creationTimestamp":
		return left.CreationTimestamp.Time.Compare(right.CreationTimestamp.Time)
	case "updationTimestamp":
		return left.UpdationTimestamp.Time.Compare(right.UpdationTimestamp.Time)
	default:
		return 0
	}
}
