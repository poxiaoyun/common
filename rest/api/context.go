package api

import (
	"context"
	"net/http"
	"time"
)

// RequestContext is a map of string to any
// it is used to pass data between filters
// RequestContext is mutable and can pass values back to parent, but context.Context is not
type RequestContext map[string]any

type filterContextKey string

var RequestContextKey = filterContextKey("request-context")

// SetContextValue sets a value in the RequestContext
// it can pass among filters in the same request
func SetContextValue(ctx context.Context, key string, value any) context.Context {
	fc, _ := ctx.Value(RequestContextKey).(RequestContext)
	if fc != nil {
		// update filter context value
		fc[key] = value
		return ctx
	}
	// init filter context
	return context.WithValue(ctx, RequestContextKey, RequestContext{key: value})
}

// GetContextValue gets a value from the RequestContext
// it can pass among filters in the same request
func GetContextValue[T any](ctx context.Context, key string) T {
	fc, _ := ctx.Value(RequestContextKey).(RequestContext)
	if fc == nil {
		return *new(T)
	}
	val, ok := fc[key].(T)
	if !ok {
		return *new(T)
	}
	return val
}

var _ Filter = ContextInitializerFilter{}

// ContextInitializerFilter is a filter that inject the [RequestContext] into the request context
type ContextInitializerFilter struct{}

// Process implements Filter.
func (c ContextInitializerFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	ctx := SetContextValue(r.Context(), "start-time", time.Now())
	next.ServeHTTP(w, r.WithContext(ctx))
}
