package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

type Builder struct {
	R Request
}

func NewRequest(method, path string) *Builder {
	return &Builder{R: Request{Method: method, Path: path}}
}

func Get(path string) *Builder {
	return NewRequest(http.MethodGet, path)
}

func Post(path string) *Builder {
	return NewRequest(http.MethodPost, path)
}

func Put(path string) *Builder {
	return NewRequest(http.MethodPut, path)
}

func Delete(path string) *Builder {
	return NewRequest(http.MethodDelete, path)
}

func (r *Builder) Client(cli *http.Client) *Builder {
	r.R.Client = cli
	return r
}

func (r *Builder) OnRequest(handler func(req *http.Request) error) *Builder {
	r.R.OnRequest = handler
	return r
}

func (r *Builder) OnResponse(handler func(req *http.Request, resp *http.Response) error) *Builder {
	r.R.OnResponse = handler
	return r
}

func (r *Builder) RoundTripper(tp http.RoundTripper) *Builder {
	r.R.RoundTripper = tp
	return r
}

func (r *Builder) BaseAddr(addr string) *Builder {
	r.R.BaseAddr = addr
	return r
}

func (r *Builder) Query(key, value string) *Builder {
	if r.R.Queries == nil {
		r.R.Queries = url.Values{}
	}
	r.R.Queries.Add(key, value)
	return r
}

func (r *Builder) Queries(queries url.Values) *Builder {
	if r.R.Queries == nil {
		r.R.Queries = url.Values{}
	}
	for k, v := range queries {
		r.R.Queries[k] = v
	}
	return r
}

func (r *Builder) Header(key, value string) *Builder {
	if r.R.Headers == nil {
		r.R.Headers = http.Header{}
	}
	r.R.Headers.Add(key, value)
	return r
}

func (r *Builder) Headers(headers http.Header) *Builder {
	if r.R.Headers == nil {
		r.R.Headers = http.Header{}
	}
	for k, v := range headers {
		r.R.Headers[k] = v
	}
	return r
}

func (r *Builder) Cookie(key, value string) *Builder {
	r.R.Cookies = append(r.R.Cookies, http.Cookie{Name: key, Value: value})
	return r
}

func (r *Builder) Cookies(cookies []http.Cookie) *Builder {
	r.R.Cookies = append(r.R.Cookies, cookies...)
	return r
}

func (r *Builder) MultiFormData(data map[string]string) *Builder {
	return r.Body(NewMultiFormData(data))
}

func (r *Builder) FormURLEncoded(data map[string]string) *Builder {
	return r.Body(NewFormURLEncoded(data))
}

func (r *Builder) JSON(data any) *Builder {
	jsondata, err := json.Marshal(data)
	if err != nil {
		r.R.Err = err
		return r
	}
	return r.Body(bytes.NewBuffer(jsondata), "application/json")
}

func (r *Builder) Text(data string) *Builder {
	return r.Body(bytes.NewBufferString(data), "text/plain")
}

func (r *Builder) Body(data io.Reader, contenttype string) *Builder {
	r.R.Body, r.R.ContentType = data, contenttype
	return r
}

func (r *Builder) Binary(data []byte) *Builder {
	return r.Body(bytes.NewReader(data), "application/octet-stream")
}

func (r *Builder) OnDecode(handler func(resp *http.Response, into any) error) *Builder {
	r.R.OnDecode = handler
	return r
}

func (r *Builder) Return(into any) *Builder {
	r.R.DecodeInto = into
	return r
}

func (r *Builder) Send(ctx context.Context) error {
	_, err := r.Do(ctx)
	return err
}

func (r *Builder) Do(ctx context.Context) (*http.Response, error) {
	return Do(ctx, r.R)
}
