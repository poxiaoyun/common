package filebrowser

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	libfs "xiaoshiai.cn/common/fs"
	"xiaoshiai.cn/common/rest/api"
)

func TestFileBrowserAPI(t *testing.T) {
	root := t.TempDir()
	const content = "filebrowser test content"
	if err := os.WriteFile(filepath.Join(root, "example.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	fsys := libfs.SubFS{
		Fsys: &libfs.OSFileSystem{},
		Dir:  root,
	}
	fs := NewFsFileBrowser(fsys)
	fbapi := &FileBrowserAPI{FileBrowser: fs}

	handler := api.New().
		Filter(
			api.NewCORSFilter(),
		).
		Group(fbapi.Group()).
		Build()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/files/example.txt", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /files/example.txt status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != content {
		t.Fatalf("GET /files/example.txt body = %q, want %q", response.Body.String(), content)
	}
}
