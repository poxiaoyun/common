// Package http implements the Asset HTTP server and client adapters.
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"xiaoshiai.cn/common/asset"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
)

// Server projects an asset Service over HTTP.
type Server struct {
	Assets asset.Service
}

// NewServer creates an Asset HTTP server adapter.
func NewServer(assets asset.Service) *Server {
	return &Server{Assets: assets}
}

// List lists assets owned by one target.
func (s *Server) List(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		options, err := api.GetListOptions(r, meta.DefaultSort("name"))
		if err != nil {
			return nil, err
		}
		return s.Assets.List(ctx, requestTarget(r), options)
	})
}

// Create creates an asset with a generated name.
func (s *Server) Create(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		options, err := readPutOptions(r, "")
		if err != nil {
			return nil, err
		}
		blob, closer, err := readBlob(r)
		if err != nil {
			return nil, err
		}
		if closer != nil {
			defer closer.Close()
		}
		result, err := s.Assets.Put(ctx, requestTarget(r), blob, options)
		if err != nil {
			return nil, err
		}
		api.Raw(w, http.StatusCreated, result)
		return nil, nil
	})
}

// Put creates or replaces a named asset.
func (s *Server) Put(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		options, err := readPutOptions(r, api.Path(r, "asset", ""))
		if err != nil {
			return nil, err
		}
		blob, closer, err := readBlob(r)
		if err != nil {
			return nil, err
		}
		if closer != nil {
			defer closer.Close()
		}
		return s.Assets.Put(
			ctx,
			requestTarget(r),
			blob,
			options,
		)
	})
}

// Get returns metadata for a named asset.
func (s *Server) Get(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		return s.Assets.Get(ctx, requestTarget(r), api.Path(r, "asset", ""))
	})
}

// ReplaceMetadata replaces a named asset's metadata.
func (s *Server) ReplaceMetadata(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		metadata := map[string]string{}
		if err := api.Body(r, &metadata); err != nil {
			return nil, err
		}
		return s.Assets.ReplaceMetadata(
			ctx,
			requestTarget(r),
			api.Path(r, "asset", ""),
			metadata,
		)
	})
}

// Delete removes one named asset.
func (s *Server) Delete(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		if err := s.Assets.Delete(
			ctx,
			requestTarget(r),
			api.Path(r, "asset", ""),
		); err != nil {
			return nil, err
		}
		return api.NoContent, nil
	})
}

// DeleteAll removes every asset owned by one target.
func (s *Server) DeleteAll(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		if err := s.Assets.DeleteAll(ctx, requestTarget(r)); err != nil {
			return nil, err
		}
		return api.NoContent, nil
	})
}

// Serve resolves one asset into a redirect or content response.
func (s *Server) Serve(w http.ResponseWriter, r *http.Request) {
	version := api.Query(r, "v", int64(0))
	resolved, err := s.Assets.Resolve(
		r.Context(),
		requestTarget(r),
		api.Path(r, "asset", ""),
		asset.ResolveOptions{
			PreferLink: api.Query(r, "link", true),
			LinkTTL:    5 * time.Minute,
			Version:    version,
			Range:      r.Header.Get("Range"),
		},
	)
	if err != nil {
		if errors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		api.Error(w, err)
		return
	}
	api.ServeContentResponse(w, r, ContentResponse(resolved, version > 0))
}

// ContentResponse projects a resolved Asset into an HTTP response. versioned
// enables long-lived caching for an immutable version-specific URL; current
// Asset URLs remain non-cacheable so replacements are observed.
func ContentResponse(resolved *asset.Resolved, versioned bool) api.ContentResponse {
	headers := http.Header{}
	descriptor, _ := json.Marshal(resolved.Asset)
	headers.Set(contentDescriptorHeader, base64.RawURLEncoding.EncodeToString(descriptor))
	if resolved.Asset.ContentType != "" {
		headers.Set("Content-Type", resolved.Asset.ContentType)
	}
	if resolved.Asset.FileName != "" {
		headers.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{
			"filename": resolved.Asset.FileName,
		}))
	}
	if !resolved.Asset.ModTime.IsZero() {
		headers.Set(modTimeHeader, resolved.Asset.ModTime.UTC().Format(time.RFC3339Nano))
	}
	if resolved.Asset.Size > 0 {
		headers.Set(contentLengthHeader, strconv.FormatInt(resolved.Asset.Size, 10))
	}
	if resolved.Link != nil {
		if !resolved.Link.ExpiresAt.IsZero() {
			headers.Set(linkExpiresAtHeader, resolved.Link.ExpiresAt.UTC().Format(time.RFC3339Nano))
		}
		return api.ContentResponse{Headers: headers, Location: resolved.Link.URL}
	}
	if resolved.ContentRange != "" {
		headers.Set("Content-Range", resolved.ContentRange)
	}
	if resolved.Asset.ETag != "" {
		headers.Set("ETag", resolved.Asset.ETag)
	}
	if !resolved.Asset.UpdationTimestamp.IsZero() {
		headers.Set("Last-Modified", resolved.Asset.UpdationTimestamp.UTC().Format(http.TimeFormat))
	}
	if versioned {
		headers.Set("Cache-Control", "public, max-age=2592000")
	} else {
		headers.Set("Cache-Control", "no-cache")
	}
	return api.ContentResponse{Headers: headers, Content: resolved.Content, ContentLength: resolved.ContentLength}
}

