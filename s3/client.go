package s3

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Options struct {
	Address            string `json:"address,omitempty"`
	AccountID          string `json:"accountID,omitempty" description:"Account ID,ignored if empty"`
	AccessKey          string `json:"accessKey,omitempty"`
	SecretKey          string `json:"secretKey,omitempty"`
	PathStyle          bool   `json:"pathStyle,omitempty"`
	Region             string `json:"region,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

func NewDefaultOptions() *Options {
	return &Options{
		PathStyle: true,
		Region:    "default", // must be set
	}
}

func NewClient(ctx context.Context, opts *Options) (*Client, error) {
	httpclient := http.DefaultClient
	if opts.InsecureSkipVerify {
		httpclient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}
	s3options := s3.Options{
		UsePathStyle: opts.PathStyle,
		BaseEndpoint: aws.String(opts.Address),
		Credentials: credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     opts.AccessKey,
				SecretAccessKey: opts.SecretKey,
				AccountID:       opts.AccountID,
			},
		},
		HTTPClient: httpclient,
		Region:     opts.Region,
	}
	s3cli := s3.New(s3options)
	s3presign := s3.NewPresignClient(s3cli)
	return &Client{Client: *s3cli, PresignClient: *s3presign}, nil
}

type Client struct {
	s3.Client
	s3.PresignClient
}
