package filebrowser

import (
	"context"
	stderrors "errors"
	"io"
	"mime"
	"net/http"
	"path"
	"time"

	"xiaoshiai.cn/common/errors"
	libfs "xiaoshiai.cn/common/fs"
	"xiaoshiai.cn/common/rest/api"
)

type FileBrowser struct {
	FS      libfs.FileSystem
	TempDir string
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
	ContentType string            `json:"contentType"`
	ModTime     time.Time         `json:"modTime"`
	Target      string            `json:"target,omitempty"` // for symlink
	Attributes  map[string]string `json:"attributes,omitempty"`
}

func NewFileBrowser(fsys libfs.FileSystem) *FileBrowser {
	return &FileBrowser{FS: fsys, TempDir: "/tmp"}
}

func (b *FileBrowser) ModifiFile(w http.ResponseWriter, r *http.Request) {
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
func (b *FileBrowser) UploadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		mr, err := r.MultipartReader()
		if err != nil {
			if stderrors.Is(err, http.ErrNotMultipart) {
				content := FileContent{
					Name:        name,
					ContentType: r.Header.Get("Content-Type"),
					Data:        r.Body,
					Size:        r.ContentLength,
				}
				if err := patchFile(ctx, fsys, content); err != nil {
					return nil, err
				}
				return nil, nil
			}
			return nil, err
		}

		firstPart, err := mr.NextRawPart()
		if err != nil {
			if err == io.EOF {
				return nil, errors.NewBadRequest("no file uploaded")
			}
			return nil, err
		}

		formname := firstPart.FormName()
		if formname == "filepond" {
			// start filepond upload
			randname := "filepond-" + time.Now().Format("20060102-150405")
			return randname, nil
		}

		for {
			err := func() error {
				part, err := mr.NextRawPart()
				if err != nil {
					return err
				}

				formname := part.FormName()
				if formname == "filepond" {
				}

				defer part.Close()

				dst, err := fsys.OpenFile(name, libfs.O_CREATE|libfs.O_WRONLY, 0o666)
				if err != nil {
					return err
				}
				defer dst.Close()

				_, err = io.Copy(dst, part)
				return err
			}()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}

		}

		return nil, nil
	})
}

// ChunckUploadStart
// it use https://pqina.nl/filepond/docs/api/server/#process-chunks
func (b *FileBrowser) ChunckUploadStart(w http.ResponseWriter, r *http.Request) {
}

func (b *FileBrowser) ChunckUploadFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		patch := api.Query(r, "patch", "")
		if patch != "" {
			content := FileContent{
				// upload-name
				Name:   api.Header(r, "Upload-Name", ""),
				Offset: api.Header(r, "Upload-Offset", int64(0)),
				Size:   api.Header(r, "Upload-Length", int64(0)),
				Data:   r.Body,
			}
			return nil, patchFile(ctx, fsys, content)
		}

		mr, err := r.MultipartReader()
		if err != nil {
			if stderrors.Is(err, http.ErrNotMultipart) {
				content := FileContent{
					Name:        name,
					ContentType: r.Header.Get("Content-Type"),
					Data:        r.Body,
					Size:        r.ContentLength,
				}
				if err := patchFile(ctx, fsys, content); err != nil {
					return nil, err
				}
				return nil, nil
			}
			return nil, err
		}

		firstPart, err := mr.NextRawPart()
		if err != nil {
			if err == io.EOF {
				return nil, errors.NewBadRequest("no file uploaded")
			}
			return nil, err
		}

		formname := firstPart.FormName()
		if formname == "filepond" {
			// start filepond upload
			randname := "filepond-" + time.Now().Format("20060102-150405")
			http.Error(w, randname, http.StatusOK)
		}
		return nil, nil
	})
}

func (b *FileBrowser) LinkFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		dest := api.Query(r, "dest", "")
		if dest == "" {
			return nil, errors.NewBadRequest("dest is required")
		}
		return nil, fsys.Symlink(dest, name)
	})
}

func (b *FileBrowser) CopyFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		dest := api.Query(r, "dest", "")
		if dest == "" {
			return nil, errors.NewBadRequest("dest is required")
		}
		return nil, libfs.Copy(fsys, dest, name)
	})
}

func (b *FileBrowser) DeleteFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		isall := api.Query(r, "all", false)
		if isall {
			return nil, fsys.RemoveAll(name)
		} else {
			return nil, fsys.Remove(name)
		}
	})
}

