package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
)

type Request struct {
	Err          error
	Client       *http.Client
	RoundTripper http.RoundTripper
	BaseAddr     string
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
	OnDecode     func(resp *http.Response, into any) error
}

func BuildRequest(ctx context.Context, r Request) (*http.Request, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	serveraddr := r.BaseAddr
	if serveraddr == "" {
		return nil, errors.NewBadRequest("empty base address on http request")
	}
	serveru, err := url.Parse(serveraddr)
	if err != nil {
		return nil, err
	}
	u := MergeURL(*serveru, r.Path, r.Queries)

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
	log.FromContext(ctx).V(5).Info("common http client request", "method", req.Method, "url", req.URL.String())
	resp, err := GetClient(r.Client, r.RoundTripper).Do(req)
	if err != nil {
		return nil, err
	}
	if r.OnResponse != nil {
		if err := r.OnResponse(req, resp); err != nil {
			return resp, err
		}
	}
	if r.DecodeInto != nil {
		if r.OnDecode == nil {
			return resp, DefaultDecodeFunc(resp, r.DecodeInto)
		}
		return resp, r.OnDecode(resp, r.DecodeInto)
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

func DefaultDecodeFunc(resp *http.Response, into any) error {
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
		jsondata, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(jsondata, into); err != nil {
			return fmt.Errorf("unexpected response: %s", string(jsondata))
		}
		return nil
	}
}
