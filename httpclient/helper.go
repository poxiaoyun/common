package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/textproto"
	"net/url"
	"path"
	"reflect"
	"strings"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/meta"
	libreflect "xiaoshiai.cn/common/reflect"
)

type Request struct {
	Err          error
	Client       *http.Client
	RoundTripper http.RoundTripper
	BaseAddr     *url.URL
	Method       string
	Path         string
	Queries      url.Values
	Headers      http.Header
	Cookies      []http.Cookie
	ContentType  string
	Body         io.Reader
	GetBody      func() (io.ReadCloser, error)
	DecodeInto   any
	OnRequest    func(req *http.Request) error
	OnResponse   func(req *http.Request, resp *http.Response) error
	OnDecode     func(req *http.Request, resp *http.Response, into any) error
	Debug        bool
}

func BuildRequest(ctx context.Context, r Request) (*http.Request, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	serveraddr := r.BaseAddr
	if serveraddr == nil {
		return nil, errors.NewBadRequest("empty base address on http request")
	}
	u := MergeURL(*serveraddr, r.Path, r.Queries)

	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
	}
	if r.GetBody != nil {
		req.ContentLength = -1
		req.GetBody = r.GetBody
		req.Body, err = r.GetBody()
		if err != nil {
			return nil, err
		}
	}
	if r.ContentType != "" {
		req.Header.Set("Content-Type", r.ContentType)
	}
	for k, v := range r.Headers {
		req.Header[k] = v
	}
	for _, v := range r.Cookies {
		req.AddCookie(&v)
	}
	if r.OnRequest != nil {
		if err := r.OnRequest(req); err != nil {
			return nil, err
		}
	}
	return req, nil
}

func Do(ctx context.Context, r Request) (*http.Response, error) {
	req, err := BuildRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	log := log.FromContext(ctx)
	log.V(6).Info("http request", "method", req.Method, "url", req.URL.String(), "headers", req.Header)
	if r.Debug {
		if _, isbuffer := r.Body.(*bytes.Buffer); isbuffer {
			dump, err := httputil.DumpRequest(req, true)
			if err != nil {
				log.Error(err, "failed to dump request")
			} else {
				log.Info("http request", "dump", string(dump))
			}
		}
	}
	resp, err := GetClient(r.Client, r.RoundTripper).Do(req)
	if err != nil {
		return nil, err
	}
	log.V(6).Info("http response", "status", resp.StatusCode, "headers", resp.Header)
	if r.Debug {
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			log.Error(err, "failed to dump response")
		} else {
			log.Info("http response", "dump", string(dump))
		}
	}
	if r.OnResponse != nil {
		if err := r.OnResponse(req, resp); err != nil {
			return resp, err
		}
	}
	if r.DecodeInto != nil {
		if r.OnDecode == nil {
			return resp, DefaultDecodeFunc(req, resp, r.DecodeInto)
		}
		return resp, r.OnDecode(req, resp, r.DecodeInto)
	} else {
		return resp, nil
	}
}

func NewMultiFormData(data map[string]string) (io.Reader, string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for k, v := range data {
		writer.WriteField(k, v)
	}
	writer.Close()
	return body, writer.FormDataContentType()
}

type MultiFormPart struct {
	FieldName string
	Reader    io.Reader
	FileName  string
	Header    textproto.MIMEHeader
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

func NewMultiFormDataStream(data []MultiFormPart) (io.Reader, string) {
	piper, pipew := io.Pipe()
	mr := multipart.NewWriter(pipew)
	go func() {
		defer pipew.Close()
		defer mr.Close()
		for _, p := range data {
			h := make(textproto.MIMEHeader)
			if p.FieldName != "" {
				h.Set("Content-Disposition",
					fmt.Sprintf(`form-data; name="%s"`, escapeQuotes(p.FieldName)))
			}
			if p.FileName != "" {
				h.Set("Content-Disposition",
					fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(p.FieldName), escapeQuotes(p.FileName)))
				h.Set("Content-Type", "application/octet-stream")
			}
			for k, v := range p.Header {
				h[k] = v
			}
			part, err := mr.CreatePart(h)
			if err != nil {
				pipew.CloseWithError(err)
				return
			}
			if _, err := io.Copy(part, p.Reader); err != nil {
				pipew.CloseWithError(err)
				return
			}
		}
	}()
	return piper, mr.FormDataContentType()
}

