package api

import (
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// HttpRange specifies the byte range to be sent to the client.
type HttpRange struct {
	start, length int64
}

func (r HttpRange) ContentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

var ErrNoOverlap = stderrors.New("no overlapping ranges")

// ParseRange parses a Range header string as per RFC 7233.
// ErrNoOverlap is returned if none of the ranges overlap.
func ParseRange(s string, size int64) ([]HttpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, stderrors.New("invalid range")
	}
	var ranges []HttpRange
	noOverlap := false
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = textproto.TrimString(ra)
		if ra == "" {
			continue
		}
		start, end, ok := strings.Cut(ra, "-")
		if !ok {
			return nil, stderrors.New("invalid range")
		}
		start, end = textproto.TrimString(start), textproto.TrimString(end)
		var r HttpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file,
			// and we are dealing with <suffix-length>
			// which has to be a non-negative integer as per
			// RFC 7233 Section 2.1 "Byte-Ranges".
			if end == "" || end[0] == '-' {
				return nil, stderrors.New("invalid range")
			}
			i, err := strconv.ParseInt(end, 10, 64)
			if i < 0 || err != nil {
				return nil, stderrors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i < 0 {
				return nil, stderrors.New("invalid range")
			}
			if i >= size {
				// If the range begins after the size of the content,
				// then it does not overlap.
				noOverlap = true
				continue
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, stderrors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	if noOverlap && len(ranges) == 0 {
		// The specified ranges did not overlap with the content.
		return nil, ErrNoOverlap
	}
	return ranges, nil
}

// ServeContent serves content from the provided [io.Reader].
// It is another version of [http.ServeContent] with support for *non-seekable* content.
// It serves the content with proper handling of conditional requests and range requests.
// e.g. If-Modified-Since, If-Unmodified-Since, If-Match, If-None-Match, If-Range headers.
//
// It only supports single range requests for non-seekable, to support multiple range requests, use [http.ServeContent] with io.Seeker.
// Make sure to set the "Last-Modified" "Etag" headers appropriately before calling this function.
//
// usage:
//
//	w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
//	w.Header().Set("Etag", etag)
//	ServeContent(w, r, contentReader, contentSize)
func ServeContent(w http.ResponseWriter, r *http.Request, content io.Reader, size int64) {
	modtime := time.Time{}
	modetimestr := w.Header().Get("Last-Modified")
	if modetimestr != "" {
		t, err := http.ParseTime(modetimestr)
		if err == nil {
			modtime = t
		}
	}

	// This function carefully follows RFC 7232 section 6.
	ch := checkIfMatch(w, r)
	if ch == condNone {
		ch = checkIfUnmodifiedSince(r, modtime)
	}
	if ch == condFalse {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	switch checkIfNoneMatch(w, r) {
	case condFalse:
		if r.Method == "GET" || r.Method == "HEAD" {
			writeNotModified(w)
			return
		} else {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
	case condNone:
		if checkIfModifiedSince(r, modtime) == condFalse {
			writeNotModified(w)
			return
		}
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" && checkIfRange(w, r, modtime) == condFalse {
		rangeHeader = ""
	}
	code := http.StatusOK
	ranges, err := ParseRange(rangeHeader, size)
	switch err {
	case nil:
	case ErrNoOverlap:
		if size == 0 {
			ranges = nil
			break
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		fallthrough
	default:
		serveFileError(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if len(ranges) > 1 {
		serveFileError(w, "multi range is not supported", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")

	if len(ranges) == 1 {
		rng := ranges[0]
		w.Header().Set("Content-Range", rng.ContentRange(size))
		w.Header().Set("Content-Length", strconv.FormatInt(rng.length, 10))
		code = http.StatusPartialContent
		if r.Method != "HEAD" {
			offset := rng.start
			if seeker, ok := content.(io.Seeker); ok {
				seeker.Seek(offset, io.SeekStart)
			} else {
				io.CopyN(io.Discard, content, offset)
			}
			io.CopyN(w, content, rng.length)
		}
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		if r.Method != "HEAD" {
			io.CopyN(w, content, size)
		}
	}
	if code != http.StatusOK {
		w.WriteHeader(code)
	}
}

type condResult int

const (
	condNone condResult = iota
	condTrue
	condFalse
)

func checkIfMatch(w http.ResponseWriter, r *http.Request) condResult {
	im := r.Header.Get("If-Match")
	if im == "" {
		return condNone
	}
	for {
		im = textproto.TrimString(im)
		if len(im) == 0 {
			break
		}
		if im[0] == ',' {
			im = im[1:]
			continue
		}
		if im[0] == '*' {
			return condTrue
		}
		etag, remain := scanETag(im)
		if etag == "" {
			break
		}
		if etagStrongMatch(etag, w.Header().Get("Etag")) {
			return condTrue
		}
		im = remain
	}

	return condFalse
}

func checkIfUnmodifiedSince(r *http.Request, modtime time.Time) condResult {
	ius := r.Header.Get("If-Unmodified-Since")
	if ius == "" || isZeroTime(modtime) {
		return condNone
	}
	t, err := http.ParseTime(ius)
	if err != nil {
		return condNone
	}

	// The Last-Modified header truncates sub-second precision so
	// the modtime needs to be truncated too.
	modtime = modtime.Truncate(time.Second)
	if ret := modtime.Compare(t); ret <= 0 {
		return condTrue
	}
	return condFalse
}

func checkIfNoneMatch(w http.ResponseWriter, r *http.Request) condResult {
	inm := r.Header.Get("If-None-Match")
	if inm == "" {
		return condNone
	}
	buf := inm
	for {
		buf = textproto.TrimString(buf)
		if len(buf) == 0 {
			break
		}
		if buf[0] == ',' {
			buf = buf[1:]
			continue
		}
		if buf[0] == '*' {
			return condFalse
		}
		etag, remain := scanETag(buf)
		if etag == "" {
			break
		}
		if etagWeakMatch(etag, w.Header().Get("Etag")) {
			return condFalse
		}
		buf = remain
	}
	return condTrue
}

func checkIfModifiedSince(r *http.Request, modtime time.Time) condResult {
	if r.Method != "GET" && r.Method != "HEAD" {
		return condNone
	}
	ims := r.Header.Get("If-Modified-Since")
	if ims == "" || isZeroTime(modtime) {
		return condNone
	}
	t, err := http.ParseTime(ims)
	if err != nil {
		return condNone
	}
	// The Last-Modified header truncates sub-second precision so
	// the modtime needs to be truncated too.
	modtime = modtime.Truncate(time.Second)
	if ret := modtime.Compare(t); ret <= 0 {
		return condFalse
	}
	return condTrue
}

var unixEpochTime = time.Unix(0, 0)

// isZeroTime reports whether t is obviously unspecified (either zero or Unix()=0).
func isZeroTime(t time.Time) bool {
	return t.IsZero() || t.Equal(unixEpochTime)
}

func checkIfRange(w http.ResponseWriter, r *http.Request, modtime time.Time) condResult {
	if r.Method != "GET" && r.Method != "HEAD" {
		return condNone
	}
	ir := r.Header.Get("If-Range")
	if ir == "" {
		return condNone
	}
	etag, _ := scanETag(ir)
	if etag != "" {
		if etagStrongMatch(etag, w.Header().Get("Etag")) {
			return condTrue
		} else {
			return condFalse
		}
	}
	// The If-Range value is typically the ETag value, but it may also be
	// the modtime date. See golang.org/issue/8367.
	if modtime.IsZero() {
		return condFalse
	}
	t, err := http.ParseTime(ir)
	if err != nil {
		return condFalse
	}
	if t.Unix() == modtime.Unix() {
		return condTrue
	}
	return condFalse
}

// scanETag determines if a syntactically valid ETag is present at s. If so,
// the ETag and remaining text after consuming ETag is returned. Otherwise,
// it returns "", "".
func scanETag(s string) (etag string, remain string) {
	s = textproto.TrimString(s)
	start := 0
	if strings.HasPrefix(s, "W/") {
		start = 2
	}
	if len(s[start:]) < 2 || s[start] != '"' {
		return "", ""
	}
	// ETag is either W/"text" or "text".
	// See RFC 7232 2.3.
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		switch {
		// Character values allowed in ETags.
		case c == 0x21 || c >= 0x23 && c <= 0x7E || c >= 0x80:
		case c == '"':
			return s[:i+1], s[i+1:]
		default:
			return "", ""
		}
	}
	return "", ""
}

// etagStrongMatch reports whether a and b match using strong ETag comparison.
// Assumes a and b are valid ETags.
func etagStrongMatch(a, b string) bool {
	return a == b && a != "" && a[0] == '"'
}

// etagWeakMatch reports whether a and b match using weak ETag comparison.
// Assumes a and b are valid ETags.
func etagWeakMatch(a, b string) bool {
	return strings.TrimPrefix(a, "W/") == strings.TrimPrefix(b, "W/")
}

// serveFileError serves an error from ServeFile, ServeFileFS, and ServeContent.
// Because those can all be configured by the caller by setting headers like
// Etag, Last-Modified, and Cache-Control to send on a successful response,
// the error path needs to clear them, since they may not be meant for errors.
func serveFileError(w http.ResponseWriter, text string, code int) {
	for _, k := range []string{
		"Cache-Control",
		"Content-Encoding",
		"Etag",
		"Last-Modified",
	} {
		w.Header().Del(k)
	}
	http.Error(w, text, code)
}

func writeNotModified(w http.ResponseWriter) {
	// RFC 7232 section 4.1:
	// a sender SHOULD NOT generate representation metadata other than the
	// above listed fields unless said metadata exists for the purpose of
	// guiding cache updates (e.g., Last-Modified might be useful if the
	// response does not have an ETag field).
	h := w.Header()
	delete(h, "Content-Type")
	delete(h, "Content-Length")
	delete(h, "Content-Encoding")
	if h.Get("Etag") != "" {
		delete(h, "Last-Modified")
	}
	w.WriteHeader(http.StatusNotModified)
}
