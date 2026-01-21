package i18n

import (
	"testing"
	"time"
)

func TestLocalizerT(t *testing.T) {
	translations := map[string]any{
		"hello":   "Hello",
		"welcome": "Welcome %s",
		"nested": map[string]any{
			"key": "Nested Value",
		},
	}

	loc := &localizer{
		lang:         "en",
		translations: translations,
		fallback:     nil,
		pluralRule:   DefaultPluralRules()["en"],
	}

	// Simple translation
	if loc.T("hello") != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", loc.T("hello"))
	}

	// Translation with sprintf
	if loc.T("welcome", "John") != "Welcome John" {
		t.Errorf("expected 'Welcome John', got '%s'", loc.T("welcome", "John"))
	}

	// Nested translation
	if loc.T("nested.key") != "Nested Value" {
		t.Errorf("expected 'Nested Value', got '%s'", loc.T("nested.key"))
	}

	// Missing key returns key
	if loc.T("missing.key") != "missing.key" {
		t.Errorf("expected 'missing.key', got '%s'", loc.T("missing.key"))
	}
}

func TestLocalizerTf(t *testing.T) {
	translations := map[string]any{
		"welcome": "Welcome, {{.name}}! You have {{.count}} messages.",
	}

	loc := &localizer{
		lang:         "en",
		translations: translations,
		fallback:     nil,
		pluralRule:   DefaultPluralRules()["en"],
	}

	result := loc.Tf("welcome", map[string]any{
		"name":  "John",
		"count": 5,
	})

	expected := "Welcome, John! You have 5 messages."
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestLocalizerP(t *testing.T) {
	translations := map[string]any{
		"items": map[string]any{
			"one":   "{{.count}} item",
			"other": "{{.count}} items",
		},
	}

	loc := &localizer{
		lang:         "en",
		translations: translations,
		fallback:     nil,
		pluralRule:   DefaultPluralRules()["en"],
	}

	// Singular - count is automatically passed to template
	result1 := loc.P("items", 1)
	if result1 != "1 item" {
		t.Errorf("expected '1 item', got '%s'", result1)
	}

	// Plural - count is automatically passed to template
	result2 := loc.P("items", 5)
	if result2 != "5 items" {
		t.Errorf("expected '5 items', got '%s'", result2)
	}
}

func TestLocalizerExists(t *testing.T) {
	translations := map[string]any{
		"hello": "Hello",
		"nested": map[string]any{
			"key": "Value",
		},
	}

	loc := &localizer{
		lang:         "en",
		translations: translations,
		fallback:     nil,
		pluralRule:   DefaultPluralRules()["en"],
	}

	if !loc.Exists("hello") {
		t.Error("expected 'hello' to exist")
	}

	if !loc.Exists("nested.key") {
		t.Error("expected 'nested.key' to exist")
	}

	if loc.Exists("missing") {
		t.Error("expected 'missing' to not exist")
	}
}

func TestLocalizerMustT(t *testing.T) {
	translations := map[string]any{
		"hello": "Hello",
	}

	loc := &localizer{
		lang:         "en",
		translations: translations,
		fallback:     nil,
		pluralRule:   DefaultPluralRules()["en"],
	}

	// Should not panic for existing key
	result := loc.MustT("hello")
	if result != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", result)
	}

	// Should panic for missing key
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustT should panic for missing key")
		}
	}()
	loc.MustT("missing")
}

func TestLocalizerDateFormat(t *testing.T) {
	loc := &localizer{
		lang:       "en",
		pluralRule: DefaultPluralRules()["en"],
	}

	testTime := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		format   DateFormat
		expected string
	}{
		{DateFormatShort, "1/21/26"},
		{DateFormatMedium, "Jan 21, 2026"},
		{DateFormatLong, "January 21, 2026"},
		{DateFormatFull, "Wednesday, January 21, 2026"},
	}

	for _, test := range tests {
		result := loc.D(testTime, test.format)
		if result != test.expected {
			t.Errorf("for format %v, expected '%s', got '%s'", test.format, test.expected, result)
		}
	}
}

func TestGetNestedValue(t *testing.T) {
	m := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": "value",
			},
		},
		"simple": "simple value",
	}

	tests := []struct {
		keys     []string
		expected string
	}{
		{[]string{"simple"}, "simple value"},
		{[]string{"level1", "level2", "level3"}, "value"},
		{[]string{"missing"}, ""},
		{[]string{"level1", "missing"}, ""},
	}

	for _, test := range tests {
		result := getNestedValue(m, test.keys)
		if result != test.expected {
			t.Errorf("for keys %v, expected '%s', got '%s'", test.keys, test.expected, result)
		}
	}
}
