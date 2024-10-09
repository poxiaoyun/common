// Copyright 2023 The Kubegems Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"xiaoshiai.cn/common/log"
)

type Filter interface {
	Process(w http.ResponseWriter, r *http.Request, next http.Handler)
}

type FilterFunc func(w http.ResponseWriter, r *http.Request, next http.Handler)

func (f FilterFunc) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	f(w, r, next)
}

type Filters []Filter

func (fs Filters) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if len(fs) == 0 {
		next.ServeHTTP(w, r)
		return
	}
	fs[0].Process(w, r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs[1:].Process(w, r, next)
	}))
}

// FilterContext is a map of string to any
// it is used to pass data between filters
// FilterContext is mutable and can pass values back to parent, but context.Context is not
type FilterContext map[string]any

type filterContextKey string

var FilterContextKey = filterContextKey("filter-context")

func SetContextValue(ctx context.Context, key string, value any) context.Context {
	fc, _ := ctx.Value(FilterContextKey).(FilterContext)
	if fc != nil {
		// update filter context value
		fc[key] = value
		return ctx
	}
	// init filter context
	return context.WithValue(ctx, FilterContextKey, FilterContext{key: value})
}

func GetContextValue[T any](ctx context.Context, key string) T {
	fc, _ := ctx.Value(FilterContextKey).(FilterContext)
	if fc == nil {
		return *new(T)
	}
	val, ok := fc[key].(T)
	if !ok {
		return *new(T)
	}
	return val
}

func NewCORSFilter() Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		orgin := r.Header.Get("Origin")
		if orgin == "" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", orgin)
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		next.ServeHTTP(w, r)
	})
}

func NewConditionFilter(cond func(r *http.Request) bool, filter Filter) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		if cond(r) {
			filter.Process(w, r, next)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

func NoopFilter() Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		next.ServeHTTP(w, r)
	})
}

func LoggingFilter(log log.Logger) Filter {
	return FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		start := time.Now()
		r = r.WithContext(SetContextValue(r.Context(), "start-time", start))
		next.ServeHTTP(w, r)
		reqpath := r.URL.Path
		i := strings.Index(reqpath, "?")
		if i != -1 {
			reqpath = reqpath[:i]
		}
		auditlog := AuditLogFromContext(r.Context())
		if auditlog != nil && auditlog.Response.StatusCode != 0 {
			log.Info(reqpath, "method", r.Method, "ip", auditlog.Request.ClientIP, "status", auditlog.Response.StatusCode, "duration", time.Since(start).String())
		} else {
			log.Info(reqpath, "method", r.Method, "remote", r.RemoteAddr, "duration", time.Since(start).String())
		}
	})
}
