package api

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	"xiaoshiai.cn/common/log"
)

// OpenTelemetryPlugin traces registered routes with OpenTelemetry HTTP
// semantic conventions.
type OpenTelemetryPlugin struct {
	// TraceProvider creates the route spans.
	TraceProvider trace.TracerProvider
}

// Install satisfies Plugin. Route tracing is installed by OnRoute.
func (o OpenTelemetryPlugin) Install(*API) error {
	return nil
}

// OnRoute prepends HTTP tracing to route filters.
func (o OpenTelemetryPlugin) OnRoute(route *Route) error {
	middleware := otelhttp.NewMiddleware(route.Path, otelhttp.WithTracerProvider(o.TraceProvider))
	filter := FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeAttribute := semconv.HTTPRoute(route.Path)
			trace.SpanFromContext(r.Context()).SetAttributes(routeAttribute)
			labeler, _ := otelhttp.LabelerFromContext(r.Context())
			labeler.Add(routeAttribute)
			next.ServeHTTP(w, r)
		})
		middleware(handler).ServeHTTP(w, r)
	})
	route.Filters = append([]Filter{filter}, route.Filters...)
	return nil
}

// NewOpenTelemetryFilter traces requests with provider.
func NewOpenTelemetryFilter(provider trace.TracerProvider) FilterFunc {
	otelhandler := otelhttp.NewMiddleware("operation", otelhttp.WithTracerProvider(provider))
	return func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		otelhandler(next).ServeHTTP(w, r)
	}
}

// NewEndUserTraceFilter records the authenticated end-user subject identifier
// on the current span. Install it after NewAuthenticationFilter only when the
// authentication domain represents its subjects as end users and collecting
// end-user identifiers has been explicitly enabled.
func NewEndUserTraceFilter() FilterFunc {
	return func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		authentication := AuthenticationFromContext(r.Context())
		trace.SpanFromContext(r.Context()).SetAttributes(semconv.EnduserID(authentication.ID))
		next.ServeHTTP(w, r)
	}
}

// NewAuthorizationTraceFilter records the request authorization target on the
// current span. Install it after NewAttributeExtractionFilter. Resource names may be
// sensitive or high-cardinality, so callers must enable this filter explicitly.
func NewAuthorizationTraceFilter() FilterFunc {
	return func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		authorization := AttributesFromContext(r.Context())
		resources := make([]string, 0, len(authorization.Resources))
		for _, resource := range authorization.Resources {
			resources = append(resources, resource.Resource+":"+resource.Name)
		}
		trace.SpanFromContext(r.Context()).SetAttributes(
			attribute.String("authorization.action", authorization.Action),
			attribute.StringSlice("authorization.resources", resources),
		)
		next.ServeHTTP(w, r)
	}
}

// NewTraceProvider creates and globally installs the configured trace provider.
func NewTraceProvider(ctx context.Context, options *TelemetryOptions) (*sdktrace.TracerProvider, func(), error) {
	newopts := []sdktrace.TracerProviderOption{}
	if options.TraceAddr != "" {
		exp, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, nil, err
		}
		newopts = append(newopts, sdktrace.WithBatcher(exp))
	}
	if options.SampleRate >= 0 {
		newopts = append(newopts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(float64(options.SampleRate)/100)))
	}
	tp := sdktrace.NewTracerProvider(newopts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	shutdown := func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.FromContext(ctx).Error(err, "failed to shutdown trace provider")
		}
	}
	return tp, shutdown, nil
}
