// Package s3 implements asset.Service with S3-compatible storage.
package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awstypes "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"xiaoshiai.cn/common/asset"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
	commons3 "xiaoshiai.cn/common/s3"
)

const (
	versionKey           = "asset-version"
	digestKey            = "asset-digest"
	metadataKey          = "asset-metadata"
	creationTimestampKey = "asset-creation-timestamp"
	updationTimestampKey = "asset-updation-timestamp"
	fileNameKey          = "asset-file-name"
	modTimeKey           = "asset-mod-time"
	linkKey              = "asset-link"
	contentLengthKey     = "asset-content-length"
)

var _ asset.Service = (*Service)(nil)

// DefaultMaxBytes is the default S3 upload size limit.
const DefaultMaxBytes = 8 * 1024 * 1024

// Policy controls content accepted by the S3-backed service.
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

// Policies contains S3 upload policies from least to most specific.
type Policies map[PolicyKey]Policy

// Options configures S3 persistence, delivery, and upload policies.
type Options struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix,omitempty"`
	// Proxy controls who accesses objects stored in this S3 bucket. When false,
	// a caller that sets ResolveOptions.PreferLink may receive a presigned S3 URL
	// and fetch the object directly. When true, only the Asset service accesses
	// S3, so S3-backed objects are read with GetObject and returned as Content
	// even when PreferLink is set. This option does not affect an external URL
	// uploaded through Blob.Link: a retained external URL is still returned as a
	// Link because it is not an object in this configured S3 bucket.
	Proxy bool `json:"proxy,omitempty"`
	// Policies controls accepted content by target kind and Asset name. It is
	// runtime policy and is not serialized as backend configuration.
	Policies Policies `json:"-"`
}

// Service stores each asset as one S3 object.
type Service struct {
	client  *commons3.Client
	options Options
}

// New returns an S3-backed asset Service.
func New(client *commons3.Client, options Options) *Service {
	options.Prefix = strings.Trim(options.Prefix, "/")
	if options.Policies == nil {
		options.Policies = Policies{{}: {MaxBytes: DefaultMaxBytes}}
	}
	return &Service{client: client, options: options}
}

// Put creates or replaces an S3 object and increments its logical version.
func (s *Service) Put(ctx context.Context, target asset.Target, blob asset.Blob, options asset.PutOptions) (*asset.Asset, error) {
	name := options.Name
	if name == "" {
		name = uuid.NewString()
	}
	prepared, err := prepareBlob(target, name, blob, s.options.Policies)
	if err != nil {
		return nil, err
	}
	objectKey := s.objectKey(target, name)
	version := int64(1)
	metadata := maps.Clone(options.Metadata)
	now := meta.Now()
	creationTimestamp := now
	head, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.options.Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		head = nil
	}
	if head != nil {
		current, err := s.asset(target, name, head)
		if err != nil {
			return nil, err
		}
		version = current.Version + 1
		if metadata == nil {
			metadata = maps.Clone(current.Metadata)
		}
		creationTimestamp = current.CreationTimestamp
	}
	storedMetadata, err := encodeMetadata(
		version,
		prepared.digest,
		metadata,
		creationTimestamp,
		now,
		blob.FileName,
		meta.Time{Time: blob.ModTime},
		blob.Link,
		blob.ContentLength,
	)
	if err != nil {
		return nil, err
	}
	_, err = s.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      aws.String(s.options.Bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(prepared.data),
		ContentType: stringPointer(blob.ContentType),
		Metadata:    storedMetadata,
	})
	if err != nil {
		return nil, err
	}
	etag := ""
	if blob.Content != nil {
		etag = fmt.Sprintf(`W/"%s"`, prepared.digest)
	}
	return &asset.Asset{
		Target:            target,
		Name:              name,
		FileName:          blob.FileName,
		ModTime:           meta.Time{Time: blob.ModTime},
		Version:           version,
		Metadata:          metadata,
		ContentType:       blob.ContentType,
		Size:              contentLength(blob, prepared.data),
		Digest:            prepared.digest,
		ETag:              etag,
		CreationTimestamp: creationTimestamp,
		UpdationTimestamp: now,
	}, nil
}

// Get returns S3 object information.
func (s *Service) Get(ctx context.Context, target asset.Target, name string) (*asset.Asset, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	result, _, err := s.get(ctx, target, name)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) get(ctx context.Context, target asset.Target, name string) (asset.Asset, *asset.Link, error) {
	head, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.options.Bucket),
		Key:    aws.String(s.objectKey(target, name)),
	})
	if err != nil {
		return asset.Asset{}, nil, backendError(target, name, err)
	}
	result, err := s.asset(target, name, head)
	if err != nil {
		return asset.Asset{}, nil, err
	}
	link, err := decodeLink(head.Metadata[linkKey])
	if err != nil {
		return asset.Asset{}, nil, err
	}
	return result, link, nil
}

