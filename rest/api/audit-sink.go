package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/log"
)

type AuditSink interface {
	// Save delivers one completed immutable audit event to the sink.
	Save(log *AuditLog) error
}

// FanoutAuditSink saves each immutable audit event to every configured sink in
// parallel. A failure from one sink does not prevent other sinks from receiving
// the event.
type FanoutAuditSink []AuditSink

// Save implements AuditSink.
func (sinks FanoutAuditSink) Save(log *AuditLog) error {
	errlist := make([]error, len(sinks))
	wait := sync.WaitGroup{}
	wait.Add(len(sinks))
	for i, sink := range sinks {
		go func() {
			defer wait.Done()
			errlist[i] = sink.Save(log)
		}()
	}
	wait.Wait()
	return errors.NewAggregate(errlist)
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
	// Options configures the audit-event HTTP endpoint and transport.
	httpclient.Options `json:",inline"`
	Timeout            time.Duration `json:"timeout,omitempty" description:"timeout when sending audit log to webhook server"`
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

// NewWebhookAuditSink creates a webhook audit sink. ctx owns the lifetime of
// dynamic TLS certificate watchers created for its transport.
func NewWebhookAuditSink(ctx context.Context, opts *WebhookAuditSinkOptions) (*WebhookAuditSink, error) {
	return NewWebhookAuditSinkWithTransport(ctx, opts, nil)
}

// NewWebhookAuditSinkWithTransport creates an audit sink whose requests use
// wrapper around the configured HTTP transport. ctx owns the lifetime of
// dynamic TLS certificate watchers created for that transport.
func NewWebhookAuditSinkWithTransport(ctx context.Context, opts *WebhookAuditSinkOptions, wrapper httpclient.TransportWrapper) (*WebhookAuditSink, error) {
	client, err := httpclient.NewClientFromOptionsWithTransport(ctx, &opts.Options, wrapper)
	if err != nil {
		return nil, err
	}
	return &WebhookAuditSink{httpclient: client, timeout: opts.Timeout}, nil
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
