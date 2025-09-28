package api

import (
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

type StaticServerOptions struct {
	CacheExtensions []string
	CacheDuration   time.Duration
	AllowTryFiles   bool
	BasePath        string
}

func NewDefaultStaticServerOptions() *StaticServerOptions {
	return &StaticServerOptions{
		CacheExtensions: []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff2", ".woff", ".ttf", ".map"},
		CacheDuration:   7 * 24 * time.Hour, // 1 week
	}
}

func NewStaticServer(fs fs.FS, options *StaticServerOptions) *StaticServer {
	httpfs := http.FS(fs)
	return &StaticServer{fileserver: http.FileServer(httpfs), fs: httpfs, options: options}
}

type StaticServer struct {
	fileserver http.Handler
	fs         http.FileSystem
	options    *StaticServerOptions
}

func (u *StaticServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if u.options.BasePath != "" {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, u.options.BasePath)
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
	}
	if ext := path.Ext(r.URL.Path); slices.Contains(u.options.CacheExtensions, strings.ToLower(ext)) {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(u.options.CacheDuration.Seconds())))
	}
	if u.options.AllowTryFiles {
		// If the file doesn't exist, serve index.html
		// This allows SPA routing to work
		if _, err := u.fs.Open(path.Clean(r.URL.Path)); err != nil {
			r.URL.Path = ""
		}
	}
	u.fileserver.ServeHTTP(w, r)
}
