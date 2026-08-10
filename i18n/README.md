# i18n

Context-based internationalization for Go applications, with JSON/YAML
translations, templates, plural forms, fallbacks, and HTTP language detection.

## Quick Start

Use the package-level helpers in handlers and service code. Assuming `ctx`
contains the English localizer loaded later in this guide:

```go
message := i18n.T(ctx, "common.hello") // "Hello"

welcome := i18n.Tf(ctx, "common.welcome", map[string]any{
    "name": "Alice",
}) // "Welcome, Alice!"

err := i18n.E(ctx, "errors.not_found") // error: "Resource not found"
items := i18n.P(ctx, "items", 3)       // "3 items"
```

Pass the same `context.Context` through the application. The HTTP middleware
puts the detected language and localizer into the request context.

## Usage

### 1. Initialize i18n

```go
manager := i18n.NewManager()

if err := manager.LoadTranslations("./locales", i18n.FormatJSON); err != nil {
    log.Fatal(err)
}

manager.SetFallbackLanguage("en")
```

Use `i18n.FormatYAML` for YAML files.

### 2. Create translation files

`locales/en.json`:

```json
{
  "common": {
    "hello": "Hello",
    "welcome": "Welcome, {{.name}}!"
  },
  "errors": {
    "not_found": "Resource not found"
  },
  "items": {
    "one": "{{.count}} item",
    "other": "{{.count}} items"
  }
}
```

File names are used as language codes, for example `en.json` and
`zh-CN.json`.

### 3. Add the HTTP middleware

```go
mux := http.NewServeMux()

mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
    message := i18n.T(r.Context(), "common.hello")
    _, _ = w.Write([]byte(message))
})

handler := i18n.Middleware(manager)(mux)
log.Fatal(http.ListenAndServe(":8080", handler))
```

The default detector checks, in order:

1. Query parameter: `?lang=zh-CN`
2. Cookie: `language=zh-CN`
3. `Accept-Language` header
4. Fallback language

### 4. Pass context into service code

```go
func (s *UserService) Find(ctx context.Context, id string) error {
    _, err := s.repo.Find(ctx, id)
    if errors.Is(err, ErrNotFound) {
        return i18n.E(ctx, "errors.not_found")
    }
    return err
}
```

### 5. Use a fixed language when needed

For work without a request context, such as a background job, get a localizer
explicitly:

```go
loc := manager.GetLocalizer("zh-CN")
message := loc.T("common.hello")
```

## Translation Patterns

### Nested keys

```json
{
  "user": {
    "profile": {
      "title": "User Profile"
    }
  }
}
```

```go
i18n.T(ctx, "user.profile.title")
```

### Templates

Templates use Go template syntax:

```json
{
  "welcome": "Welcome, {{.name}}!"
}
```

```go
i18n.Tf(ctx, "welcome", map[string]any{"name": "Alice"})
```

For `fmt.Sprintf`-style translations, pass positional arguments to `T`:

```json
{
  "welcome": "Welcome, %s!"
}
```

```go
i18n.T(ctx, "welcome", "Alice")
```

### Plurals

```json
{
  "items": {
    "one": "{{.count}} item",
    "other": "{{.count}} items"
  }
}
```

```go
i18n.P(ctx, "items", 1) // "1 item"
i18n.P(ctx, "items", 5) // "5 items"
```

Plural rules are included for common languages. Each rule may select `zero`,
`one`, `two`, `few`, `many`, or `other`.

## Other APIs

### Custom language detection

```go
type UserLanguageDetector struct{}

func (d *UserLanguageDetector) Detect(r *http.Request) string {
    return "zh-CN"
}

handler := i18n.MiddlewareWithDetector(
    manager,
    &UserLanguageDetector{},
)(mux)
```

### Add or load translations

```go
manager.AddTranslation("en", "custom.key", "Custom value")
manager.LoadTranslationsFromBytes("en", data, i18n.FormatJSON)
```

### Localizer helpers

Formatting helpers are available from an explicit localizer:

```go
loc := manager.GetLocalizer("en")

loc.N(1234.56)
loc.D(time.Now(), i18n.DateFormatLong)
loc.M(99.99, "USD")
loc.Exists("common.hello")
loc.MustT("common.hello")
```

## Context Helpers

```go
i18n.T(ctx, "key", args...)
i18n.Tf(ctx, "key", params)
i18n.E(ctx, "key", args...)
i18n.P(ctx, "key", count, args...)
i18n.FromContext(ctx)
i18n.LanguageFromContext(ctx)
```

See [`examples`](./examples) for a complete runnable HTTP server.
