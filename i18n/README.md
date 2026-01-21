# I18n - Internationalization Library

A comprehensive, production-ready internationalization (i18n) library for Go applications.

## Features

- ✅ **Multiple Languages Support** - Easy support for any language
- ✅ **Nested Translation Keys** - Organize translations with dot notation (`user.profile.title`)
- ✅ **Template-based Translations** - Named parameters using Go templates
- ✅ **Plural Forms** - Automatic plural handling for 15+ languages
- ✅ **HTTP Integration** - Built-in middleware for automatic language detection
- ✅ **Context Awareness** - Pass language preferences through context
- ✅ **Fallback Mechanism** - Graceful degradation to default language
- ✅ **Multiple Formats** - JSON and YAML translation files
- ✅ **Thread-Safe** - Safe for concurrent use
- ✅ **Zero Dependencies** - Uses only Go standard library and sigs.k8s.io/yaml

## Quick Start

### 1. Create Translation Files

Create translation files in `locales/` directory:

**locales/en.json:**
```json
{
  "common": {
    "hello": "Hello",
    "welcome": "Welcome, {{.name}}!"
  },
  "errors": {
    "not_found": "Resource not found"
  }
}
```

**locales/zh-CN.json:**
```json
{
  "common": {
    "hello": "你好",
    "welcome": "欢迎, {{.name}}!"
  },
  "errors": {
    "not_found": "未找到资源"
  }
}
```

### 2. Initialize the Manager

```go
package main

import (
    "xiaoshiai.cn/common/i18n"
)

func main() {
    // Create manager
    manager := i18n.NewManager()

    // Load translations
    err := manager.LoadTranslations("./locales", i18n.FormatJSON)
    if err != nil {
        panic(err)
    }

    // Set fallback language
    manager.SetFallbackLanguage("en")
}
```

### 3. Basic Usage

```go
// Get a localizer for a specific language
loc := manager.GetLocalizer("zh-CN")

// Simple translation
greeting := loc.T("common.hello") // "你好"

// Template-based translation
welcome := loc.Tf("common.welcome", map[string]any{
    "name": "张三",
}) // "欢迎, 张三!"

// Error translation
err := loc.E("errors.not_found") // error: "未找到资源"
```

### 4. HTTP Integration

```go
func main() {
    manager := i18n.NewManager()
    manager.LoadTranslations("./locales", i18n.FormatJSON)

    mux := http.NewServeMux()

    mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
        // Get localizer from context (automatically detected)
        loc := manager.GetLocalizerFromContext(r.Context())

        greeting := loc.T("common.hello")
        w.Write([]byte(greeting))
    })

    // Apply i18n middleware
    handler := i18n.Middleware(manager)(mux)

    http.ListenAndServe(":8080", handler)
}
```

The middleware automatically detects language from:
1. Query parameter (`?lang=zh-CN`)
2. Cookie (`language=zh-CN`)
3. Accept-Language header
4. Fallback to default language

### 5. Context-based Usage

```go
// In your service layer
func (s *UserService) CreateUser(ctx context.Context, name string) error {
    // Use context-aware translation functions
    message := i18n.T(ctx, "user.created")
    fmt.Println(message)

    // Return localized errors
    if err != nil {
        return i18n.E(ctx, "errors.validation_failed", err.Error())
    }

    return nil
}
```

## Advanced Features

### Plural Forms

Different languages have different plural rules. This library handles them automatically.

**Translation file:**
```json
{
  "items": {
    "one": "{{.count}} item",
    "other": "{{.count}} items"
  }
}
```

**Usage:**
```go
loc := manager.GetLocalizer("en")

// English has "one" and "other" forms
loc.P("items", 1, 1)  // "1 item"
loc.P("items", 5, 5)  // "5 items"

// Chinese has only "other" form
zhLoc := manager.GetLocalizer("zh-CN")
zhLoc.P("items", 1, 1)  // "1 个项目"
zhLoc.P("items", 5, 5)  // "5 个项目"
```

Supported plural forms:
- **zero** - for count = 0 (Arabic)
- **one** - for count = 1 (English, Spanish, etc.)
- **two** - for count = 2 (Arabic)
- **few** - for small counts (Russian, Polish, Czech)
- **many** - for large counts (Russian, Polish, Arabic)
- **other** - default/fallback form

Built-in rules for 15+ languages: English, Chinese, Japanese, Korean, Spanish, French, German, Russian, Polish, Arabic, Czech, Slovak, Italian, Portuguese, and more.

