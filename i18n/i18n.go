package i18n

import (
	"context"
	"fmt"
	"time"
)

// Localizer provides localization methods for translating messages.
type Localizer interface {
	// T translates a message with optional sprintf-style parameters.
	// Example: T("hello", "name", "John") -> "Hello, John!"
	T(key string, args ...any) string

	// Tf translates with named format parameters using template syntax.
	// Example: Tf("welcome", map[string]any{"name": "John", "age": 25})
	Tf(key string, params map[string]any) string

	// E returns a translated error.
	E(key string, args ...any) error

	// P handles plural translations based on count.
	// Example: P("items", 5, "count", 5) -> "5 items"
	P(key string, count int, args ...any) string

	// N formats numbers according to locale.
	N(number float64) string

	// D formats dates according to locale.
	D(time time.Time, format DateFormat) string

	// M formats money/currency.
	M(amount float64, currency string) string

	// Language returns current language code.
	Language() string

	// Exists checks if a translation key exists.
	Exists(key string) bool

	// MustT is like T but panics if the key doesn't exist.
	MustT(key string, args ...any) string
}

// Manager manages multiple localizers and translation loading.
type Manager interface {
	// GetLocalizer returns a localizer for the given language.
	GetLocalizer(lang string) Localizer

	// GetLocalizerFromContext extracts language from context and returns localizer.
	GetLocalizerFromContext(ctx context.Context) Localizer

	// LoadTranslations loads translation files from a directory.
	LoadTranslations(dir string, format Format) error

	// LoadTranslationsFromBytes loads translations from byte data.
	LoadTranslationsFromBytes(lang string, data []byte, format Format) error

	// AddTranslation adds a single translation key-value pair.
	AddTranslation(lang, key, value string) error

	// SetFallbackLanguage sets the fallback language (default: "en").
	SetFallbackLanguage(lang string)

	// SupportedLanguages returns list of loaded languages.
	SupportedLanguages() []string

	// DefaultLanguage returns the default/fallback language.
	DefaultLanguage() string
}

// DateFormat represents different date format styles.
type DateFormat int

const (
	DateFormatShort  DateFormat = iota // 1/2/06
	DateFormatMedium                   // Jan 2, 2006
	DateFormatLong                     // January 2, 2006
	DateFormatFull                     // Monday, January 2, 2006
)

// Format represents translation file format.
type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// PluralRule defines plural form selection logic for a language.
type PluralRule func(n int) string

// Common plural form names.
const (
	PluralFormZero  = "zero"
	PluralFormOne   = "one"
	PluralFormTwo   = "two"
	PluralFormFew   = "few"
	PluralFormMany  = "many"
	PluralFormOther = "other"
)

// Context keys for i18n.
type contextKey string

const (
	ContextKeyLanguage  contextKey = "i18n:language"
	ContextKeyLocalizer contextKey = "i18n:localizer"
)

// Default is the default localizer (simple passthrough).
var Default Localizer = &SimpleLocalizer{}

// SimpleLocalizer is a simple implementation that doesn't translate.
type SimpleLocalizer struct{}

func (s *SimpleLocalizer) T(message string, args ...any) string {
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

func (s *SimpleLocalizer) Tf(message string, params map[string]any) string {
	return message
}

func (s *SimpleLocalizer) E(message string, args ...any) error {
	return fmt.Errorf(message, args...)
}

func (s *SimpleLocalizer) P(key string, count int, args ...any) string {
	return s.T(key, args...)
}

func (s *SimpleLocalizer) N(number float64) string {
	return fmt.Sprintf("%g", number)
}

func (s *SimpleLocalizer) D(t time.Time, format DateFormat) string {
	return t.Format(time.RFC3339)
}

func (s *SimpleLocalizer) M(amount float64, currency string) string {
	return fmt.Sprintf("%.2f %s", amount, currency)
}

func (s *SimpleLocalizer) Language() string {
	return "en"
}

func (s *SimpleLocalizer) Exists(key string) bool {
	return false
}

func (s *SimpleLocalizer) MustT(key string, args ...any) string {
	return s.T(key, args...)
}

// FromContext extracts the localizer from context or returns default.
func FromContext(ctx context.Context) Localizer {
	if loc, ok := ctx.Value(ContextKeyLocalizer).(Localizer); ok {
		return loc
	}
	return Default
}

// T is a convenience function that gets localizer from context and translates.
func T(ctx context.Context, key string, args ...any) string {
	return FromContext(ctx).T(key, args...)
}

// Tf is a convenience function for template-based translation.
func Tf(ctx context.Context, key string, params map[string]any) string {
	return FromContext(ctx).Tf(key, params)
}

// E is a convenience function that returns a translated error.
func E(ctx context.Context, key string, args ...any) error {
	return FromContext(ctx).E(key, args...)
}

// P is a convenience function for plural translations.
func P(ctx context.Context, key string, count int, args ...any) string {
	return FromContext(ctx).P(key, count, args...)
}

// LanguageFromContext extracts the language code from context.
func LanguageFromContext(ctx context.Context) string {
	if lang, ok := ctx.Value(ContextKeyLanguage).(string); ok {
		return lang
	}
	return "en"
}
