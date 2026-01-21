package i18n

import (
	"context"
	"testing"
)

func TestSimpleLocalizer(t *testing.T) {
	loc := &SimpleLocalizer{}

	// Test T
	result := loc.T("hello %s", "world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result)
	}

	// Test E
	err := loc.E("error: %s", "failed")
	if err.Error() != "error: failed" {
		t.Errorf("expected 'error: failed', got '%s'", err.Error())
	}

	// Test Language
	if loc.Language() != "en" {
		t.Errorf("expected 'en', got '%s'", loc.Language())
	}

	// Test Exists (always false for SimpleLocalizer)
	if loc.Exists("any.key") {
		t.Error("SimpleLocalizer should always return false for Exists")
	}
}

func TestFromContext(t *testing.T) {
	// Test with empty context
	loc := FromContext(context.Background())
	if loc == nil {
		t.Error("FromContext should return Default localizer, not nil")
	}

	// Test with localizer in context
	customLoc := &SimpleLocalizer{}
	ctx := context.WithValue(context.Background(), ContextKeyLocalizer, customLoc)
	loc = FromContext(ctx)
	if loc != customLoc {
		t.Error("FromContext should return the localizer from context")
	}
}

func TestLanguageFromContext(t *testing.T) {
	// Test with empty context
	lang := LanguageFromContext(context.Background())
	if lang != "en" {
		t.Errorf("expected 'en', got '%s'", lang)
	}

	// Test with language in context
	ctx := context.WithValue(context.Background(), ContextKeyLanguage, "zh-CN")
	lang = LanguageFromContext(ctx)
	if lang != "zh-CN" {
		t.Errorf("expected 'zh-CN', got '%s'", lang)
	}
}

func TestConvenienceFunctions(t *testing.T) {
	ctx := context.Background()

	// Test T
	result := T(ctx, "hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}

	// Test E
	err := E(ctx, "error")
	if err.Error() != "error" {
		t.Errorf("expected 'error', got '%s'", err.Error())
	}
}
