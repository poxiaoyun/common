package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"

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
	GetBody      func() (io.Reader, error)
	DecodeInto   any
	OnRequest    func(req *http.Request) error
	OnResponse   func(req *http.Request, resp *http.Response) error
	OnDecode     func(resp *http.Response, into any) error
}

func BuildRequest(r Request) (*http.Request, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	u, err := MergeURL(r.BaseAddr, r.Path, r.Queries)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		return nil, err
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
	req, err := BuildRequest(r)
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

func MergeURL(server, reqpath string, queries url.Values) (*url.URL, error) {
	if server == "" {
		return nil, errors.NewBadRequest("empty base address on http request")
	}
	u, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, reqpath)
	existsQuery := u.Query()
	for k, v := range queries {
		existsQuery[k] = v
	}
	u.RawQuery = existsQuery.Encode()
	return u, nil
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
		return json.NewDecoder(resp.Body).Decode(into)
	}
}
