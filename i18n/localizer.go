package i18n

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// localizer implements the Localizer interface.
type localizer struct {
	lang         string
	translations map[string]any
	fallback     map[string]any
	pluralRule   PluralRule
}

func (l *localizer) T(key string, args ...any) string {
	val := l.get(key)
	if val == "" {
		return key
	}

	// Simple sprintf if args are provided
	if len(args) > 0 {
		return fmt.Sprintf(val, args...)
	}

	return val
}

func (l *localizer) Tf(key string, params map[string]any) string {
	val := l.get(key)
	if val == "" {
		return key
	}

	// Use text/template for named parameters
	tmpl, err := template.New(key).Parse(val)
	if err != nil {
		// If template parsing fails, return the raw value
		return val
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		// If execution fails, return the raw value
		return val
	}

	return buf.String()
}

func (l *localizer) E(key string, args ...any) error {
	return fmt.Errorf("%s", l.T(key, args...))
}

func (l *localizer) P(key string, count int, args ...any) string {
	pluralForm := l.pluralRule(count)
	fullKey := key + "." + pluralForm

	val := l.get(fullKey)
	if val == "" {
		// Try to fall back to the base key
		val = l.get(key)
		if val == "" {
			return key
		}
	}

	// Use template if it contains {{
	if strings.Contains(val, "{{") {
		// Build params map from args
		params := map[string]any{"count": count}
		for i := 0; i < len(args)-1; i += 2 {
			if key, ok := args[i].(string); ok && i+1 < len(args) {
				params[key] = args[i+1]
			}
		}

		tmpl, err := template.New(key).Parse(val)
		if err == nil {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, params); err == nil {
				return buf.String()
			}
		}
	}

	// Simple sprintf if args are provided
	if len(args) > 0 {
		return fmt.Sprintf(val, args...)
	}

	return val
}

func (l *localizer) N(number float64) string {
	// Basic number formatting
	// In a production system, this would use locale-specific formatting
	return fmt.Sprintf("%.2f", number)
}

func (l *localizer) D(t time.Time, format DateFormat) string {
	// Basic date formatting
	// In a production system, this would use locale-specific formatting
	switch format {
	case DateFormatShort:
		return t.Format("1/2/06")
	case DateFormatMedium:
		return t.Format("Jan 2, 2006")
	case DateFormatLong:
		return t.Format("January 2, 2006")
	case DateFormatFull:
		return t.Format("Monday, January 2, 2006")
	default:
		return t.Format("1/2/06")
	}
}

func (l *localizer) M(amount float64, currency string) string {
	// Basic money formatting
	// In a production system, this would use locale-specific formatting
	return fmt.Sprintf("%.2f %s", amount, currency)
}

func (l *localizer) Language() string {
	return l.lang
}

func (l *localizer) Exists(key string) bool {
	return l.get(key) != ""
}

func (l *localizer) MustT(key string, args ...any) string {
	if !l.Exists(key) {
		panic(fmt.Sprintf("translation key not found: %s", key))
	}
	return l.T(key, args...)
}

// get retrieves a translation value by key, supporting nested keys.
func (l *localizer) get(key string) string {
	// Split key by dots for nested access
	parts := strings.Split(key, ".")

	// Try main translations
	if val := getNestedValue(l.translations, parts); val != "" {
		return val
	}

	// Try fallback
	if val := getNestedValue(l.fallback, parts); val != "" {
		return val
	}

	return ""
}

// getNestedValue retrieves a nested value from a map using a key path.
func getNestedValue(m map[string]any, keys []string) string {
	if len(keys) == 0 || m == nil {
		return ""
	}

	current := m
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return ""
		}

		if i == len(keys)-1 {
			// Last key, should be string
			if str, ok := val.(string); ok {
				return str
			}
			return ""
		}

		// Continue nested access
		if nested, ok := val.(map[string]any); ok {
			current = nested
		} else {
			return ""
		}
	}

	return ""
}
