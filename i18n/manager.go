package i18n

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"
)

// manager implements the Manager interface.
type manager struct {
	mu           sync.RWMutex
	translations map[string]map[string]any // lang -> nested translations
	fallbackLang string
	pluralRules  map[string]PluralRule
}

// NewManager creates a new i18n manager.
func NewManager() Manager {
	return &manager{
		translations: make(map[string]map[string]any),
		fallbackLang: "en",
		pluralRules:  DefaultPluralRules(),
	}
}

func (m *manager) GetLocalizer(lang string) Localizer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trans := m.translations[lang]
	fallback := m.translations[m.fallbackLang]
	pluralRule := m.pluralRules[lang]

	if pluralRule == nil {
		pluralRule = m.pluralRules[m.fallbackLang]
	}
	if pluralRule == nil {
		pluralRule = DefaultPluralRules()["en"]
	}

	return &localizer{
		lang:         lang,
		translations: trans,
		fallback:     fallback,
		pluralRule:   pluralRule,
	}
}

func (m *manager) GetLocalizerFromContext(ctx context.Context) Localizer {
	if loc, ok := ctx.Value(ContextKeyLocalizer).(Localizer); ok {
		return loc
	}
	if lang, ok := ctx.Value(ContextKeyLanguage).(string); ok {
		return m.GetLocalizer(lang)
	}
	return m.GetLocalizer(m.fallbackLang)
}

func (m *manager) LoadTranslations(dir string, format Format) error {
	pattern := filepath.Join(dir, "*."+string(format))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob translation files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no translation files found in %s with format %s", dir, format)
	}

	for _, file := range files {
		// Extract language code from filename (e.g., "en.json" -> "en")
		lang := strings.TrimSuffix(filepath.Base(file), "."+string(format))

		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read translation file %s: %w", file, err)
		}

		if err := m.LoadTranslationsFromBytes(lang, data, format); err != nil {
			return fmt.Errorf("failed to load translations from %s: %w", file, err)
		}
	}

	return nil
}

func (m *manager) LoadTranslationsFromBytes(lang string, data []byte, format Format) error {
	var translations map[string]any

	switch format {
	case FormatJSON:
		if err := json.Unmarshal(data, &translations); err != nil {
			return fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
	case FormatYAML:
		if err := yaml.Unmarshal(data, &translations); err != nil {
			return fmt.Errorf("failed to unmarshal YAML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	m.mu.Lock()
	m.translations[lang] = translations
	m.mu.Unlock()

	return nil
}

func (m *manager) AddTranslation(lang, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.translations[lang] == nil {
		m.translations[lang] = make(map[string]any)
	}

	// Support nested keys like "user.profile.title"
	parts := strings.Split(key, ".")
	current := m.translations[lang]

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part, set the value
			current[part] = value
		} else {
			// Intermediate part, ensure it's a map
			if _, ok := current[part]; !ok {
				current[part] = make(map[string]any)
			}
			if nested, ok := current[part].(map[string]any); ok {
				current = nested
			} else {
				return fmt.Errorf("cannot set nested key %s: %s is not a map", key, part)
			}
		}
	}

	return nil
}

func (m *manager) SetFallbackLanguage(lang string) {
	m.mu.Lock()
	m.fallbackLang = lang
	m.mu.Unlock()
}

func (m *manager) SupportedLanguages() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	langs := make([]string, 0, len(m.translations))
	for lang := range m.translations {
		langs = append(langs, lang)
	}
	return langs
}

func (m *manager) DefaultLanguage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fallbackLang
}
