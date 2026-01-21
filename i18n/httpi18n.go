package i18n

import (
	"context"
	"net/http"
	"strings"
)

// ParseAcceptLanguage parses the Accept-Language header with quality values.
// Example: "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7"
// Returns languages in order of preference.
func ParseAcceptLanguage(header string) []string {
	if header == "" {
		return nil
	}
	var languages []string
	for part := range strings.SplitSeq(header, ",") {
		lang := strings.TrimSpace(strings.Split(part, ";")[0])
		if lang != "" {
			languages = append(languages, lang)
		}
	}

	return languages
}

// ParseLanguageFromHeader is a simple language parser for backward compatibility.
// Deprecated: Use ParseAcceptLanguage or LanguageDetector instead.
func ParseLanguageFromHeader(header http.Header) string {
	s := strings.Split(header.Get("Accept-Language"), ",")[0]
	return s
}

// LanguageDetector detects language from various sources.
type LanguageDetector interface {
	Detect(r *http.Request) string
}

// DefaultDetector tries multiple sources in order:
// 1. Query parameter (?lang=en)
// 2. Cookie (language=en)
// 3. Accept-Language header
// 4. Fallback language
type DefaultDetector struct {
	SupportedLanguages []string
	FallbackLanguage   string
	QueryParam         string // default: "lang"
	CookieName         string // default: "language"
}

// NewDefaultDetector creates a new DefaultDetector with sensible defaults.
func NewDefaultDetector(supportedLangs []string, fallbackLang string) *DefaultDetector {
	return &DefaultDetector{
		SupportedLanguages: supportedLangs,
		FallbackLanguage:   fallbackLang,
		QueryParam:         "lang",
		CookieName:         "language",
	}
}

// Detect detects the language from the request.
func (d *DefaultDetector) Detect(r *http.Request) string {
	// 1. Check query parameter
	queryParam := d.QueryParam
	if queryParam == "" {
		queryParam = "lang"
	}
	if lang := r.URL.Query().Get(queryParam); lang != "" {
		if d.isSupported(lang) {
			return lang
		}
	}

	// 2. Check cookie
	cookieName := d.CookieName
	if cookieName == "" {
		cookieName = "language"
	}
	if cookie, err := r.Cookie(cookieName); err == nil {
		if d.isSupported(cookie.Value) {
			return cookie.Value
		}
	}

	// 3. Check Accept-Language header
	acceptLangs := ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	for _, lang := range acceptLangs {
		// Try exact match
		if d.isSupported(lang) {
			return lang
		}

		// Try base language (en-US -> en)
		if base := strings.Split(lang, "-")[0]; d.isSupported(base) {
			return base
		}
	}

	// 4. Return fallback
	return d.FallbackLanguage
}

func (d *DefaultDetector) isSupported(lang string) bool {
	for _, supported := range d.SupportedLanguages {
		if supported == lang {
			return true
		}
	}
	return false
}

// Middleware creates an HTTP middleware that detects language and adds it to context.
func Middleware(mgr Manager) func(http.Handler) http.Handler {
	detector := NewDefaultDetector(mgr.SupportedLanguages(), mgr.DefaultLanguage())

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang := detector.Detect(r)
			localizer := mgr.GetLocalizer(lang)

			ctx := context.WithValue(r.Context(), ContextKeyLanguage, lang)
			ctx = context.WithValue(ctx, ContextKeyLocalizer, localizer)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MiddlewareWithDetector creates an HTTP middleware with a custom detector.
func MiddlewareWithDetector(mgr Manager, detector LanguageDetector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang := detector.Detect(r)
			localizer := mgr.GetLocalizer(lang)

			ctx := context.WithValue(r.Context(), ContextKeyLanguage, lang)
			ctx = context.WithValue(ctx, ContextKeyLocalizer, localizer)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
