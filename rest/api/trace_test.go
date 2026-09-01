package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestOpenTelemetryFilterUsesTraceProvider(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	NewOpenTelemetryFilter(provider).Process(httptest.NewRecorder(), request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
}

func TestEndUserTraceFilterRecordsAuthenticatedEndUser(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx, span := provider.Tracer("test").Start(context.Background(), "request")
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(
		WithAuthentication(ctx, Authentication{Subject: Subject{ID: "user-1"}}),
	)
	handlerCalled := false

	NewEndUserTraceFilter().Process(httptest.NewRecorder(), request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		handlerCalled = true
	}))
	span.End()

	if !handlerCalled {
		t.Fatal("handler was not called")
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	for _, attribute := range spans[0].Attributes() {
		if attribute.Key == semconv.EnduserIDKey && attribute.Value.AsString() == "user-1" {
			return
		}
	}
	t.Fatal("enduser.id attribute was not recorded")
}

func TestEndUserTraceFilterRecordsAnonymousUser(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx, span := provider.Tracer("test").Start(context.Background(), "request")
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(
		WithAuthentication(ctx, Authentication{Subject: Subject{ID: AnonymousSubjectID}}),
	)

	NewEndUserTraceFilter().Process(httptest.NewRecorder(), request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	span.End()

	for _, attribute := range recorder.Ended()[0].Attributes() {
		if attribute.Key == semconv.EnduserIDKey && attribute.Value.AsString() == AnonymousSubjectID {
			return
		}
	}
	t.Fatalf("enduser.id = %q was not recorded", AnonymousSubjectID)
}

func TestAuthorizationTraceFilterRecordsAuthorizationTarget(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx, span := provider.Tracer("test").Start(context.Background(), "request")
	request := httptest.NewRequest(http.MethodGet, "/orders/42", nil).WithContext(
		WithAttributes(ctx, &Attributes{
			Action:    "get",
			Resources: []AttributeResource{{Resource: "orders", Name: "42"}},
		}),
	)

	NewAuthorizationTraceFilter().Process(httptest.NewRecorder(), request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	span.End()

	attributes := map[attribute.Key]attribute.Value{}
	for _, item := range recorder.Ended()[0].Attributes() {
		attributes[item.Key] = item.Value
	}
	if got := attributes[attribute.Key("authorization.action")].AsString(); got != "get" {
		t.Fatalf("authorization.action = %q, want get", got)
	}
	resources := attributes[attribute.Key("authorization.resources")].AsStringSlice()
	if len(resources) != 1 || resources[0] != "orders:42" {
		t.Fatalf("authorization.resources = %v, want [orders:42]", resources)
	}
}
