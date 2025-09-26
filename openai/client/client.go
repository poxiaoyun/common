package client

import (
	"context"
	"net/http"
	"net/url"

	"xiaoshiai.cn/common/httpclient"
)

type Client struct {
	cli *httpclient.Client
}

type Options struct {
	Server string `json:"server,omitempty"`
	Token  string `json:"token,omitempty"`
}

func NewClient(options *Options) (*Client, error) {
	serverURL, err := url.Parse(options.Server)
	if err != nil {
		return nil, err
	}
	cli := httpclient.NewClientFromClientConfig(&httpclient.ClientConfig{
		Server: serverURL,
	})
	cli.OnRequest = func(req *http.Request) error {
		if options.Token != "" {
			req.Header.Set("Authorization", "Bearer "+options.Token)
		}
		return nil
	}
	return &Client{cli: cli}, nil
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ListModels lists the currently available models, and provides basic information about each one such as the owner and availability.
// https://platform.openai.com/docs/api-reference/models/list
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	type ListModelsResponse struct {
		Object string  `json:"object"`
		Data   []Model `json:"data"`
	}
	resp := &ListModelsResponse{}
	if err := c.cli.Get("/v1/models").Return(resp).Send(ctx); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// RetrieveModel retrieves a model instance, providing basic information about the model such as its owner and permissioning.
// https://platform.openai.com/docs/api-reference/models/retrieve
func (c *Client) RetrieveModel(ctx context.Context, model string) (*Model, error) {
	resp := &Model{}
	if err := c.cli.Get("/v1/models/" + model).Return(resp).Send(ctx); err != nil {
		return nil, err
	}
	return resp, nil
}
