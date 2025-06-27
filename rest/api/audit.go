// Copyright 2023 The Kubegems Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/wildcard"
)

type Auditor interface {
	OnRequest(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, *AuditLog)
	OnResponse(w http.ResponseWriter, r *http.Request, auditlog *AuditLog)
}

type AuditSink interface {
	Save(log *AuditLog) error
}

func NewAuditFilter(auditor Auditor, sink AuditSink) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		ww, auditlog := auditor.OnRequest(w, r)
		if auditlog == nil {
			next.ServeHTTP(ww, r)
			return
		}
		if ww == nil {
			ww = w
		}
		// save audit log to context
		r = r.WithContext(WithAuditLog(r.Context(), auditlog))
		next.ServeHTTP(ww, r)
		auditor.OnResponse(ww, r, auditlog)
		_ = sink.Save(auditlog)
	})
}

type LoggerAuditSink struct {
	Sink   AuditSink
	Logger log.Logger
}

func (l *LoggerAuditSink) Save(log *AuditLog) error {
	// trim query params
	reqpath := log.Request.URL
	if idx := strings.Index(reqpath, "?"); idx > 0 {
		reqpath = reqpath[:idx]
	}
	l.Logger.Info(
		reqpath,
		"method", log.Request.Method,
		"remote", log.Request.ClientIP,
		"code", log.Response.StatusCode,
		"duration", log.EndTime.Sub(log.StartTime).String(),
		"resource", log.ResourceType,
		"name", log.ResourceName,
	)
	if l.Sink != nil {
		return l.Sink.Save(log)
	}
	return nil
}

const DefaultAuditLogCacheSize = 256

