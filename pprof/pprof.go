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

// ServeDebug provides a debug endpoint
func Handler() http.Handler {
	// don't use the default http server mux to make sure nothing gets registered
	// that we don't want to expose via containerd
	m := http.NewServeMux()
	m.Handle("/debug/vars", expvar.Handler())
	m.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	m.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	m.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	m.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	m.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	return m
}

func Run(ctx context.Context) error {
	listenaddr := os.Getenv("PPROF_PORT")
	if listenaddr == "" {
		listenaddr = ":6060"
	}
	ctx = log.NewContext(ctx, log.FromContext(ctx).WithValues("component", "pprof"))
	return api.ServeContext(ctx, listenaddr, Handler())
}