func (b *FileBrowser) RenameFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		dest := api.Query(r, "dest", "")
		if dest == "" {
			return nil, errors.NewBadRequest("dest is required")
		}
		return nil, fsys.Rename(name, dest)
	})
}

func (b *FileBrowser) GetFile(w http.ResponseWriter, r *http.Request) {
	b.OnPath(w, r, func(ctx context.Context, fsys libfs.FileSystem, name string) (any, error) {
		info, err := fsys.Stat(name)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			items, err := listDir(fsys, name)
			if err != nil {
				return nil, err
			}
			paged := api.PageFromRequest(r, items,
				func(item TreeItem) string { return item.Name },
				func(item TreeItem) time.Time { return item.ModTime })
			return paged, nil
		}
		if info.Mode().IsRegular() {
			return downloadFile(w, r, fsys, info, name)
		}
		if libfs.IsSymbolicLink(info) {
			target, err := fsys.Readlink(name)
			if err != nil {
				return nil, err
			}
			info, err = fsys.Stat(target)
			if err != nil {
				return nil, err
			}
			return downloadFile(w, r, fsys, info, target)
		}
		return fileinfoToTreeItem(fsys, name, info), nil
	})
}

type FileContent struct {
	Name        string
	Offset      int64
	Size        int64
	Data        io.ReadCloser
	ContentType string
}

func patchFile(_ context.Context, fsys libfs.FileSystem, content FileContent) error {
	dst, err := fsys.OpenFile(content.Name, libfs.O_CREATE|libfs.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer dst.Close()

	if content.Offset > 0 {
		if _, err := dst.Seek(content.Offset, io.SeekStart); err != nil {
			return err
		}
	}
	src := io.LimitReader(content.Data, content.Size)
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func downloadFile(w http.ResponseWriter, r *http.Request, fsys libfs.FileSystem, info libfs.FileInfo, name string) (any, error) {
	file, err := libfs.Open(fsys, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	http.ServeContent(w, r, name, info.ModTime(), file)
	return nil, nil
}

func listDir(fsys libfs.FileSystem, name string) ([]TreeItem, error) {
	entries, err := fsys.ReadDir(name)
	if err != nil {
		return nil, err
	}

	ret := make([]TreeItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ret = append(ret, TreeItem{
				Name: entry.Name(),
				Type: TreeItemTypeDir,
			})
		} else {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			ret = append(ret, fileinfoToTreeItem(fsys, name, info))
		}
	}
	return ret, nil
}

func fileinfoToTreeItem(fsys libfs.FileSystem, dir string, info libfs.FileInfo) TreeItem {
	item := TreeItem{
		Name:    info.Name(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Type:    TreeItemTypeUnknown,
	}
	if info.Mode().IsRegular() {
		item.Type = TreeItemTypeFile
		item.ContentType = mime.TypeByExtension(path.Ext(item.Name))
		item.Size = info.Size()
	} else if info.IsDir() {
		item.Type = TreeItemTypeDir
	} else if info.Mode()&libfs.ModeSymlink != 0 {
		item.Type = TreeItemTypeLink
		item.Target, _ = fsys.Readlink(path.Join(dir, info.Name()))
		item.ContentType = mime.TypeByExtension(path.Ext(item.Target))
	}
	return item
}

func (b *FileBrowser) OnPath(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, fsys libfs.FileSystem, path string) (any, error)) {
	obj, err := fn(r.Context(), b.FS, api.Path(r, "path", ""))
	if err != nil {
		api.Error(w, err)
		return
	}
	api.Success(w, obj)
}

func (b *FileBrowser) Group() api.Group {
	return api.NewGroup("").
		Tag("FileBrowser").
		Route(
			api.GET("/{path}*").To(b.GetFile).Doc("download file or list dir"),
			api.POST("/{path}*").To(b.ModifiFile).
				Param(
					api.QueryParam("action", "action on the file").In("rename", "link", "copy").Optional(),
					api.QueryParam("dest", "destination path, dest is path which rename/copy/link to").Optional(),
					api.BodyParam("body", "file content"),
				),
			api.PATCH("/{path}*").To(b.ChunckUploadFile),
			api.DELETE("/{path}*").To(b.DeleteFile),
		)
}
