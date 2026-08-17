package eventbus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHandlerFunc(t *testing.T) {
	want := errors.New("handle event")
	handler := HandlerFunc(func(context.Context, Event) error {
		return want
	})

	if got := handler.Handle(context.Background(), Event{}); !errors.Is(got, want) {
		t.Fatalf("Handle() error = %v, want %v", got, want)
	}
}

func TestNoRetry(t *testing.T) {
	want := errors.New("invalid payload")
	err := NoRetry(want)

	if !errors.Is(err, want) {
		t.Fatalf("NoRetry() does not wrap %v", want)
	}
	if !IsNoRetry(err) {
		t.Fatal("IsNoRetry() = false, want true")
	}
	if NoRetry(nil) != nil {
		t.Fatal("NoRetry(nil) != nil")
	}
}

func TestRetryAfter(t *testing.T) {
	want := errors.New("temporarily unavailable")
	err := RetryAfter(want, 5*time.Second)

	if !errors.Is(err, want) {
		t.Fatalf("RetryAfter() does not wrap %v", want)
	}
	if got, ok := RetryDelay(err); !ok || got != 5*time.Second {
		t.Fatalf("RetryDelay() = (%v, %v), want (%v, true)", got, ok, 5*time.Second)
	}
	if RetryAfter(nil, time.Second) != nil {
		t.Fatal("RetryAfter(nil) != nil")
	}
}

func TestNoRetryTakesPrecedence(t *testing.T) {
	err := RetryAfter(NoRetry(errors.New("permanent")), time.Minute)

	if !IsNoRetry(err) {
		t.Fatal("IsNoRetry() = false, want true")
	}
	if delay, ok := RetryDelay(err); ok || delay != 0 {
		t.Fatalf("RetryDelay() = (%v, %v), want (0, false)", delay, ok)
	}
}

func TestEventTypePatterns(t *testing.T) {
	compiled, err := CompileEventTypePattern("order.*.v1")
	if err != nil {
		t.Fatalf("CompileEventTypePattern() error = %v", err)
	}
	if !compiled.Match("order.created.v1") {
		t.Fatal("Match() = false, want true")
	}
	if compiled.Match("order.eu.created.v1") {
		t.Fatal("Match() = true, want false")
	}
	compiled, err = CompileEventTypePattern("order.foo*")
	if err != nil {
		t.Fatalf("CompileEventTypePattern() error = %v", err)
	}
	if !compiled.Match("order.foobar") {
		t.Fatal("Match() = false, want true")
	}

	for _, value := range []string{
		"",
		"order.*",
		"order..created",
		"order.created,admin",
		"order.{created}",
	} {
		if err := ValidateEventType(value); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("ValidateEventType(%q) error = %v, want ErrInvalidArgument", value, err)
		}
	}
	for _, value := range []string{
		"",
		"order.**.v1",
		"**.order",
		"order.{**,created}.v1",
		"order.**,created.v1",
		"order.**.ignored,**",
		"order,**",
		"order.foo**",
	} {
		if _, err := CompileEventTypePattern(value); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("CompileEventTypePattern(%q) error = %v, want ErrInvalidArgument", value, err)
		}
	}
}

func FuzzCompileEventTypePattern(f *testing.F) {
	for _, value := range []string{
		"order.created.v1",
		"order.foo*",
		"order.**",
		"order.{**,created}.v1",
		"order.**.ignored,**",
		"order,**",
	} {
		f.Add(value)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if _, err := CompileEventTypePattern(value); err != nil {
			return
		}
		doubleStar := strings.Index(value, "**")
		if doubleStar >= 0 && (doubleStar != len(value)-2 || doubleStar > 0 && value[doubleStar-1] != '.') {
			t.Fatalf("CompileEventTypePattern(%q) accepted hidden **", value)
		}
	})
}
