package filebrowser

import (
	"context"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	libfs "xiaoshiai.cn/common/fs"
	"xiaoshiai.cn/common/rest/api"
)

const FileSystemSessionPrefix = "session-"

func NewFsFileBrowser(fsys libfs.FileSystem) *FSFileBrowser {
	return &FSFileBrowser{FS: fsys, TempDir: "/tmp"}
}

var _ WebBrowser = &FSFileBrowser{}

type FSFileBrowser struct {
	FS      libfs.FileSystem
	TempDir string
}

// UploadPart implements WebBrowser.
func (f *FSFileBrowser) UploadPart(ctx context.Context, uploadID string, offset, total int64, content FileContent) error {
	fullfilename := filepath.Join(f.TempDir, uploadID, content.Name)

	ff, err := f.FS.OpenFile(fullfilename, libfs.O_CREATE|libfs.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer ff.Close()

	if _, err := ff.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.Copy(ff, content.Content); err != nil {
		return err
	}
	return nil
}

// OpenMultiPartUpload implements WebBrowser.
func (f *FSFileBrowser) OpenMultiPartUpload(ctx context.Context, path string) (string, error) {
	tmpname := time.Now().Format("20060102-150405")
	if err := f.FS.MkdirAll(filepath.Join(f.TempDir, tmpname), 0o777); err != nil {
		return "", err
	}
	return tmpname, nil
}

// CancelMultiPartUpload implements WebBrowser.
func (f *FSFileBrowser) CancelMultiPartUpload(ctx context.Context, uploadID string) error {
	tempdir := filepath.Join(f.TempDir, uploadID)
	if err := f.FS.RemoveAll(tempdir); err != nil {
		return err
	}
	return nil
}

// CompleteMultiPartUpload implements WebBrowser.
func (f *FSFileBrowser) CompleteMultiPartUpload(ctx context.Context, uploadID string) error {
	srcdir := filepath.Join(f.TempDir, uploadID)
	if err := libfs.Copy(f.FS, "/", srcdir); err != nil {
		return err
	}
	if err := f.FS.RemoveAll(srcdir); err != nil {
		return err
	}
	return nil
}

// UploadFile implements WebBrowser.
func (f *FSFileBrowser) UploadFile(ctx context.Context, path string, content FileContent) error {
	file, err := f.FS.OpenFile(path, libfs.O_CREATE|libfs.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, content.Content)
	return err
}

// CopyFile implements WebBrowser.
func (f *FSFileBrowser) CopyFile(ctx context.Context, src string, dest string) error {
	return libfs.CopyFile(f.FS, src, dest)
}

// DeleteFile implements WebBrowser.
func (f *FSFileBrowser) DeleteFile(ctx context.Context, path string, all bool) error {
	if all {
		return f.FS.RemoveAll(path)
	}
	return f.FS.Remove(path)
}

// DownloadFile implements WebBrowser.
func (f *FSFileBrowser) DownloadFile(ctx context.Context, path string, options DownloadFileOptions) (*FileContent, error) {
	info, err := f.FS.Stat(path)
	if err != nil {
		return nil, err
	}
	ff, err := f.FS.OpenFile(path, libfs.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &FileContent{
		Name:          info.Name(),
		Content:       ff,
		ContentType:   mime.TypeByExtension(filepath.Ext(info.Name())),
		ContentLength: info.Size(),
	}, nil
}

// LinkFile implements WebBrowser.
func (f *FSFileBrowser) LinkFile(ctx context.Context, src string, dest string) error {
	return f.FS.Symlink(dest, src)
}

// MoveFile implements WebBrowser.
func (f *FSFileBrowser) MoveFile(ctx context.Context, src string, dest string) error {
	return f.FS.Rename(src, dest)
}

func (f *FSFileBrowser) StateFile(ctx context.Context, name string, options StateFileOptions) (*TreeItem, error) {
	info, err := f.FS.Stat(name)
	if err != nil {
		return nil, err
	}
	ret := &TreeItem{
		Name:       info.Name(),
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Permission: info.Mode().String(),
		Type:       TreeItemTypeUnknown,
	}
	if info.IsDir() {
		items, continuekey, err := f.listDir(name, options)
		if err != nil {
			return nil, err
		}
		ret.Type = TreeItemTypeDir
		ret.Childern = items
		ret.Continue = continuekey
		return ret, nil
	}
	if info.Mode().IsRegular() {
		ret.Type = TreeItemTypeFile
		ret.ContentType = mime.TypeByExtension(path.Ext(info.Name()))
		return ret, nil
	}
	if libfs.IsSymbolicLink(info) {
		target, err := f.FS.Readlink(name)
		if err != nil {
			return nil, err
		}
		ret.Type = TreeItemTypeLink
		ret.Target = target
		return ret, nil
	}
	return ret, nil
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

func (f *FSFileBrowser) listDir(name string, options StateFileOptions) ([]TreeItem, string, error) {
	entries, err := f.FS.ReadDir(name)
	if err != nil {
		return nil, "", err
	}
	items := make([]TreeItem, 0, len(entries))

	limit := options.Limit
	for _, entry := range entries {
		if entry.IsDir() {
			items = append(items, TreeItem{Name: entry.Name(), Type: TreeItemTypeDir})
		} else {
			info, err := entry.Info()
			if err != nil {
				items = append(items, TreeItem{Name: entry.Name(), Type: TreeItemTypeUnknown})
			} else {
				items = append(items, f.fileinfoToTreeItem(name, info))
			}
		}
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	// search
	items = slices.DeleteFunc(items, func(item TreeItem) bool {
		return options.Search != "" && strings.Contains(item.Name, options.Search)
	})
	// sort
	api.SortByFunc(options.Sort,
		func(item TreeItem) string { return item.Name },
		func(item TreeItem) time.Time { return item.ModTime })
	// pagination
	if options.Continue != "" {
		i := slices.IndexFunc(items, func(item TreeItem) bool { return item.Name == options.Continue })
		if i >= 0 {
			items = items[i:]
		}
	}
	// limit
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		return items, items[len(items)-1].Name, nil
	}
	return items, "", nil
}

func (f *FSFileBrowser) fileinfoToTreeItem(dir string, info libfs.FileInfo) TreeItem {
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
		item.Target, _ = f.FS.Readlink(path.Join(dir, info.Name()))
		item.ContentType = mime.TypeByExtension(path.Ext(item.Target))
	}
	return item
}
