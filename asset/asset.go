// Package asset defines the caller-facing contract for named binary attachments.
package asset

import (
	"context"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

var (
	// Asset names and target kinds are portable identity components. They are
	// used as HTTP path segments and Store or object-storage keys, so they
	// exclude path and URL separators, whitespace, and punctuation-only path
	// segments. The 64-character limit preserves the existing asset API
	// identity contract rather than reflecting a backend-specific limit.
	assetNameRegexp = regexp.MustCompile(
		`^[a-zA-Z0-9](?:[a-zA-Z0-9._-]{0,62}[a-zA-Z0-9])?$`,
	)
	// Target names use the same portable components but allow one colon as the
	// structural separator in a scoped identity such as "cloud:database". Each
	// component retains the existing target API's 128-character limit.
	targetNameRegexp = regexp.MustCompile(
		`^[a-zA-Z0-9](?:[a-zA-Z0-9._-]{0,126}[a-zA-Z0-9])?(?::[a-zA-Z0-9](?:[a-zA-Z0-9._-]{0,126}[a-zA-Z0-9])?)?$`,
	)
)

// Target identifies the object that owns assets.
type Target struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Asset describes one named attachment owned by a target.
type Asset struct {
	Target            Target            `json:"target"`
	Name              string            `json:"name"`
	FileName          string            `json:"fileName,omitempty"`
	ModTime           meta.Time         `json:"modTime,omitempty"`
	Version           int64             `json:"version"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	ContentType       string            `json:"contentType,omitempty"`
	Size              int64             `json:"size,omitempty"`
	Digest            string            `json:"digest,omitempty"`
	ETag              string            `json:"etag,omitempty"`
	CreationTimestamp meta.Time         `json:"creationTimestamp,omitempty"`
	UpdationTimestamp meta.Time         `json:"updationTimestamp,omitempty"`
}

// Link describes content available from a self-contained direct URL. The URL
// is the delivery mechanism returned by Resolve; it is not part of Asset
// metadata and callers must not use it to determine how an Asset is stored.
type Link struct {
	// URL can be fetched directly without adding service-side credentials.
	URL string `json:"url"`
	// ExpiresAt is the time after which URL is no longer guaranteed to work.
	// A zero value means that no expiry was declared.
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

// Blob contains one content source and its file properties. Exactly one of
// Content and Link must be set. Put consumes Content before returning and does
// not close it.
type Blob struct {
	// Content is synchronously consumed by Put and remains owned by the caller.
	Content io.Reader
	// Link asks the service to ingest content from a direct URL. The service may
	// either store a copy or retain the URL and return it from Resolve. Callers
	// therefore cannot infer the eventual delivery method from the upload method.
	Link *Link
	// ContentType is the media type of the uploaded content and is required for
	// both Content and Link sources.
	ContentType string
	// ContentLength is the declared total byte length. Zero means unknown; the
	// service reports the authoritative size after materializing Content.
	ContentLength int64
	// FileName is source/display metadata and never becomes the Asset name or a
	// storage key.
	FileName string
	// ModTime is the source file's modification time, not the service's creation
	// or update timestamp. A zero value means unknown.
	ModTime time.Time
}

// PutOptions supplies optional Asset values for Put.
type PutOptions struct {
	// Name identifies the Asset to create or replace. An empty value generates
	// a new name.
	Name string
	// Metadata replaces caller-defined metadata when non-nil. A nil value
	// preserves existing metadata when replacing a named Asset.
	Metadata map[string]string
}

// ResolveOptions controls content delivery.
type ResolveOptions struct {
	// PreferLink allows the implementation to return a direct Link when one is
	// available. It does not require a Link result.
	PreferLink bool
	// LinkTTL requests the lifetime of a generated delivery Link.
	LinkTTL time.Duration
	// Version selects a content version. Zero selects the current version.
	Version int64
	// Range is an RFC 7233 byte-range request. Implementations that return
	// Content apply it and describe the result with ContentRange. An
	// implementation may still return a Link; the caller then forwards the Range
	// header when fetching that URL.
	Range string
}

// Resolved contains an asset and exactly one content delivery method.
type Resolved struct {
	// Asset describes the complete logical object. Asset.Size remains the total
	// object size even when Content contains only a requested range.
	Asset Asset
	// Content contains either the complete representation or the requested
	// Range. ContentLength is the number of bytes returned by Content.
	Content       io.ReadCloser
	ContentLength int64
	// ContentRange is the RFC 7233 Content-Range value when Content contains a
	// byte range.
	ContentRange string
	// Link is an alternative delivery method to Content. Exactly one of Content
	// and Link is returned.
	Link *Link
}

// Service is the caller-facing asset capability. Store and remote HTTP
// implementations provide the same behavior through this contract.
type Service interface {
	// Put stores a Blob and advances its content version. An empty options.Name
	// generates a new Asset name; a non-empty Name creates or replaces it.
	Put(ctx context.Context, target Target, blob Blob, options PutOptions) (*Asset, error)
	// Get returns one named asset without loading its content.
	Get(ctx context.Context, target Target, name string) (*Asset, error)
	// List returns one page of assets owned by a target.
	List(ctx context.Context, target Target, options meta.ListOptions) (meta.Page[Asset], error)
	// ReplaceMetadata replaces caller-defined metadata without changing the
	// content Version, Digest, or ETag.
	ReplaceMetadata(ctx context.Context, target Target, name string, metadata map[string]string) (*Asset, error)
	// Resolve returns an asset and either Content or a direct Link. Range is
	// applied when Content is returned; it does not force a backend to replace a
	// Link result with proxied Content.
	Resolve(ctx context.Context, target Target, name string, options ResolveOptions) (*Resolved, error)
	// Delete removes one named asset.
	Delete(ctx context.Context, target Target, name string) error
	// DeleteAll removes every asset owned by a target.
	DeleteAll(ctx context.Context, target Target) error
}

// ValidateBlob checks the content-source invariant and required media type.
func ValidateBlob(blob Blob) error {
	if (blob.Content == nil) == (blob.Link == nil) {
		return errors.NewBadRequest("asset blob must contain exactly one of content or link")
	}
	if blob.ContentType == "" {
		return errors.NewBadRequest("asset blob content type is required")
	}
	if blob.ContentLength < 0 {
		return errors.NewBadRequest("asset blob content length must not be negative")
	}
	if blob.Link != nil && blob.Link.URL == "" {
		return errors.NewBadRequest("asset blob link URL is required")
	}
	return nil
}

// IsMediaTypeAllowed reports whether contentType matches an allowed media
// range. An empty allowed list accepts every media type. Entries may be exact
// media types or wildcard media ranges such as image/*. Parameters do not affect
// matching.
func IsMediaTypeAllowed(contentType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	typeName, subtype, ok := strings.Cut(mediaType, "/")
	if !ok {
		return false
	}
	for _, mediaRange := range allowed {
		allowedType, _, err := mime.ParseMediaType(mediaRange)
		if err != nil {
			continue
		}
		allowedTypeName, allowedSubtype, ok := strings.Cut(allowedType, "/")
		if !ok {
			continue
		}
		if (allowedTypeName == "*" || allowedTypeName == typeName) &&
			(allowedSubtype == "*" || allowedSubtype == subtype) {
			return true
		}
	}
	return false
}

// Validate checks an asset identity.
func Validate(target Target, name string) error {
	if err := ValidateTarget(target); err != nil {
		return err
	}
	if !assetNameRegexp.MatchString(name) {
		return errors.NewBadRequest(
			fmt.Sprintf("invalid asset name %s, must match %s", name, assetNameRegexp),
		)
	}
	return nil
}

// ValidateTarget checks an asset target identity.
func ValidateTarget(target Target) error {
	if !assetNameRegexp.MatchString(target.Kind) {
		return errors.NewBadRequest(
			fmt.Sprintf("invalid target kind %s, must match %s", target.Kind, assetNameRegexp),
		)
	}
	return ValidateTargetName(target.Name)
}

// ValidateTargetName validates a single or two-segment target identifier.
func ValidateTargetName(name string) error {
	if !targetNameRegexp.MatchString(name) {
		return errors.NewBadRequest(
			fmt.Sprintf("invalid target name %s, must match %s", name, targetNameRegexp),
		)
	}
	return nil
}
