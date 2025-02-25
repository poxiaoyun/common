package filebrowser

import (
	"context"
	stderrors "errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
)

const FilepondName = "filepond"

type FileBrowserAPI struct {
	FileBrowser WebBrowser
}

// https://pqina.nl/filepond/docs/api/server/#process
func (b *FileBrowserAPI) UploadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		if fpath == "" {
			return nil, errors.NewBadRequest("file path is required on upload")
		}
		// open multipart upload
		if isuploads := api.Query(r, "uploads", false); isuploads {
			newsession, err := fsys.OpenMultiPartUpload(ctx, fpath, OpenMultiPartUploadMetadata{})
			if err != nil {
				return nil, err
			}
			ret := OpenMultiPartUploadResponse{
				UploadID: newsession,
			}
			return ret, nil
		}
		// complete the upload session
		if uploadid := api.Query(r, "uploadId", ""); uploadid != "" {
			if err := fsys.CompleteMultiPartUpload(ctx, uploadid); err != nil {
				return nil, err
			}
			return api.Empty, nil
		}

		// upload file multipart
		mr, err := r.MultipartReader()
		if err != nil {
			if !stderrors.Is(err, http.ErrNotMultipart) {
				return nil, err
			}
			// direct upload file
			dir, filename := path.Split(fpath)
			content := FileContent{
				Name:          filename,
				ContentType:   r.Header.Get("Content-Type"),
				Content:       r.Body,
				ContentLength: r.ContentLength,
			}
			if err := fsys.UploadFile(ctx, dir, content); err != nil {
				return nil, err
			}
			return api.Empty, nil
		}

		for {
			part, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			// start filepond upload
			if part.FormName() == FilepondName {
				uploadLength, _ := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
				// filepond set uploadLength on multipart upload
				if uploadLength > 0 {
					newsession, err := fsys.OpenMultiPartUpload(ctx, fpath, OpenMultiPartUploadMetadata{})
					if err != nil {
						return nil, err
					}
					return newsession, nil
				}
			}
			// the first part will be finpond metadata and no filename
			filename := part.FileName()
			if filename == "" {
				continue
			}
			// otherwise, direct upload file
			content := FileContent{}
			content.Name = filename
			content.ContentType = part.Header.Get("Content-Type")
			content.Content = part
			content.ContentLength, _ = strconv.ParseInt(part.Header.Get("Content-Length"), 10, 64)
			if err := fsys.UploadFile(ctx, fpath, content); err != nil {
				return nil, err
			}
			// only the first file will be uploaded
			return api.Empty, nil
		}
		return nil, errors.NewBadRequest("no file uploaded")
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

type OpenMultiPartUploadResponse struct {
	UploadID string `json:"uploadID"`
	Filename string `json:"filename"`
}

func (b *FileBrowserAPI) Upload(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		if fpath == "" {
			return nil, errors.NewBadRequest("file path is required on upload")
		}
		newsession, err := fsys.OpenMultiPartUpload(ctx, fpath, OpenMultiPartUploadMetadata{})
		if err != nil {
			return nil, err
		}
		response := OpenMultiPartUploadResponse{
			UploadID: newsession,
			Filename: fpath,
		}
		return response, nil
	})
}

func (b *FileBrowserAPI) CreateChunckUpload(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		if fpath == "" {
			return nil, errors.NewBadRequest("file path is required on upload")
		}
		newsession, err := fsys.OpenMultiPartUpload(ctx, fpath, OpenMultiPartUploadMetadata{})
		if err != nil {
			return nil, err
		}
		response := OpenMultiPartUploadResponse{
			UploadID: newsession,
			Filename: fpath,
		}
		return response, nil
	})
}

func (b *FileBrowserAPI) UploadChunck(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		uploadid := api.Query(r, "uploadID", "")
		if uploadid == "" {
			return nil, errors.NewBadRequest("uploadID is required")
		}
		offset := api.Header(r, "Upload-Offset", int64(0))
		uploadLength := api.Header(r, "Upload-Length", int64(0))
		content := PartialFileContent{
			Name:          api.Header(r, "Upload-Name", ""),
			ContentLength: r.ContentLength,
			Content:       r.Body,
			Offset:        offset,
		}
		if err := fsys.UploadPart(ctx, uploadid, content); err != nil {
			return nil, err
		}
		if offset+content.ContentLength >= uploadLength {
			if err := fsys.CompleteMultiPartUpload(ctx, uploadid); err != nil {
				return nil, err
			}
		}
		return api.Empty, nil
	})
}

// it use https://pqina.nl/filepond/docs/api/server/#process-chunks
// ChunckUploadFile
func (b *FileBrowserAPI) ChunckUploadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		// part number style
		if uploadID := api.Query(r, "uploadId", ""); uploadID != "" {
			offset := api.Header(r, "Upload-Offset", int64(0))
			uploadLength := api.Header(r, "Upload-Length", int64(0))
			content := PartialFileContent{
				Name:          api.Header(r, "Upload-Name", ""),
				ContentLength: r.ContentLength,
				Content:       r.Body,
				Offset:        offset,
			}
			if err := fsys.UploadPart(ctx, uploadID, content); err != nil {
				return nil, err
			}
			if offset+content.ContentLength >= uploadLength {
				if err := fsys.CompleteMultiPartUpload(ctx, uploadID); err != nil {
					return nil, err
				}
			}
			return api.Empty, nil
		}
		// offset style
		if patch := api.Query(r, "patch", ""); patch != "" {
			offset := api.Header(r, "Upload-Offset", int64(0))
			uploadLength := api.Header(r, "Upload-Length", int64(0))
			content := PartialFileContent{
				Name:          api.Header(r, "Upload-Name", ""),
				ContentLength: r.ContentLength,
				Content:       r.Body,
			}
			if err := fsys.UploadPart(ctx, patch, content); err != nil {
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

func (b *FileBrowserAPI) CompleteChunckUpload(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys WebBrowser, fpath string) (any, error) {
		uploadid := api.Query(r, "uploadID", "")
		if uploadid == "" {
			return nil, errors.NewBadRequest("uploadID is required")
		}
		if err := fsys.CompleteMultiPartUpload(ctx, uploadid); err != nil {
			return nil, err
		}
		return api.Empty, nil
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
			api.HEAD("/files/{path}*").
				Operation("options file/dir").
				To(b.HeadFile),

			api.GET("/files/{path}*").
				Operation("state file/dir or download file").
				To(b.GetFile).
				Param(
					api.QueryParam("stat", "stat the file").Optional(),
				).
				Response(TreeItem{}),

			api.POST("/files/{path}*").
				Operation("upload file").
				To(b.UploadFile).
				Param(
					api.QueryParam("uploads", "open multipart upload").Optional(),
					api.QueryParam("uploadId", "complete the upload session").Optional(),
					api.BodyParam("body", "file content to upload").Optional(),
				),

			api.PUT("/files/{path}*").
				Operation("chunck upload file").
				Param(
					api.QueryParam("uploadId", "upload session id").Optional(),
				).
				To(b.ChunckUploadFile),

			api.PATCH("/files/{path}*").
				Operation("filepond chunck upload file").
				Param(
					api.QueryParam("patch", "upload session id").Optional(),
				).
				To(b.ChunckUploadFile),

			api.DELETE("/files/{path}*").
				Operation("delete file or dir").
				Param(
					api.QueryParam("all", "delete all child files").Optional(),
					api.QueryParam("uploadId", "delete upload session").Optional(),
				).
				To(b.DeleteFile),
		)
}
