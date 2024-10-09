// +k8s:openapi-gen=true
package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

var _ Filter = &AdminssionFilter{}

type AdmissionContent struct {
	ContentEncoding string
	ContentType     string
	Data            io.Reader
}

type AdmissionPlugin interface {
	Match(Attributes) bool
	Admit(ctx context.Context, attr Attributes, body AdmissionContent) (bool, error)
}

const (
	// DefaultBodySizeLimit is the default maximum size of a request body.
	DefaultBodySizeLimit = 5 * 1024 * 1024
)

func NewAdminssionFilter(plugins ...AdmissionPlugin) *AdminssionFilter {
	return &AdminssionFilter{
		BodySizeLimit: DefaultBodySizeLimit,
		Plugins:       plugins,
	}
}

type AdminssionFilter struct {
	BodySizeLimit int64
	Plugins       []AdmissionPlugin
}

// Process implements api.Filter.
func (a *AdminssionFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	var bodyBytes []byte
	if attr := AttributesFromContext(r.Context()); attr != nil {
		for _, plugin := range a.Plugins {
			if !plugin.Match(*attr) {
				continue
			}
			if bodyBytes == nil {
				readBytes, err := ReadBodyNoSideEffect(r, a.BodySizeLimit)
				if err != nil {
					BadRequest(w, err.Error())
					return
				}
				bodyBytes = readBytes
			}
			admissionBody := AdmissionContent{
				ContentType: r.Header.Get("Content-Type"),
				Data:        bytes.NewReader(bodyBytes),
			}
			allowed, err := plugin.Admit(r.Context(), *attr, admissionBody)
			if !allowed {
				Forbidden(w, fmt.Sprintf("admission denied by %T: %v", plugin, err))
				return
			}
		}
	}
	next.ServeHTTP(w, r)
}

// ReadBodyNoSideEffect reads the request body up to limit bytes and returns the bytes read.
// The request body is reset so it can be read again.
func ReadBodyNoSideEffect(r *http.Request, limit int64) ([]byte, error) {
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, io.LimitReader(r.Body, limit)); err != nil {
		return nil, err
	}
	bodyBytes := buf.Bytes()
	type rc struct {
		io.Reader
		io.Closer
	}
	// Reset the body so it can be read again.
	r.Body = rc{Reader: bytes.NewReader(bodyBytes), Closer: r.Body}
	return bodyBytes, nil
}
