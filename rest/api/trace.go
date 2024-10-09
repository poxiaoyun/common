package api

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

func NewOpenTelemetryFilter(tracer trace.Tracer) FilterFunc {
	otelhandler := otelhttp.NewMiddleware("operation")
	return func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		otelhandler(next).ServeHTTP(w, r)
	}
}
