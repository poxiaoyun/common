package api

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"xiaoshiai.cn/common/log"
)

// NewDefaultTelemetryOptions returns the default telemetry configuration.
func NewDefaultTelemetryOptions() *TelemetryOptions {
	return &TelemetryOptions{
		SampleRate: 100,
	}
}

// TelemetryOptions configures trace and metric providers.
type TelemetryOptions struct {
	TraceAddr  string `json:"traceAddr,omitempty"`
	MetricAddr string `json:"metricAddr,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty" description:"sample rate for trace 0-100"`
}

func NewMeterProvider(ctx context.Context, options *TelemetryOptions) (*sdkmetric.MeterProvider, func(), error) {
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
			log.FromContext(ctx).Error(err, "failed to shutdown meter provider")
		}
	}
	return mp, deferfunc, nil
}
