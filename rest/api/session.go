package api

import (
	"net/http"
	"time"

	"xiaoshiai.cn/common/rand"
)

var _ Filter = SessionFilter{}

func NewSessionFilter(sessionKey string) SessionFilter {
	newfunc := func() string {
		return rand.RandomHex(32)
	}
	return SessionFilter{SessionKey: sessionKey, NewSession: newfunc}
}

type SessionFilter struct {
	SessionKey string
	NewSession func() string
}

func (d SessionFilter) Process(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if val := GetCookie(r, d.SessionKey); val == "" {
		SetCookie(w, d.SessionKey, d.NewSession(), time.Time{})
	}
}
