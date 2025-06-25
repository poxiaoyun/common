package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
	"xiaoshiai.cn/common/errors"
)

type CookieSessionManagerOptions struct {
	MaxAge     int    // seconds, 0 means no expiration
	Secret     []byte // secret used to sign the session cookie
	HashKey    []byte // hash key for securecookie
	CookieName string // name of the session cookie
}

type CookieSessionManager struct {
	cookieName string
	codec      securecookie.Codec
}

func NewCookieSessionManager(options CookieSessionManagerOptions) *CookieSessionManager {
	if len(options.HashKey) == 0 {
		options.HashKey = securecookie.GenerateRandomKey(32) // default to 32 bytes
	}
	if len(options.Secret) == 0 {
		options.Secret = securecookie.GenerateRandomKey(32) // default to 32 bytes
	}
	if options.CookieName == "" {
		options.CookieName = "_cookie_session" // default session cookie name
	}
	return &CookieSessionManager{
		cookieName: options.CookieName,
		codec:      securecookie.New(options.HashKey, options.Secret).MaxAge(options.MaxAge),
	}
}

// GetSession implements SessionManager.
func (c *CookieSessionManager) GetSession(r *http.Request, w http.ResponseWriter, options OpenSessionOptions) (Session, error) {
	cookie, _ := r.Cookie(c.cookieName)
	var values map[string]string
	if cookie != nil {
		if err := c.codec.Decode(c.cookieName, cookie.Value, &values); err != nil {
			return nil, errors.NewInternalError(err)
		}
	}
	session := &CookieSession{
		r:          r,
		w:          w,
		codec:      c.codec,
		cookieName: c.cookieName,
		values:     values,
	}
	if options.Expires > 0 {
		session.expiresAt = time.Now().Add(time.Duration(options.Expires) * time.Second)
	}
	return session, nil
}

// ListSessions implements SessionManager.
func (c *CookieSessionManager) ListSessions(ctx context.Context) ([]Session, error) {
	return nil, errors.NewUnsupported("listing sessions is not supported for CookieSessionManager")
}

// RemoveSession implements SessionManager.
func (c *CookieSessionManager) RemoveSession(r *http.Request, w http.ResponseWriter) error {
	UnsetCookie(w, c.cookieName) // remove it from the response header
	return nil
}

type CookieSession struct {
	r *http.Request
	w http.ResponseWriter

	codec      securecookie.Codec
	cookieName string
	expiresAt  time.Time
	values     map[string]string
}

func (c *CookieSession) Identity() string {
	return "fake-identity"
}

func (c *CookieSession) ExpiresAt() time.Time {
	return c.expiresAt
}

func (c *CookieSession) Get(ctx context.Context, key string) (string, error) {
	val, ok := c.values[key]
	if !ok {
		return "", ErrSessionValueNotFound
	}
	return val, nil
}

func (c *CookieSession) Set(ctx context.Context, key string, value string) error {
	if c.values == nil {
		c.values = make(map[string]string)
	}
	c.values[key] = value
	c.sync()
	return nil
}

func (c *CookieSession) Remove(ctx context.Context, key string) error {
	delete(c.values, key)
	c.sync()
	return nil
}

func (c *CookieSession) Clear(ctx context.Context) error {
	clear(c.values)
	c.sync()
	return nil
}

func (c *CookieSession) sync() {
	if c.w == nil || c.r == nil {
		return
	}
	encoded, err := c.codec.Encode(c.cookieName, c.values)
	if err != nil {
		return // handle error appropriately
	}
	SetCookie(c.w, c.cookieName, encoded, c.expiresAt)
}

func (c *CookieSession) Values(ctx context.Context) (map[string]string, error) {
	return c.values, nil
}