// List returns one page of S3 objects under a target prefix.
func (s *Service) List(ctx context.Context, target asset.Target, options meta.ListOptions) (meta.Page[asset.Asset], error) {
	if err := asset.ValidateTarget(target); err != nil {
		return meta.Page[asset.Asset]{}, err
	}
	prefix := s.objectPrefix(target)
	input := &awss3.ListObjectsV2Input{
		Bucket: aws.String(s.options.Bucket),
		Prefix: aws.String(prefix),
	}
	items := []asset.Asset{}
	for {
		page, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return meta.Page[asset.Asset]{}, err
		}
		for _, object := range page.Contents {
			name := strings.TrimPrefix(aws.ToString(object.Key), prefix)
			current, err := s.Get(ctx, target, name)
			if err != nil {
				return meta.Page[asset.Asset]{}, err
			}
			items = append(items, *current)
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		input.ContinuationToken = page.NextContinuationToken
	}
	return paginate(items, options)
}

// ReplaceMetadata replaces caller-defined metadata without changing content identity.
func (s *Service) ReplaceMetadata(ctx context.Context, target asset.Target, name string, metadata map[string]string) (*asset.Asset, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	objectKey := s.objectKey(target, name)
	head, err := s.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(s.options.Bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, backendError(target, name, err)
	}
	current, err := s.asset(target, name, head)
	if err != nil {
		return nil, err
	}
	storedLink, err := decodeLink(head.Metadata[linkKey])
	if err != nil {
		return nil, err
	}
	now := meta.Now()
	storedMetadata, err := encodeMetadata(
		current.Version,
		current.Digest,
		metadata,
		current.CreationTimestamp,
		now,
		current.FileName,
		current.ModTime,
		storedLink,
		current.Size,
	)
	if err != nil {
		return nil, err
	}
	_, err = s.client.CopyObject(ctx, &awss3.CopyObjectInput{
		Bucket:            aws.String(s.options.Bucket),
		CopySource:        aws.String(url.PathEscape(s.options.Bucket + "/" + objectKey)),
		Key:               aws.String(objectKey),
		ContentType:       head.ContentType,
		Metadata:          storedMetadata,
		MetadataDirective: awstypes.MetadataDirectiveReplace,
	})
	if err != nil {
		return nil, err
	}
	current.Metadata = maps.Clone(metadata)
	current.UpdationTimestamp = now
	return &current, nil
}

// Resolve returns either an S3 object body or a presigned URL.
func (s *Service) Resolve(ctx context.Context, target asset.Target, name string, options asset.ResolveOptions) (*asset.Resolved, error) {
	if err := asset.Validate(target, name); err != nil {
		return nil, err
	}
	current, storedLink, err := s.get(ctx, target, name)
	if err != nil {
		return nil, err
	}
	if options.Version > 0 && options.Version != current.Version {
		return nil, commonerrors.NewNotFound("asset", name)
	}
	if storedLink != nil {
		return &asset.Resolved{Asset: current, Link: storedLink}, nil
	}
	objectKey := s.objectKey(target, name)
	if !s.options.Proxy && options.PreferLink {
		ttl := options.LinkTTL
		if ttl == 0 {
			ttl = 5 * time.Minute
		}
		presigned, err := s.client.PresignGetObject(ctx, &awss3.GetObjectInput{
			Bucket: aws.String(s.options.Bucket),
			Key:    aws.String(objectKey),
		}, func(options *awss3.PresignOptions) {
			options.Expires = ttl
		})
		if err != nil {
			return nil, err
		}
		if len(presigned.SignedHeader) == 0 {
			return &asset.Resolved{
				Asset: current,
				Link: &asset.Link{
					URL:       presigned.URL,
					ExpiresAt: time.Now().Add(ttl),
				},
			}, nil
		}
	}
	input := &awss3.GetObjectInput{
		Bucket: aws.String(s.options.Bucket),
		Key:    aws.String(objectKey),
	}
	if options.Range != "" {
		input.Range = aws.String(options.Range)
	}
	object, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, backendError(target, name, err)
	}
	return &asset.Resolved{
		Asset:         current,
		Content:       object.Body,
		ContentLength: aws.ToInt64(object.ContentLength),
		ContentRange:  aws.ToString(object.ContentRange),
	}, nil
}

// Delete removes one S3 object.
func (s *Service) Delete(ctx context.Context, target asset.Target, name string) error {
	if err := asset.Validate(target, name); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.options.Bucket),
		Key:    aws.String(s.objectKey(target, name)),
	})
	return err
}