func NewFormURLEncoded(data map[string]string) (io.Reader, string) {
	form := url.Values{}
	for k, v := range data {
		form.Add(k, v)
	}
	return bytes.NewBufferString(form.Encode()), "application/x-www-form-urlencoded"
}

func MergeURL(u url.URL, reqpath string, queries url.Values) *url.URL {
	u.Path = path.Join(u.Path, reqpath)
	existsQuery := u.Query()
	for k, v := range queries {
		existsQuery[k] = v
	}
	u.RawQuery = existsQuery.Encode()
	return &u
}

func GetClient(cli *http.Client, tp http.RoundTripper) *http.Client {
	if cli == nil {
		if tp == nil {
			return http.DefaultClient
		}
		return &http.Client{Transport: tp}
	}
	return cli
}

func DefaultDecodeFunc(req *http.Request, resp *http.Response, into any) error {
	if resp.StatusCode < http.StatusInternalServerError && resp.StatusCode >= http.StatusBadRequest {
		bytes, _ := io.ReadAll(resp.Body)
		return errors.NewBadRequest(string(bytes))
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		bytes, _ := io.ReadAll(resp.Body)
		return errors.NewInternalError(fmt.Errorf("status code: %d, message: %s", resp.StatusCode, string(bytes)))
	}
	if into == nil {
		return nil
	}
	defer resp.Body.Close()
	switch into := into.(type) {
	case io.Writer:
		_, err := io.Copy(into, resp.Body)
		return err
	default:
		if resp.ContentLength == 0 {
			return errors.NewInternalError(fmt.Errorf("empty response body"))
		}
		jsondata, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		log.FromContext(req.Context()).V(6).Info("common http client response", "status", resp.StatusCode, "body", string(jsondata))
		if err := json.Unmarshal(jsondata, into); err != nil {
			return errors.NewInternalError(fmt.Errorf("failed to unmarshal response: %w, response: %s", err, string(jsondata)))
		}
		return nil
	}
}

func ObjectToQuery(v any) url.Values {
	values := url.Values{}
	libreflect.FlattenStructOmmitEmpty("", 1, true, reflect.ValueOf(v), func(name string, v reflect.Value) error {
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Struct {
			jsondata, err := json.Marshal(v.Interface())
			if err != nil {
				return err
			}
			values.Add(name, string(jsondata))
			return nil
		}
		values.Add(name, fmt.Sprint(v.Interface()))
		return nil
	})
	return values
}

func StatusOnResponse(req *http.Request, resp *http.Response) error {
	if resp.StatusCode < http.StatusBadRequest {
		return nil
	}
	reader := io.LimitReader(resp.Body, 1024)
	var cache bytes.Buffer
	statuserr := &errors.Status{}
	if err := json.NewDecoder(io.TeeReader(reader, &cache)).Decode(statuserr); err == nil {
		return statuserr
	}
	// read the rest of body
	io.Copy(&cache, reader)
	return errors.NewCustomError(resp.StatusCode, errors.StatusReasonUnknown, cache.String())
}

func ListOptionsToQuery(options meta.ListOptions) url.Values {
	values := url.Values{}
	if options.Size > 0 {
		values.Set("size", fmt.Sprint(options.Size))
	}
	if options.Page > 0 {
		values.Set("page", fmt.Sprint(options.Page))
	}
	if options.Search != "" {
		values.Set("search", options.Search)
	}
	if options.Sort != "" {
		values.Set("sort", options.Sort)
	}
	if options.FieldSelector != "" {
		values.Set("fieldSelector", options.FieldSelector)
	}
	if options.LabelSelector != "" {
		values.Set("labelSelector", options.LabelSelector)
	}
	if options.Continue != "" {
		values.Set("continue", options.Continue)
	}
	return values
}