func NewCachedAuditSink(ctx context.Context, sink AuditSink, maxCacheSize int) AuditSink {
	if maxCacheSize <= 0 {
		maxCacheSize = DefaultAuditLogCacheSize
	}
	logger := log.FromContext(ctx).WithName("cached-audit-sink")
	cachesink := &CachedAuditSink{
		sink:   sink,
		cache:  make(chan *AuditLog, maxCacheSize),
		logger: logger,
	}
	go func() {
		for {
			select {
			case auditlog := <-cachesink.cache:
				if err := sink.Save(auditlog); err != nil {
					logger.Error(err, "save audit log")
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return cachesink
}

type CachedAuditSink struct {
	sink   AuditSink
	logger log.Logger
	cache  chan *AuditLog
}

func (c *CachedAuditSink) Save(log *AuditLog) error {
	select {
	case c.cache <- log:
	default:
		c.logger.Error(fmt.Errorf("cache channel full,drop audit log"), "save audit log")
		return fmt.Errorf("cache is full")
	}
	return nil
}

const MB = 1 << 20

type AuditOptions struct {
	RecordMethods             []string
	RecordRequestContentTypes []string
	RecordRequestBody         bool
	RecordResponseBody        bool
	MaxRecordBodySize         int
	WhiteList                 []string // list of paths to exclude from audit, allow wildcard
}

func NewDefaultAuditOptions() *AuditOptions {
	return &AuditOptions{
		RecordMethods:             []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		RecordRequestContentTypes: []string{"application/json", "application/yaml", "application/xml", "application/x-www-form-urlencoded"},
		RecordRequestBody:         true,
		RecordResponseBody:        true,
		MaxRecordBodySize:         1 * MB,
		WhiteList:                 []string{},
	}
}

type AuditSSH struct {
	User          string   `json:"user,omitempty"`
	RemoteAddr    string   `json:"remoteAddr,omitempty"`
	LocalAddr     string   `json:"localAddr,omitempty"`
	SessionID     string   `json:"sessionID,omitempty"`
	ClientVersion string   `json:"clientVersion,omitempty"`
	ServerVersion string   `json:"serverVersion,omitempty"`
	PublicKey     string   `json:"publicKey,omitempty"`
	Command       string   `json:"command,omitempty"`
	Env           []string `json:"env,omitempty"`
}

type AuditRequest struct {
	HttpVersion string            `json:"httpVersion,omitempty"` // http version
	Method      string            `json:"method,omitempty"`      // method
	URL         string            `json:"url,omitempty"`         // full url
	Header      map[string]string `json:"header,omitempty"`      // header
	Body        []byte            `json:"body,omitempty"`        // ignore body if size > 1MB or stream.
	ClientIP    string            `json:"clientIP,omitempty"`    // client ip
	RemoteAddr  string            `json:"remoteAddr,omitempty"`
	LocalAddr   string            `json:"localAddr,omitempty"`
}

type AuditResponse struct {
	StatusCode   int               `json:"statusCode,omitempty"`   // status code
	Header       map[string]string `json:"header,omitempty"`       // header
	Hijacked     bool              `json:"hijacked,omitempty"`     // hijacked
	ResponseBody []byte            `json:"responseBody,omitempty"` // ignore body if size > 1MB or stream.
}

type AuditExtraMetadata map[string]string

type AuditLog struct {
	RequestID string `json:"requestID,omitempty"` // request id
	// request
	Request  AuditRequest  `json:"request,omitempty"`
	Response AuditResponse `json:"response,omitempty"`
	// authz
	Subject string `json:"subject,omitempty"` // username
	// Resource is the resource type, e.g. "pods", "namespaces/default/pods/nginx-xxx"
	// we can detect the resource type and name from the request path.
	// GET  /zoos/{zoo_id}/animals/{animal_id} 	-> get zoos,zoo_id,animals,animal_id
	// GET  /zoos/{zoo_id}/animals 				-> list zoos,zoo_id,animals,animal_id
	// POST /zoos/{zoo_id}/animals:set-free 	-> set-free zoos,zoo_id,animals
	Action       string             `json:"action,omitempty"`       // create, update, delete, get, list, set-free, etc.
	Domain       string             `json:"domain,omitempty"`       // for multi-tenant
	Parents      []AttrbuteResource `json:"parents,omitempty"`      // parent resources, e.g. "zoos/{zoo_id}",
	ResourceType string             `json:"resourceType,omitempty"` // resource type, e.g. "animals"
	ResourceName string             `json:"resourceName,omitempty"` //  "{animal_id}", or "" if list
	// metadata
	StartTime time.Time          `json:"startTime,omitempty"` // request start time
	EndTime   time.Time          `json:"endTime,omitempty"`   // request end time
	Extra     AuditExtraMetadata `json:"extra,omitempty"`     // extra metadata
}

func WithAuditLog(ctx context.Context, log *AuditLog) context.Context {
	return SetContextValue(ctx, "audit-log", log)
}

func AuditLogFromContext(ctx context.Context) *AuditLog {
	return GetContextValue[*AuditLog](ctx, "audit-log")
}

// AddAuditLogExtra adds extra metadata to the audit log.
func AddAuditLogExtra(r *http.Request, key string, value string) {
	if log := AuditLogFromContext(r.Context()); log != nil {
		if log.Extra == nil {
			log.Extra = make(AuditExtraMetadata)
		}
		log.Extra[key] = value
	}
}

const RequestIDHeader = "X-Request-ID"

func NewSimpleAuditFilter(sink AuditSink, options *AuditOptions) *SimpleAuditor {
	return &SimpleAuditor{Sink: sink, Options: options}
}

type SimpleAuditor struct {
	Sink    AuditSink
	Options *AuditOptions
}

func (a *SimpleAuditor) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	uid := r.Header.Get(RequestIDHeader)
	if uid == "" {
		uid = uuid.NewString()
		r.Header.Set(RequestIDHeader, uid)
	}
	if !slices.Contains(a.Options.RecordMethods, r.Method) {
		next.ServeHTTP(w, r)
		return
	}
	for _, path := range a.Options.WhiteList {
		if wildcard.Match(path, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
	}
	ww, auditlog := a.OnRequest(w, r)
	if auditlog == nil {
		next.ServeHTTP(ww, r)
		return
	}
	if ww == nil {
		ww = w
	}
	// save audit log to context
	r = r.WithContext(WithAuditLog(r.Context(), auditlog))
	next.ServeHTTP(ww, r)
	a.OnResponse(ww, r, auditlog)
	if err := a.Sink.Save(auditlog); err != nil {
		log.FromContext(r.Context()).Error(err, "save audit log")
	}
}

func (a *SimpleAuditor) OnRequest(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, *AuditLog) {
	auditlog := &AuditLog{
		RequestID: r.Header.Get(RequestIDHeader),
		Request: AuditRequest{
			HttpVersion: r.Proto,
			Method:      r.Method,
			URL:         r.URL.String(),
			Header:      HttpHeaderToMap(r.Header),
			ClientIP:    ExtractClientIP(r),
		},
		StartTime: time.Now(),
	}
	if a.Options.RecordRequestBody {
		auditlog.Request.Body = ReadBodySafely(r, a.Options.RecordRequestContentTypes, a.Options.MaxRecordBodySize)
	}

	var responseBodyCache *bytes.Buffer
	respcachesize := 0
	if a.Options.RecordResponseBody {
		respcachesize = a.Options.MaxRecordBodySize
	}
	w = httpsnoop.Wrap(w, httpsnoop.Hooks{
		WriteHeader: func(whf httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				if code == 0 {
					code = http.StatusOK
				}
				auditlog.Response.StatusCode = code
				whf(code)
			}
		},
		Hijack: func(hf httpsnoop.HijackFunc) httpsnoop.HijackFunc {
			return func() (net.Conn, *bufio.ReadWriter, error) {
				auditlog.Response.Hijacked = true
				return hf()
			}
		},
		Write: func(wf httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(p []byte) (int, error) {
				// Write before WriteHeader
				if auditlog.Response.StatusCode == 0 {
					auditlog.Response.StatusCode = http.StatusOK
				}
				if respcachesize > 0 {
					if responseBodyCache == nil {
						responseBodyCache = bytes.NewBuffer(nil)
					}
					if len(p) > respcachesize {
						p = p[:respcachesize]
					}
					n, _ := responseBodyCache.Write(p)
					respcachesize -= n
				}
				return wf(p)
			}
		},
	})
	return w, auditlog
}

func (a *SimpleAuditor) OnResponse(w http.ResponseWriter, r *http.Request, auditlog *AuditLog) {
	if attr := AttributesFromContext(r.Context()); attr != nil {
		auditlog.Action = attr.Action
		if size := len(attr.Resources); size > 0 {
			parents, last := attr.Resources[:size-1], attr.Resources[size-1]
			auditlog.Parents, auditlog.ResourceType, auditlog.ResourceName = parents, last.Resource, last.Name
		}
	}
	if username := AuthenticateFromContext(r.Context()).User.Name; username != "" {
		auditlog.Subject = username
	}
	auditlog.EndTime = time.Now()
	auditlog.Response.Header = HttpHeaderToMap(w.Header())

	// if not call WriteHeader, set status code to 200
	if code := auditlog.Response.StatusCode; code == 0 {
		auditlog.Response.StatusCode = http.StatusOK
	}
}

func ExtractClientIP(r *http.Request) string {
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-Ip")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	return clientIP
}

func HttpHeaderToMap(header http.Header) map[string]string {
	m := make(map[string]string)
	for k, v := range header {
		m[k] = strings.Join(v, ",")
	}
	return m
}

func ReadBodySafely(req *http.Request, allowsContentType []string, maxReadSize int) []byte {
	contenttype, contentlen := req.Header.Get("Content-Type"), req.ContentLength
	if contenttype == "" || contentlen == 0 || maxReadSize <= 0 {
		return nil
	}
	mediatype, _, _ := mime.ParseMediaType(contenttype)
	allowed := slices.Contains(allowsContentType, mediatype)
	if !allowed {
		return nil
	}
	cachesize := maxReadSize
	if contentlen < int64(maxReadSize) {
		cachesize = int(contentlen)
	}
	if cachesize <= 0 {
		return nil
	}
	cachedbody := make([]byte, cachesize)
	n, err := io.ReadFull(req.Body, cachedbody)
	// io.ReadFull returns io.ErrUnexpectedEOF if EOF is encountered before filling the buffer.
	if err != nil && err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	req.Body = NewCachedBody(req.Body, cachedbody[:n], err)
	return cachedbody[:n]
}

var _ io.ReadCloser = &CachedBody{}

type CachedBody struct {
	cached []byte
	err    error // early read error
	readn  int
	body   io.ReadCloser
}

// NewCachedBody returns a new CachedBody.
// a CachedBody is a io.ReadCloser that read from cached first, then read from body.
func NewCachedBody(body io.ReadCloser, cached []byte, earlyerr error) *CachedBody {
	return &CachedBody{body: body, cached: cached, err: earlyerr}
}

func (w *CachedBody) Read(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.readn < len(w.cached) {
		n += copy(p, w.cached[w.readn:])
		w.readn += n
		if n == len(p) {
			return n, nil
		}
		p = p[n:] // continue read from body
	}
	bn, err := w.body.Read(p)
	n += bn
	return n, err
}

func (w *CachedBody) Close() error {
	return w.body.Close()
}