// DeleteAll removes every S3 object under a target prefix.
func (s *Service) DeleteAll(ctx context.Context, target asset.Target) error {
	if err := asset.ValidateTarget(target); err != nil {
		return err
	}
	input := &awss3.ListObjectsV2Input{
		Bucket: aws.String(s.options.Bucket),
		Prefix: aws.String(s.objectPrefix(target)),
	}
	objects := []awstypes.ObjectIdentifier{}
	for {
		page, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			objects = append(objects, awstypes.ObjectIdentifier{Key: object.Key})
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		input.ContinuationToken = page.NextContinuationToken
	}
	for start := 0; start < len(objects); start += 1000 {
		end := min(start+1000, len(objects))
		if _, err := s.client.DeleteObjects(ctx, &awss3.DeleteObjectsInput{
			Bucket: aws.String(s.options.Bucket),
			Delete: &awstypes.Delete{
				Objects: objects[start:end],
				Quiet:   aws.Bool(true),
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) objectPrefix(target asset.Target) string {
	return path.Join(s.options.Prefix, target.Kind, target.Name) + "/"
}

func (s *Service) objectKey(target asset.Target, name string) string {
	return s.objectPrefix(target) + name
}

func (s *Service) asset(target asset.Target, name string, head *awss3.HeadObjectOutput) (asset.Asset, error) {
	version, err := strconv.ParseInt(head.Metadata[versionKey], 10, 64)
	if err != nil {
		return asset.Asset{}, err
	}
	metadata, err := decodeMetadata(head.Metadata[metadataKey])
	if err != nil {
		return asset.Asset{}, err
	}
	creationTimestamp, err := parseTimestamp(head.Metadata[creationTimestampKey])
	if err != nil {
		return asset.Asset{}, err
	}
	updationTimestamp, err := parseTimestamp(head.Metadata[updationTimestampKey])
	if err != nil {
		return asset.Asset{}, err
	}
	fileName, err := decodeString(head.Metadata[fileNameKey])
	if err != nil {
		return asset.Asset{}, err
	}
	modTime, err := parseOptionalTimestamp(head.Metadata[modTimeKey])
	if err != nil {
		return asset.Asset{}, err
	}
	link, err := decodeLink(head.Metadata[linkKey])
	if err != nil {
		return asset.Asset{}, err
	}
	size := aws.ToInt64(head.ContentLength)
	digest := head.Metadata[digestKey]
	etag := fmt.Sprintf(`W/"%s"`, digest)
	if link != nil {
		size, err = parseOptionalInt64(head.Metadata[contentLengthKey])
		if err != nil {
			return asset.Asset{}, err
		}
		etag = ""
		digest = ""
	}
	return asset.Asset{
		Target:            target,
		Name:              name,
		FileName:          fileName,
		ModTime:           modTime,
		Version:           version,
		Metadata:          metadata,
		ContentType:       aws.ToString(head.ContentType),
		Size:              size,
		Digest:            digest,
		ETag:              etag,
		CreationTimestamp: creationTimestamp,
		UpdationTimestamp: updationTimestamp,
	}, nil
}

func encodeMetadata(
	version int64,
	digest string,
	metadata map[string]string,
	creationTimestamp meta.Time,
	updationTimestamp meta.Time,
	fileName string,
	modTime meta.Time,
	link *asset.Link,
	contentLength int64,
) (map[string]string, error) {
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	linkData, err := json.Marshal(link)
	if err != nil {
		return nil, err
	}
	storedModTime := ""
	if !modTime.IsZero() {
		storedModTime = modTime.UTC().Format(time.RFC3339Nano)
	}
	return map[string]string{
		versionKey:           strconv.FormatInt(version, 10),
		digestKey:            digest,
		metadataKey:          base64.StdEncoding.EncodeToString(data),
		creationTimestampKey: creationTimestamp.UTC().Format(time.RFC3339Nano),
		updationTimestampKey: updationTimestamp.UTC().Format(time.RFC3339Nano),
		fileNameKey:          base64.StdEncoding.EncodeToString([]byte(fileName)),
		modTimeKey:           storedModTime,
		linkKey:              base64.StdEncoding.EncodeToString(linkData),
		contentLengthKey:     strconv.FormatInt(contentLength, 10),
	}, nil
}

func parseOptionalTimestamp(value string) (meta.Time, error) {
	if value == "" {
		return meta.Time{}, nil
	}
	return parseTimestamp(value)
}

func parseOptionalInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func decodeString(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeLink(value string) (*asset.Link, error) {
	if value == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var link *asset.Link
	if err := json.Unmarshal(data, &link); err != nil {
		return nil, err
	}
	return link, nil
}

func parseTimestamp(value string) (meta.Time, error) {
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return meta.Time{}, err
	}
	return meta.Time{Time: timestamp}, nil
}

func decodeMetadata(value string) (map[string]string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	metadata := map[string]string(nil)
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func backendError(target asset.Target, name string, err error) error {
	if isNotFound(err) {
		return commonerrors.NewNotFound("asset", name)
	}
	return fmt.Errorf(
		"access S3 asset %s/%s/%s: %w",
		target.Kind,
		target.Name,
		name,
		err,
	)
}

func isNotFound(err error) bool {
	var apiError smithy.APIError
	return stderrors.As(err, &apiError) &&
		(apiError.ErrorCode() == "NotFound" || apiError.ErrorCode() == "NoSuchKey")
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
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

func contentLength(blob asset.Blob, data []byte) int64 {
	if blob.Link != nil {
		return blob.ContentLength
	}
	return int64(len(data))
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
			return meta.Page[asset.Asset]{}, commonerrors.NewBadRequest(
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
