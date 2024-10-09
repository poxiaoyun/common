package api

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// NewCompressionFilter returns a filter that compresses the response body
func NewCompressionFilter() Filter {
	gzipPool := &sync.Pool{
		New: func() interface{} {
			gw, err := gzip.NewWriterLevel(nil, gzip.BestSpeed)
			if err != nil {
				panic(err)
			}
			return gw
		},
	}
	flatePool := &sync.Pool{
		New: func() interface{} {
			fw, err := flate.NewWriter(nil, flate.BestSpeed)
			if err != nil {
				panic(err)
			}
			return fw
		},
	}
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		var wrappedWriter io.Writer
		encoding := r.Header.Get("Accept-Encoding")
		accept := ""
		for len(encoding) > 0 {
			var token string
			if next := strings.Index(encoding, ","); next != -1 {
				token = encoding[:next]
				encoding = encoding[next+1:]
			} else {
				token = encoding
				encoding = ""
			}
			if strings.TrimSpace(token) != "" {
				accept = token
				break
			}
		}
		switch accept {
		case "gzip":
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")

			gw := gzipPool.Get().(*gzip.Writer)
			gw.Reset(w)
			defer gzipPool.Put(gw)

			wrappedWriter = gw
		case "deflate":
			w.Header().Set("Content-Encoding", "deflate")
			w.Header().Add("Vary", "Accept-Encoding")
			fw := flatePool.Get().(*flate.Writer)
			fw.Reset(w)
			defer flatePool.Put(fw)

			wrappedWriter = fw
		}
		if wrappedWriter != nil {
			w = &CompresseWriter{ResponseWriter: w, w: wrappedWriter}
		}
		next.ServeHTTP(w, r)
	})
}

type CompresseWriter struct {
	http.ResponseWriter
	w io.Writer
}

func (cw *CompresseWriter) Flush() {
	if flusher, ok := cw.w.(http.Flusher); ok {
		flusher.Flush()
	}
	if flusher, ok := cw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
