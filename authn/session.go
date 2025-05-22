package authn

import (
	"context"
	"log"
	"net/http"

	"xiaoshiai.cn/common/rest/api"
)

const SessionCookieKey = "user_session"

var _ api.Filter = SessionFilter{}

func NewSessionFilter(provider AuthProvider) SessionFilter {
	return SessionFilter{Provider: provider}
}

type SessionFilter struct {
	Provider AuthProvider
}

// Process implements api.Filter.
func (s SessionFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	session := api.GetCookie(r, SessionCookieKey)
	newsession, err := s.Provider.CheckSession(r.Context(), session)
	if err != nil {
		log.Printf("check session error: %v", err)
		next.ServeHTTP(w, r)
		return
	}
	// refresh the session
	if newsession != nil {
		api.SetCookie(w, SessionCookieKey, newsession.Value, newsession.Expires)
	}
	next.ServeHTTP(w, r)
}

func (a *API) OnSession(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, session string) (any, error)) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		key := api.GetCookie(r, SessionCookieKey)
		if key == "" {
			key = api.ExtracBearerTokenFromRequest(r)
		}
		return fn(ctx, key)
	})
}
