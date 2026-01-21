package i18n

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("NewManager should return non-nil manager")
	}

	if mgr.DefaultLanguage() != "en" {
		t.Errorf("expected default language 'en', got '%s'", mgr.DefaultLanguage())
	}
}

func TestManagerAddTranslation(t *testing.T) {
	mgr := NewManager()

	// Add simple translation
	err := mgr.AddTranslation("en", "hello", "Hello")
	if err != nil {
		t.Fatalf("AddTranslation failed: %v", err)
	}

	// Add nested translation
	err = mgr.AddTranslation("en", "user.name", "User Name")
	if err != nil {
		t.Fatalf("AddTranslation failed: %v", err)
	}

	// Verify translations
	loc := mgr.GetLocalizer("en")
	if loc.T("hello") != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", loc.T("hello"))
	}

	if loc.T("user.name") != "User Name" {
		t.Errorf("expected 'User Name', got '%s'", loc.T("user.name"))
	}
}

func TestManagerLoadTranslationsFromBytes(t *testing.T) {
	mgr := NewManager()

	jsonData := []byte(`{
		"hello": "Hello",
		"user": {
			"name": "User Name"
		}
	}`)

	err := mgr.LoadTranslationsFromBytes("en", jsonData, FormatJSON)
	if err != nil {
		t.Fatalf("LoadTranslationsFromBytes failed: %v", err)
	}

	loc := mgr.GetLocalizer("en")
	if loc.T("hello") != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", loc.T("hello"))
	}

	if loc.T("user.name") != "User Name" {
		t.Errorf("expected 'User Name', got '%s'", loc.T("user.name"))
	}
}

func TestManagerLoadTranslations(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "i18n-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test translation files
	enFile := filepath.Join(tmpDir, "en.json")
	enData := []byte(`{"hello": "Hello", "goodbye": "Goodbye"}`)
	if err := os.WriteFile(enFile, enData, 0644); err != nil {
		t.Fatalf("failed to write en.json: %v", err)
	}

	zhFile := filepath.Join(tmpDir, "zh-CN.json")
	zhData := []byte(`{"hello": "你好", "goodbye": "再见"}`)
	if err := os.WriteFile(zhFile, zhData, 0644); err != nil {
		t.Fatalf("failed to write zh-CN.json: %v", err)
	}

	// Load translations
	mgr := NewManager()
	err = mgr.LoadTranslations(tmpDir, FormatJSON)
	if err != nil {
		t.Fatalf("LoadTranslations failed: %v", err)
	}

	// Verify English
	enLoc := mgr.GetLocalizer("en")
	if enLoc.T("hello") != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", enLoc.T("hello"))
	}

	// Verify Chinese
	zhLoc := mgr.GetLocalizer("zh-CN")
	if zhLoc.T("hello") != "你好" {
		t.Errorf("expected '你好', got '%s'", zhLoc.T("hello"))
	}

	// Check supported languages
	langs := mgr.SupportedLanguages()
	if len(langs) != 2 {
		t.Errorf("expected 2 languages, got %d", len(langs))
	}
}

func TestManagerFallback(t *testing.T) {
	mgr := NewManager()

	// Add English translations
	mgr.AddTranslation("en", "hello", "Hello")
	mgr.AddTranslation("en", "only.in.english", "Only in English")

	// Add Chinese translation (missing "only.in.english")
	mgr.AddTranslation("zh-CN", "hello", "你好")

	mgr.SetFallbackLanguage("en")

	// Get Chinese localizer
	loc := mgr.GetLocalizer("zh-CN")

	// Should get Chinese translation
	if loc.T("hello") != "你好" {
		t.Errorf("expected '你好', got '%s'", loc.T("hello"))
	}

	// Should fallback to English
	if loc.T("only.in.english") != "Only in English" {
		t.Errorf("expected 'Only in English', got '%s'", loc.T("only.in.english"))
	}
}

func TestManagerGetLocalizerFromContext(t *testing.T) {
	mgr := NewManager()
	mgr.AddTranslation("en", "hello", "Hello")
	mgr.AddTranslation("zh-CN", "hello", "你好")

	// Test with language in context
	ctx := context.WithValue(context.Background(), ContextKeyLanguage, "zh-CN")
	loc := mgr.GetLocalizerFromContext(ctx)

	if loc.T("hello") != "你好" {
		t.Errorf("expected '你好', got '%s'", loc.T("hello"))
	}

	// Test with localizer in context
	zhLoc := mgr.GetLocalizer("zh-CN")
	ctx2 := context.WithValue(context.Background(), ContextKeyLocalizer, zhLoc)
	loc2 := mgr.GetLocalizerFromContext(ctx2)

	if loc2.T("hello") != "你好" {
		t.Errorf("expected '你好', got '%s'", loc2.T("hello"))
	}

	// Test with empty context (should use fallback)
	loc3 := mgr.GetLocalizerFromContext(context.Background())
	if loc3.T("hello") != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", loc3.T("hello"))
	}
}
