package filebrowser

import (
	"context"
	stderrors "errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
)

type FileBrowserAPI struct {
	FileBrowser WebBrowser
}

func (b *FileBrowserAPI) PostFile(w http.ResponseWriter, r *http.Request) {
	action := api.Query(r, "action", "")
	switch action {
	case "rename":
		b.RenameFile(w, r)
	case "link":
		b.LinkFile(w, r)
	case "upload", "":
		b.UploadFile(w, r)
	case "cp", "copy":
		b.CopyFile(w, r)
	default:
		api.Error(w, errors.NewBadRequest("unknown action: "+action))
	}
}

// https://pqina.nl/filepond/docs/api/server/#process
func (b *FileBrowserAPI) UploadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		if fpath == "" {
			return nil, errors.NewBadRequest("file path is required on upload")
		}
		mr, err := r.MultipartReader()
		if err != nil {
			if !stderrors.Is(err, http.ErrNotMultipart) {
				return nil, err
			}
			// direct upload
			content := FileContent{
				Name:          fpath,
				ContentType:   r.Header.Get("Content-Type"),
				Content:       r.Body,
				ContentLength: r.ContentLength,
			}
			if err := fsys.UploadFile(ctx, fpath, content); err != nil {
				return nil, err
			}
			return api.Empty, nil
		}

		var filepondSession string
		for {
			part, err := mr.NextRawPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			switch part.FormName() {
			case "filepond":
				// start filepond upload
				if filepondSession == "" {
					newsession, err := fsys.OpenMultiPartUpload(ctx, fpath)
					if err != nil {
						return nil, err
					}
					filepondSession = newsession
					continue
				}
			}
			filename := part.FileName()
			if filename == "" {
				return nil, errors.NewBadRequest("no filename in form")
			}
			content := FileContent{}
			content.Name = filename
			content.ContentType = part.Header.Get("Content-Type")
			content.Content = part
			if err := fsys.UploadFile(ctx, fpath, content); err != nil {
				return nil, err
			}
		}
		if filepondSession != "" {
			return filepondSession, nil
		}
		return api.Empty, nil
	})
}

// it use https://pqina.nl/filepond/docs/api/server/#process-chunks
// ChunckUploadFile
func (b *FileBrowserAPI) ChunckUploadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		if patch := api.Query(r, "patch", ""); patch != "" {
			offset := api.Header(r, "Upload-Offset", int64(0))
			uploadLength := api.Header(r, "Upload-Length", int64(0))
			content := FileContent{
				Name:          api.Header(r, "Upload-Name", ""),
				ContentLength: r.ContentLength,
				Content:       r.Body,
			}
			if err := fsys.UploadPart(ctx, patch, offset, uploadLength, content); err != nil {
				return nil, err
			}
			if offset+content.ContentLength >= uploadLength {
				if err := fsys.CompleteMultiPartUpload(ctx, patch); err != nil {
					return nil, err
				}
			}
			return api.Empty, nil
		}
		return nil, errors.NewBadRequest("patch is required")
	})
}

func (b *FileBrowserAPI) LinkFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, name string) (any, error) {
		dest := api.Query(r, "dest", "")
		if dest == "" {
			return nil, errors.NewBadRequest("dest is required")
		}
		return nil, fsys.LinkFile(ctx, name, dest)
	})
}

func (b *FileBrowserAPI) CopyFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, name string) (any, error) {
		dest := api.Query(r, "dest", "")
		if dest == "" {
			return nil, errors.NewBadRequest("dest is required")
		}
		return nil, fsys.CopyFile(ctx, name, dest)
	})
}

func (b *FileBrowserAPI) DeleteFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, name string) (any, error) {
		return nil, fsys.DeleteFile(ctx, name, api.Query(r, "all", false))
	})
}

func (b *FileBrowserAPI) RenameFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, name string) (any, error) {
		dest := api.Query(r, "dest", "")
		if dest == "" {
			return nil, errors.NewBadRequest("dest is required")
		}
		return nil, fsys.MoveFile(ctx, name, dest)
	})
}

func (b *FileBrowserAPI) HeadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, name string) (any, error) {
		if patch := api.Query(r, "patch", ""); patch != "" {
			// check upload session exist
			// server responds with Upload-Offset set to the next expected chunk offset in bytes.
		}
		if _, err := fsys.StateFile(ctx, name, StateFileOptions{}); err != nil {
			if stderrors.Is(err, fs.ErrNotExist) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		} else {
			w.WriteHeader(http.StatusOK)
		}
		return nil, nil
	})
}

func (b *FileBrowserAPI) GetFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, name string) (any, error) {
		if api.Query(r, "stat", false) {
			opt := StateFileOptions{
				Continue: api.Query(r, "continue", ""),
				Limit:    api.Query(r, "limit", 0),
				Search:   api.Query(r, "search", ""),
				Sort:     api.Query(r, "sort", ""),
			}
			return fsys.StateFile(ctx, name, opt)
		}
		content, err := fsys.DownloadFile(ctx, name, DownloadFileOptions{
			Range:           r.Header.Get("Range"),
			IfMatch:         r.Header.Get("If-Match"),
			IfNoneMatch:     r.Header.Get("If-None-Match"),
			IfModifiedSince: api.Header(r, "If-Modified-Since", time.Time{}),
		})
		if err != nil {
			return nil, err
		}
		if content.Content != nil {
			defer content.Content.Close()
		}

		w.Header().Set("Content-Type", content.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(content.ContentLength, 10))
		if content.ContentRange != "" {
			w.Header().Set("Content-Range", content.ContentRange)
		}
		if !content.LastModified.IsZero() {
			w.Header().Set("Last-Modified", content.LastModified.Format(time.RFC1123))
		}
		if content.Etag != "" {
			w.Header().Set("Etag", content.Etag)
		}
		if content.Content != nil {
			io.Copy(w, content.Content)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
		return nil, nil
	})
}

func (b *FileBrowserAPI) OnPath(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, fsys WebBrowser, path string) (any, error)) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		return fn(ctx, b.FileBrowser, api.Path(r, "path", ""))
	})
}

func (b *FileBrowserAPI) Group() api.Group {
	return api.
		NewGroup("").
		Tag("FileBrowser").
		Route(
			api.HEAD("/{path}*").
				Operation("options file/dir").
				To(b.HeadFile),

			api.GET("/{path}*").
				Operation("state file/dir or download file").
				To(b.GetFile).
				Param(
					api.QueryParam("stat", "stat the file").Optional(),
					api.QueryParam("download", "download the file").Optional(),
				).
				Response(TreeItem{}),

			api.POST("/{path}*").
				Operation("modify file").
				To(b.PostFile).
				Param(
					api.QueryParam("action", "action on the file").In("rename", "link", "copy", "state").Optional(),
					api.QueryParam("dest", "destination path, dest is path which rename/copy/link to").Optional(),
					api.BodyParam("body", "file content to upload").Optional(),
				),

			api.PATCH("/{path}*").
				Operation("chunck upload file").
				To(b.ChunckUploadFile),

			api.DELETE("/{path}*").
				Operation("delete file or dir").
				Param(api.QueryParam("all", "delete all child files").Optional()).
				To(b.DeleteFile),
		)
}