### Nested Keys

Organize translations hierarchically:

```json
{
  "user": {
    "profile": {
      "title": "User Profile",
      "settings": {
        "privacy": "Privacy Settings"
      }
    }
  }
}
```

```go
loc.T("user.profile.title")              // "User Profile"
loc.T("user.profile.settings.privacy")   // "Privacy Settings"
```

### Template Variables

Use Go template syntax for complex translations:

```json
{
  "user": {
    "info": "User {{.name}} (ID: {{.id}}) has {{.count}} items"
  }
}
```

```go
loc.Tf("user.info", map[string]any{
    "name": "John",
    "id": 123,
    "count": 5,
})
// "User John (ID: 123) has 5 items"
```

### Custom Language Detector

```go
type CustomDetector struct{}

func (d *CustomDetector) Detect(r *http.Request) string {
    // Custom logic to detect language
    // e.g., from user preferences in database
    return "zh-CN"
}

handler := i18n.MiddlewareWithDetector(manager, &CustomDetector{})(mux)
```

### Adding Translations Programmatically

```go
// Add single translation
manager.AddTranslation("en", "custom.key", "Custom value")
manager.AddTranslation("zh-CN", "custom.key", "自定义值")

// Load from byte data
jsonData := []byte(`{"key": "value"}`)
manager.LoadTranslationsFromBytes("en", jsonData, i18n.FormatJSON)
```

### Format Helpers

```go
// Number formatting
loc.N(1234.56)  // "1234.56"

// Date formatting
loc.D(time.Now(), i18n.DateFormatShort)   // "1/21/26"
loc.D(time.Now(), i18n.DateFormatMedium)  // "Jan 21, 2026"
loc.D(time.Now(), i18n.DateFormatLong)    // "January 21, 2026"
loc.D(time.Now(), i18n.DateFormatFull)    // "Tuesday, January 21, 2026"

// Currency formatting
loc.M(99.99, "USD")  // "99.99 USD"
```

## API Reference

### Manager Interface

```go
type Manager interface {
    GetLocalizer(lang string) Localizer
    GetLocalizerFromContext(ctx context.Context) Localizer
    LoadTranslations(dir string, format Format) error
    LoadTranslationsFromBytes(lang string, data []byte, format Format) error
    AddTranslation(lang, key, value string) error
    SetFallbackLanguage(lang string)
    SupportedLanguages() []string
    DefaultLanguage() string
}
```

### Localizer Interface

```go
type Localizer interface {
    T(key string, args ...any) string              // Simple translation
    Tf(key string, params map[string]any) string   // Template-based
    E(key string, args ...any) error                // Error translation
    P(key string, count int, args ...any) string   // Plural translation
    N(number float64) string                        // Number formatting
    D(time time.Time, format DateFormat) string    // Date formatting
    M(amount float64, currency string) string       // Money formatting
    Language() string                               // Current language
    Exists(key string) bool                         // Check key exists
    MustT(key string, args ...any) string          // Panics if missing
}
```

### Convenience Functions

```go
// Use from context
i18n.T(ctx, "key", args...)           // Translate
i18n.Tf(ctx, "key", params)           // Template translate
i18n.E(ctx, "key", args...)           // Error
i18n.P(ctx, "key", count, args...)    // Plural
i18n.FromContext(ctx)                 // Get localizer
i18n.LanguageFromContext(ctx)         // Get language code
```

## Translation File Formats

### JSON Format
```json
{
  "key": "value",
  "nested": {
    "key": "nested value"
  }
}
```

### YAML Format
```yaml
key: value
nested:
  key: nested value
```

## Best Practices

1. **Use Nested Keys** - Organize translations by feature/module
2. **Consistent Naming** - Use `entity.action` pattern (e.g., `user.created`, `order.deleted`)
3. **Complete Translations** - Provide all keys in fallback language
4. **Context-Aware** - Pass language through context, not function parameters
5. **Error Messages** - Always translate user-facing errors
6. **Testing** - Test with different languages to ensure proper rendering

## Examples

See the [examples](./examples) directory for complete working examples:
- Basic usage
- HTTP server integration
- Service layer integration
- Custom detectors
- Plural forms

Run the example:
```bash
cd examples
go run main.go
```

Then visit:
- http://localhost:8080/hello
- http://localhost:8080/hello?lang=zh-CN
- http://localhost:8080/hello?lang=ja

## License

This library is part of the xiaoshiai.cn/common package.
