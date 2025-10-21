package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/log"
)

type AuditSink interface {
	Save(log *AuditLog) error
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

type WebhookAuditSinkOptions struct {
	Server                string        `json:"server,omitempty" description:"webhook server address. e.g. https://example.com/webhook-audit"`
	ProxyURL              string        `json:"proxyURL,omitempty" description:"proxy server address"`
	Token                 string        `json:"token,omitempty" description:"authentication token"`
	Username              string        `json:"username,omitempty" description:"basic auth username"`
	Password              string        `json:"password,omitempty" description:"basic auth password"`
	CertFile              string        `json:"certFile,omitempty" description:"path to TLS certificate file"`
	KeyFile               string        `json:"keyFile,omitempty" description:"path to TLS key file"`
	CAFile                string        `json:"caFile,omitempty" description:"path to CA certificate file"`
	InsecureSkipTLSVerify bool          `json:"insecureSkipTLSVerify,omitempty" description:"skip TLS verification"`
	Timeout               time.Duration `json:"timeout,omitempty" description:"timeout when sending audit log to webhook server"`
}

type WebhookAuditSink struct {
	httpclient *httpclient.Client
	timeout    time.Duration
}

func NewDefaultWebhookAuditSinkOptions() *WebhookAuditSinkOptions {
	return &WebhookAuditSinkOptions{
		Timeout: 30 * time.Second,
	}
}

func NewWebhookAuditSink(opts *WebhookAuditSinkOptions) (*WebhookAuditSink, error) {
	return NewWebhookAuditSinkWithContext(context.Background(), opts)
}

func NewWebhookAuditSinkWithContext(ctx context.Context, opts *WebhookAuditSinkOptions) (*WebhookAuditSink, error) {
	config := &httpclient.Config{
		Server:                opts.Server,
		ProxyURL:              opts.ProxyURL,
		Token:                 opts.Token,
		Username:              opts.Username,
		Password:              opts.Password,
		CertFile:              opts.CertFile,
		KeyFile:               opts.KeyFile,
		CAFile:                opts.CAFile,
		InsecureSkipTLSVerify: opts.InsecureSkipTLSVerify,
	}
	cli, err := httpclient.NewClientFromConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	return &WebhookAuditSink{httpclient: cli, timeout: opts.Timeout}, nil
}

func (w *WebhookAuditSink) Save(log *AuditLog) error {
	ctx := context.Background()
	if w.timeout > 0 {
		timeoutctx, cancel := context.WithTimeout(context.Background(), w.timeout)
		defer cancel()
		ctx = timeoutctx
	}
	_, err := w.httpclient.Post("").JSON(log).Do(ctx)
	return err
}
