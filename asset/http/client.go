package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"xiaoshiai.cn/common/asset"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/meta"
)

var _ asset.Service = (*Client)(nil)

const (
	contentDescriptorHeader = "X-Asset-Descriptor"
	linkMediaType           = "application/vnd.xiaoshiai.asset-link+json"
	modTimeHeader           = "X-Asset-Mod-Time"
	linkExpiresAtHeader     = "X-Asset-Link-Expires-At"
	contentLengthHeader     = "X-Asset-Content-Length"
	metadataHeader          = "X-Asset-Metadata"
)

type linkPutRequest struct {
	Link          asset.Link `json:"link"`
	ContentType   string     `json:"contentType"`
	ContentLength int64      `json:"contentLength,omitempty"`
	FileName      string     `json:"fileName,omitempty"`
	ModTime       time.Time  `json:"modTime,omitempty"`
}

// Client implements asset.Service over HTTP.
type Client struct {
	client *httpclient.Client
}

// TransportWrapper composes authentication or other request behavior around
// the HTTP adapter's configured base transport.
type TransportWrapper = httpclient.TransportWrapper

// Options configures the remote Asset service endpoint and authentication.
type Options struct {
	Address string `json:"address"`
	Token   string `json:"token,omitempty" config:"token,sensitive"`
}

// New returns an HTTP-backed asset Service.
func New(options Options) (*Client, error) {
	return NewWithTransport(options, nil)
}

// NewWithTransport returns an HTTP-backed asset Service using wrapper.
func NewWithTransport(options Options, wrapper TransportWrapper) (*Client, error) {
	client, err := httpclient.NewClientFromOptionsWithTransport(&httpclient.Options{
		Server: options.Address,
		Token:  options.Token,
	}, wrapper)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

// Put creates an asset or replaces a named asset.
func (c *Client) Put(ctx context.Context, target asset.Target, blob asset.Blob, options asset.PutOptions) (*asset.Asset, error) {
	if err := asset.ValidateBlob(blob); err != nil {
		return nil, err
	}
	request := c.client.Post(targetPath(target))
	if options.Name != "" {
		request = c.client.Put(assetPath(target, options.Name))
	}
	encodedMetadata, err := encodeMetadataHeader(options.Metadata)
	if err != nil {
		return nil, err
	}
	if encodedMetadata != "" {
		request.Header(metadataHeader, encodedMetadata)
	}
	if blob.Link != nil {
		request.JSON(linkPutRequest{
			Link:          *blob.Link,
			ContentType:   blob.ContentType,
			ContentLength: blob.ContentLength,
			FileName:      blob.FileName,
			ModTime:       blob.ModTime,
		}).Header("Content-Type", linkMediaType)
	} else {
		request.Body(blob.Content, blob.ContentType).ContentLength(blob.ContentLength)
		if blob.FileName != "" {
			request.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
				"filename": blob.FileName,
			}))
		}
		if !blob.ModTime.IsZero() {
			request.Header(modTimeHeader, blob.ModTime.UTC().Format(time.RFC3339Nano))
		}
	}
	result := &asset.Asset{}
	response, err := request.Return(result).Do(ctx)
	if err != nil {
		closeResponse(response)
		return nil, err
	}
	return result, nil
}

// Get returns metadata for one named asset.
func (c *Client) Get(ctx context.Context, target asset.Target, name string) (*asset.Asset, error) {
	result := &asset.Asset{}
	response, err := c.client.
		Get(assetPath(target, name) + "/metadata").
		Return(result).
		Do(ctx)
	if err != nil {
		closeResponse(response)
		return nil, err
	}
	return result, nil
}

// List returns one page of assets owned by a target.
func (c *Client) List(ctx context.Context, target asset.Target, options meta.ListOptions) (meta.Page[asset.Asset], error) {
	result := meta.Page[asset.Asset]{}
	response, err := c.client.
		Get(targetPath(target)).
		Queries(httpclient.ListOptionsToQuery(options)).
		Return(&result).
		Do(ctx)
	if err != nil {
		closeResponse(response)
		return meta.Page[asset.Asset]{}, err
	}
	return result, nil
}

// ReplaceMetadata replaces metadata without changing content identity.
func (c *Client) ReplaceMetadata(ctx context.Context, target asset.Target, name string, metadata map[string]string) (*asset.Asset, error) {
	result := &asset.Asset{}
	response, err := c.client.
		Put(assetPath(target, name) + "/metadata").
		JSON(metadata).
		Return(result).
		Do(ctx)
	if err != nil {
		closeResponse(response)
		return nil, err
	}
	return result, nil
}

func encodeMetadataHeader(metadata map[string]string) (string, error) {
	if metadata == nil {
		return "", nil
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeMetadataHeader(value string) (map[string]string, error) {
	if value == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	metadata := map[string]string{}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

// Resolve requests content once and returns either the redirect or body.
func (c *Client) Resolve(ctx context.Context, target asset.Target, name string, options asset.ResolveOptions) (*asset.Resolved, error) {
	request := c.client.
		Get(assetPath(target, name)).
		Query("link", strconv.FormatBool(options.PreferLink)).
		Client(&http.Client{
			Transport: c.client.RoundTripper,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		})
	if options.Version > 0 {
		request.Query("v", strconv.FormatInt(options.Version, 10))
	}
	if options.Range != "" {
		request.Header("Range", options.Range)
	}
	response, err := request.Do(ctx)
	if err != nil {
		closeResponse(response)
		return nil, err
	}
	current, err := assetFromResponse(response)
	if err != nil {
		closeResponse(response)
		return nil, fmt.Errorf("decode resolved asset: %w", err)
	}
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		defer response.Body.Close()
		location, err := response.Location()
		if err != nil {
			return nil, fmt.Errorf("resolve asset redirect: %w", err)
		}
		expiresAt := time.Time{}
		if value := response.Header.Get(linkExpiresAtHeader); value != "" {
			expiresAt, err = time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return nil, fmt.Errorf("decode asset link expiry: %w", err)
			}
		}
		return &asset.Resolved{
			Asset: current,
			Link: &asset.Link{
				URL:       location.String(),
				ExpiresAt: expiresAt,
			},
		}, nil
	}
	return &asset.Resolved{
		Asset:         current,
		Content:       response.Body,
		ContentLength: response.ContentLength,
		ContentRange:  response.Header.Get("Content-Range"),
	}, nil
}

// Delete removes one named asset.
func (c *Client) Delete(ctx context.Context, target asset.Target, name string) error {
	response, err := c.client.
		Delete(assetPath(target, name)).
		Do(ctx)
	closeResponse(response)
	return err
}

// DeleteAll removes every asset owned by one target.
func (c *Client) DeleteAll(ctx context.Context, target asset.Target) error {
	response, err := c.client.
		Delete(targetPath(target)).
		Do(ctx)
	closeResponse(response)
	return err
}

func targetPath(target asset.Target) string {
	return "/assets/" + url.PathEscape(target.Kind) + "/" + url.PathEscape(target.Name)
}

func assetPath(target asset.Target, name string) string {
	return targetPath(target) + "/" + url.PathEscape(name)
}

func assetFromResponse(response *http.Response) (asset.Asset, error) {
	payload, err := base64.RawURLEncoding.DecodeString(response.Header.Get(contentDescriptorHeader))
	if err != nil {
		return asset.Asset{}, err
	}
	current := asset.Asset{}
	if err := json.Unmarshal(payload, &current); err != nil {
		return asset.Asset{}, err
	}
	return current, nil
}

func closeResponse(response *http.Response) {
	if response != nil {
		response.Body.Close()
	}
}
