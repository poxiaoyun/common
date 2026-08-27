package pprof

import (
	"context"
	"expvar"
	"net/http"
	"net/http/pprof"
	"os"

	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/rest/api"
)

// Handler returns an isolated handler for the expvar and pprof debug endpoints.
func Handler() http.Handler {
	// Keep unrelated handlers registered on DefaultServeMux out of the debug server.
	m := http.NewServeMux()
	m.Handle("GET /debug/vars", expvar.Handler())
	m.Handle("GET /debug/pprof/", http.HandlerFunc(pprof.Index))
	m.Handle("GET /debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	m.Handle("GET /debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	m.Handle("GET /debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	m.Handle("GET /debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	return m
}

// Run serves the debug endpoints on 127.0.0.1:6060 until ctx is canceled.
// PPROF_PORT overrides the complete listen address when set.
func Run(ctx context.Context) error {
	listenaddr := os.Getenv("PPROF_PORT")
	if listenaddr == "" {
		listenaddr = "127.0.0.1:6060"
	}
	ctx = log.NewContext(ctx, log.FromContext(ctx).WithValues("component", "pprof"))
	return api.ServeContext(ctx, listenaddr, Handler())
}
