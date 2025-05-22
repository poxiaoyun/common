package harbor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"xiaoshiai.cn/common/httpclient"
)

const APIPREFIX = "/api/v2.0"

const HeaderXTotalCount = "X-Total-Count"

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

func GetHeaderTotalCount(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	if totalCount := resp.Header.Get(HeaderXTotalCount); totalCount != "" {
		if count, err := strconv.Atoi(totalCount); err == nil {
			return count
		}
	}
	return 0
}

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
	if resp.StatusCode >= 400 {
		he := HarborErrors{}
		bodydata, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(bodydata) > 0 {
			if err := json.Unmarshal(bodydata, &he); err != nil {
				return fmt.Errorf("unexpected response status %d, %s", resp.StatusCode, string(bodydata))
			}
			return he
		}
		return fmt.Errorf("unexpected response status %d", resp.StatusCode)
	}
	return nil
}

type CommonOptions struct {
	// Query string to query resources.
	// Supported query patterns are "exact match(k=v)", "fuzzy match(k=~v)", "range(k=[min~max])",
	//  "list with union releationship(k={v1 v2 v3})" and "list with intersetion relationship(k=(v1 v2 v3))".
	// The value of range and list can be string(enclosed by " or '),
	// integer or time(in format "2020-04-09 02:36:00").
	// All of these query patterns should be put in the query string "q=xxx" and splitted by ",". e.g. q=k1=v1,k2=~v2,k3=[min~max]
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

type List[T any] struct {
	Total int `json:"total"`
	Items []T `json:"items"`
}
