package filebrowser

import (
	"context"
	"io"
	"time"
)

type FileContent struct {
	// basic
	Name          string
	Content       io.ReadCloser
	ContentType   string
	ContentLength int64
	// optinal
	ContentRange string
	LastModified time.Time
	Etag         string
}

type StateFileOptions struct {
	Continue string
	Limit    int
	Search   string
	Sort     string
}

type DownloadFileOptions struct {
	Range           string
	IfMatch         string
	IfNoneMatch     string
	IfModifiedSince time.Time
}

type TreeItemType string

const (
	TreeItemTypeFile    TreeItemType = "file"
	TreeItemTypeDir     TreeItemType = "dir"
	TreeItemTypeLink    TreeItemType = "link"
	TreeItemTypeUnknown TreeItemType = "unknown"
)

type TreeItem struct {
	Name        string            `json:"name"`
	Type        TreeItemType      `json:"type"`
	Size        int64             `json:"size"`
	Etag        string            `json:"etag,omitempty"`
	ContentType string            `json:"contentType"`
	Permission  string            `json:"permission"`
	ModTime     time.Time         `json:"modTime"`
	Target      string            `json:"target,omitempty"` // for symlink
	Attributes  map[string]string `json:"attributes,omitempty"`
	Childern    []TreeItem        `json:"childern,omitempty"`
	Continue    string            `json:"continue,omitempty"` // for pagination
}

type WebBrowser interface {
	// File operations
	StateFile(ctx context.Context, path string, options StateFileOptions) (*TreeItem, error)
	DownloadFile(ctx context.Context, path string, options DownloadFileOptions) (*FileContent, error)
	DeleteFile(ctx context.Context, path string, all bool) error
	MoveFile(ctx context.Context, src, dest string) error
	CopyFile(ctx context.Context, src, dest string) error
	LinkFile(ctx context.Context, src, dest string) error
	UploadFile(ctx context.Context, path string, content FileContent) error

	// Multipart upload
	OpenMultiPartUpload(ctx context.Context, path string) (string, error)
	UploadPart(ctx context.Context, uploadID string, offset, total int64, content FileContent) error
	CompleteMultiPartUpload(ctx context.Context, uploadID string) error
	CancelMultiPartUpload(ctx context.Context, uploadID string) error
}
