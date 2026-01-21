package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"xiaoshiai.cn/common/i18n"
)

func main() {
	// Initialize i18n manager
	manager := i18n.NewManager()

	// Load translations from files
	err := manager.LoadTranslations("./examples/locales", i18n.FormatJSON)
	if err != nil {
		log.Fatal(err)
	}

	// Set fallback language
	manager.SetFallbackLanguage("en")

	fmt.Println("=== Basic Usage Examples ===\n")
	basicUsageExamples(manager)

	fmt.Println("\n=== Plural Examples ===\n")
	pluralExamples(manager)

	fmt.Println("\n=== HTTP Server Example ===")
	httpServerExample(manager)
}

func basicUsageExamples(manager i18n.Manager) {
	// English
	enLoc := manager.GetLocalizer("en")
	fmt.Println("English:")
	fmt.Println("  Simple:", enLoc.T("common.hello"))
	fmt.Println("  Template:", enLoc.Tf("common.welcome", map[string]any{"name": "John"}))
	fmt.Println("  Profile:", enLoc.Tf("user.profile.age", map[string]any{"age": 25}))

	// Chinese
	zhLoc := manager.GetLocalizer("zh-CN")
	fmt.Println("\nChinese:")
	fmt.Println("  Simple:", zhLoc.T("common.hello"))
	fmt.Println("  Template:", zhLoc.Tf("common.welcome", map[string]any{"name": "张三"}))
	fmt.Println("  Profile:", zhLoc.Tf("user.profile.age", map[string]any{"age": 25}))

	// Japanese
	jaLoc := manager.GetLocalizer("ja")
	fmt.Println("\nJapanese:")
	fmt.Println("  Simple:", jaLoc.T("common.hello"))
	fmt.Println("  Template:", jaLoc.Tf("common.welcome", map[string]any{"name": "太郎"}))
	fmt.Println("  Profile:", jaLoc.Tf("user.profile.age", map[string]any{"age": 25}))

	// Error messages
	fmt.Println("\nError Messages:")
	fmt.Println("  EN:", enLoc.E("errors.not_found"))
	fmt.Println("  ZH:", zhLoc.E("errors.not_found"))
	fmt.Println("  JA:", jaLoc.E("errors.not_found"))
}

func pluralExamples(manager i18n.Manager) {
	enLoc := manager.GetLocalizer("en")
	zhLoc := manager.GetLocalizer("zh-CN")

	fmt.Println("English Plurals:")
	fmt.Println("  1 item:", enLoc.P("items", 1, 1))
	fmt.Println("  5 items:", enLoc.P("items", 5, 5))

	fmt.Println("\nChinese Plurals (no plural forms):")
	fmt.Println("  1 item:", zhLoc.P("items", 1, 1))
	fmt.Println("  5 items:", zhLoc.P("items", 5, 5))
}

func httpServerExample(manager i18n.Manager) {
	mux := http.NewServeMux()

	// Example endpoint that uses i18n
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// Get localizer from context
		loc := manager.GetLocalizerFromContext(r.Context())

		// Translate messages
		greeting := loc.T("common.hello")
		welcome := loc.Tf("common.welcome", map[string]any{"name": "User"})

		response := fmt.Sprintf("%s\n%s\nLanguage: %s\n", greeting, welcome, loc.Language())
		w.Write([]byte(response))
	})

	mux.HandleFunc("/user/profile", func(w http.ResponseWriter, r *http.Request) {
		loc := manager.GetLocalizerFromContext(r.Context())

		title := loc.T("user.profile.title")
		age := loc.Tf("user.profile.age", map[string]any{"age": 30})
		email := loc.Tf("user.profile.email", map[string]any{"email": "user@example.com"})

		response := fmt.Sprintf("%s\n%s\n%s\n", title, age, email)
		w.Write([]byte(response))
	})

	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		// Return localized error
		err := i18n.E(r.Context(), "errors.not_found")
		http.Error(w, err.Error(), http.StatusNotFound)
	})

	// Apply i18n middleware
	handler := i18n.Middleware(manager)(mux)

	fmt.Println("Starting HTTP server on :8080")
	fmt.Println("\nTry these URLs:")
	fmt.Println("  http://localhost:8080/hello")
	fmt.Println("  http://localhost:8080/hello?lang=zh-CN")
	fmt.Println("  http://localhost:8080/hello?lang=ja")
	fmt.Println("  http://localhost:8080/user/profile")
	fmt.Println("  http://localhost:8080/error")
	fmt.Println("\nPress Ctrl+C to stop")

	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}

// Example of using i18n in a service layer
type UserService struct {
	manager i18n.Manager
}

func (s *UserService) CreateUser(ctx context.Context, username string) error {
	// Simulate user creation
	// ...

	// Return localized success message
	message := i18n.T(ctx, "user.created")
	fmt.Println(message)

	return nil
}

func (s *UserService) GetUserProfile(ctx context.Context, userID string) error {
	// Simulate user lookup
	found := false

	if !found {
		// Return localized error
		return i18n.E(ctx, "errors.not_found")
	}

	return nil
}
