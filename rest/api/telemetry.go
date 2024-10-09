package api

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type OpenTelemetryPlugin struct {
	TraceProvider trace.TracerProvider
}

func (o OpenTelemetryPlugin) Install(m *API) error {
	return nil
}

func (o OpenTelemetryPlugin) OnRoute(route *Route) error {
	route.Handler = otelhttp.WithRouteTag(route.Path, route.Handler)
	// inject filter
	midware := otelhttp.NewMiddleware(route.Path, otelhttp.WithTracerProvider(o.TraceProvider))
	filter := FilterFunc(func(w http.ResponseWriter, r *http.Request, next http.Handler) {
		nn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			vars := PathVars(r)
			vals := make([]string, 0, len(vars))
			for _, v := range vars {
				vals = append(vals, v.Key+"="+v.Value)
			}
			sp := trace.SpanFromContext(r.Context())
			sp.SetAttributes(
				attribute.StringSlice("pathvars", vals),
			)
		})
		midware(nn).ServeHTTP(w, r)
	})
	// prepend
	route.Filters = append([]Filter{filter}, route.Filters...)
	return nil
}

func NewDefaultTelmetryOptions() *TelmetryOptions {
	return &TelmetryOptions{
		SampleRate: 100,
	}
}

type TelmetryOptions struct {
	TraceAddr  string `json:"traceAddr,omitempty"`
	MetricAddr string `json:"metricAddr,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty" description:"sample rate for trace 0-100"`
}

func NewTraceProvider(ctx context.Context, options *TelmetryOptions) (*sdktrace.TracerProvider, func(), error) {
	newopts := []sdktrace.TracerProviderOption{}
	if options.MetricAddr != "" {
		exp, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, nil, err
		}
		newopts = append(newopts, sdktrace.WithBatcher(exp))
	}
	if options.SampleRate >= 0 {
		newopts = append(newopts, sdktrace.WithSampler(sdktrace.TraceIDRatioBased(float64(options.SampleRate)/100)))
	}
	// For the demonstration, use sdktrace.AlwaysSample sampler to sample all traces.
	// In a production application, use sdktrace.ProbabilitySampler with a desired probability.
	tp := sdktrace.NewTracerProvider(newopts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	deferfunc := func() {
		if err := tp.Shutdown(ctx); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to shutdown trace provider")
		}
	}
	return tp, deferfunc, nil
}

func NewMeterProvider(ctx context.Context, options *TelmetryOptions) (*sdkmetric.MeterProvider, func(), error) {
	exp, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exp),
		))
	otel.SetMeterProvider(mp)

	deferfunc := func() {
		if err := mp.Shutdown(ctx); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to shutdown meter provider")
		}
	}
	return mp, deferfunc, nil
}
