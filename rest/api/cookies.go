package api

import (
	"net/http"
	"time"
)

func UnsetCookie(w http.ResponseWriter, name string) {
	cookie := &http.Cookie{Name: name, MaxAge: -1}
	w.Header().Add("Set-Cookie", cookie.String())
}

func SetCookie(w http.ResponseWriter, name, value string, expires time.Time) {
	SetCookieHeader(w.Header(), name, value, expires)
}

func SetCookieHeader(header http.Header, name, value string, expires time.Time) {
	cookie := &http.Cookie{Name: name, Path: "/", Value: value, Expires: expires}
	if cookiestr := cookie.String(); cookiestr != "" {
		header.Add("Set-Cookie", cookiestr)
	}
}

func GetCookie(r *http.Request, name string) string {
	cookie, _ := r.Cookie(name)
	if cookie == nil || cookie.Value == "" {
		return ""
	}
	return cookie.Value
}
