package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"time"

	"xiaoshiai.cn/common/errors"
)

var _ Filter = SessionFilter{}

type OpenSessionOptions struct {
	Expires int64 // seconds, 0 means no expiration
}

type SessionManager interface {
	GetSession(r *http.Request, w http.ResponseWriter, options OpenSessionOptions) (Session, error)
	RemoveSession(r *http.Request, w http.ResponseWriter) error
	ListSessions(ctx context.Context) ([]Session, error)
}

var ErrSessionValueNotFound = stderrors.New("session value not found")

type Session interface {
	Identity() string
	ExpiresAt() time.Time

	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	Values(ctx context.Context) (map[string]string, error)
	Remove(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

type sessionContextKey struct{}

func SessionFromContext(ctx context.Context) (Session, bool) {
	session := GetContextValue[Session](ctx, "session")
	if session == nil {
		return nil, false
	}
	return session, true
}

func WithSession(ctx context.Context, session Session) context.Context {
	return SetContextValue(ctx, "session", session)
}

type SessionFilter struct {
	SessionManager SessionManager
}

func (f SessionFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	var session Session
	session, err := f.SessionManager.GetSession(r, w, OpenSessionOptions{})
	if err != nil {
		Error(w, errors.NewInternalError(fmt.Errorf("failed to open session")))
		return
	}
	ctx := WithSession(r.Context(), session)
	next.ServeHTTP(w, r.WithContext(ctx))
}
