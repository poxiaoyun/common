package filebrowser

import (
	"context"
	"testing"

	libfs "xiaoshiai.cn/common/fs"
	"xiaoshiai.cn/common/rest/api"
)

func TestFileBrowserAPI(t *testing.T) {
	ctx := context.Background()

	fsys := libfs.SubFS{
		Fsys: &libfs.OSFileSystem{},
		Dir:  "/tmp",
	}
	fs := NewFsFileBrowser(fsys)
	fbapi := &FileBrowserAPI{FileBrowser: fs}

	api := api.New().
		Filter(
			api.NewCORSFilter(),
		).
		Group(fbapi.Group())

	if err := api.Serve(ctx, ":8089"); err != nil {
		t.Fatal(err)
	}
}
