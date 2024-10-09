package harbor

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"xiaoshiai.cn/common/httpclient"
)

const APIPREFIX = "/api/v2.0"

type Client struct {
	cli       *httpclient.Client
	csrftoken string
	options   *Options
}

type Options struct {
	Addr     string `json:"addr,omitempty"`
	Username string `json:"username,omitempty"`
	Passwd   string `json:"passwd,omitempty"`
}

func NewClient(o *Options) (*Client, error) {
	addr := o.Addr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "https://" + addr
	}
	addr = strings.TrimRight(addr, "/") + APIPREFIX
	c := &Client{
		cli:     &httpclient.Client{Server: addr},
		options: o,
	}
	c.cli.OnRequest = c.onRequest
	c.cli.OnResponse = c.onResponse
	return c, nil
}

const csrfTokenHeader = "X-Harbor-CSRF-Token"

func (c *Client) onRequest(req *http.Request) error {
	if req.Method != http.MethodGet {
		// add csrftoken header
		if c.csrftoken == "" {
			if _, err := c.SystemInfo(req.Context()); err != nil {
				return fmt.Errorf("error in harbor when get csrt token %w", err)
			}
		}
		req.Header.Add(csrfTokenHeader, c.csrftoken)
	}
	req.SetBasicAuth(c.options.Username, c.options.Passwd)
	return nil
}

func (c *Client) onResponse(req *http.Request, resp *http.Response) error {
	// update csrftoken if exist
	if req.Method == http.MethodGet {
		if csrftoken := resp.Header.Get(csrfTokenHeader); csrftoken != "" {
			c.csrftoken = csrftoken
		}
	}
	return nil
}

type CommonOptions struct {
	Q        string
	Sort     string
	Page     int
	PageSize int
}

func (o CommonOptions) ToQuery() url.Values {
	q := make(url.Values)
	if o.Q != "" {
		enc := url.QueryEscape(o.Q)
		q.Set("q", enc)
	}
	if o.Sort != "" {
		q.Set("sort", o.Sort)
	}
	if o.Page > 0 {
		q.Set("page", strconv.Itoa(o.Page))
	}
	if o.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(o.PageSize))
	}
	return q
}