// PublicGroup returns content delivery routes.
func (s *Server) PublicGroup() api.Group {
	return api.
		NewGroup("/assets").
		Tag("Assets").
		Route(
			api.GET("/{kind}/{target}/{asset}").
				To(s.Serve).
				Doc("Get asset content"),
		)
}

// Group returns asset management routes.
func (s *Server) Group() api.Group {
	return api.
		NewGroup("/assets").
		Tag("Assets").
		Route(
			api.GET("/{kind}/{target}").
				To(s.List).
				Doc("List assets").
				Param(api.PageParams...).
				Response(meta.Page[asset.Asset]{}),
			api.POST("/{kind}/{target}").
				To(s.Create).
				Doc("Create asset").
				Param(
					api.BodyParam("file", []byte{}).Desc("asset content"),
					api.HeaderParam(metadataHeader, "base64url-encoded JSON asset metadata").Optional(),
				).
				ResponseStatus(http.StatusCreated, asset.Asset{}),
			api.PUT("/{kind}/{target}/{asset}").
				To(s.Put).
				Doc("Put asset").
				Param(
					api.BodyParam("file", []byte{}).Desc("asset content"),
					api.HeaderParam(metadataHeader, "base64url-encoded JSON asset metadata").Optional(),
				).
				Response(asset.Asset{}),
			api.GET("/{kind}/{target}/{asset}/metadata").
				To(s.Get).
				Doc("Get asset metadata").
				Response(asset.Asset{}),
			api.PUT("/{kind}/{target}/{asset}/metadata").
				To(s.ReplaceMetadata).
				Doc("Replace asset metadata").
				Param(api.BodyParam("metadata", map[string]string{})).
				Response(asset.Asset{}),
			api.DELETE("/{kind}/{target}/{asset}").
				To(s.Delete).
				Doc("Delete asset"),
			api.DELETE("/{kind}/{target}").
				To(s.DeleteAll).
				Doc("Delete target assets"),
		)
}

func requestTarget(r *http.Request) asset.Target {
	return asset.Target{
		Kind: api.Path(r, "kind", ""),
		Name: api.Path(r, "target", ""),
	}
}

func readPutOptions(r *http.Request, name string) (asset.PutOptions, error) {
	metadata, err := decodeMetadataHeader(r.Header.Get(metadataHeader))
	if err != nil {
		return asset.PutOptions{}, errors.NewBadRequest("invalid asset metadata")
	}
	return asset.PutOptions{Name: name, Metadata: metadata}, nil
}

func readBlob(r *http.Request) (asset.Blob, io.Closer, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && mediaType == linkMediaType {
		input := linkPutRequest{}
		if err := api.Body(r, &input); err != nil {
			return asset.Blob{}, nil, err
		}
		return asset.Blob{
			Link:          &input.Link,
			ContentType:   input.ContentType,
			ContentLength: input.ContentLength,
			FileName:      input.FileName,
			ModTime:       input.ModTime,
		}, nil, nil
	}
	content, contentType, fileName, contentLength, err := readContent(r)
	if err != nil {
		return asset.Blob{}, nil, err
	}
	modTime := time.Time{}
	if value := r.Header.Get(modTimeHeader); value != "" {
		modTime, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			content.Close()
			return asset.Blob{}, nil, errors.NewBadRequest("invalid asset modification time")
		}
	}
	return asset.Blob{
		Content:       content,
		ContentType:   contentType,
		ContentLength: contentLength,
		FileName:      fileName,
		ModTime:       modTime,
	}, content, nil
}

func readContent(r *http.Request) (io.ReadCloser, string, string, int64, error) {
	multipart, err := r.MultipartReader()
	if err != nil {
		if err == http.ErrNotMultipart {
			fileName := ""
			if disposition := r.Header.Get("Content-Disposition"); disposition != "" {
				_, parameters, parseErr := mime.ParseMediaType(disposition)
				if parseErr != nil {
					return nil, "", "", 0, errors.NewBadRequest("invalid content disposition")
				}
				fileName = parameters["filename"]
			}
			return r.Body, r.Header.Get("Content-Type"), fileName, max(r.ContentLength, 0), nil
		}
		return nil, "", "", 0, err
	}
	for {
		part, err := multipart.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", "", 0, err
		}
		if part.FormName() == "file" {
			contentLength, _ := strconv.ParseInt(part.Header.Get("Content-Length"), 10, 64)
			return part, part.Header.Get("Content-Type"), part.FileName(), max(contentLength, 0), nil
		}
		part.Close()
	}
	return nil, "", "", 0, errors.NewBadRequest("asset content is required")
}
