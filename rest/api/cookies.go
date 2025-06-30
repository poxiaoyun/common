package api

import (
	"net/http"
	"slices"
	"time"
)

func UnsetCookie(w http.ResponseWriter, name string) {
	cookie := &http.Cookie{Name: name, MaxAge: -1}
	w.Header().Add("Set-Cookie", cookie.String())
}

func UnsetCookieWithDomain(w http.ResponseWriter, name, domain string, insecure bool) {
	cookie := &http.Cookie{
		Name:     name,
		MaxAge:   -1,        // delete the cookie
		HttpOnly: true,      // make it HttpOnly for security
		Secure:   !insecure, // use Secure flag if served over HTTPS
		Domain:   domain,
	}
	SetCookieHeader(w.Header(), cookie)
}

func SetCookie(w http.ResponseWriter, name, value string, expires time.Time) {
	SetCookieHeader(w.Header(), &http.Cookie{Name: name, Value: value, Expires: expires})
}

func SetCookieWithDomain(w http.ResponseWriter, name, value string, expires time.Time, domain string, insecure bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Expires:  expires,
		HttpOnly: true,      // make it HttpOnly for security
		Secure:   !insecure, // use Secure flag if served over HTTPS
		Domain:   domain,
	}
	SetCookieHeader(w.Header(), cookie)
}

func GetCookie(r *http.Request, name string) string {
	cookie, _ := r.Cookie(name)
	if cookie == nil || cookie.Value == "" {
		return ""
	}
	return cookie.Value
}

// SetCookieHeader sets or replaces a set-cookie item in the response header.
// It will overwrite any existing set-cookie with the same name.
// This is useful for updating set-cookie values without creating duplicates.
func SetCookieHeader(header http.Header, cookie *http.Cookie) {
	if cookie == nil {
		return
	}
	setCookies := ReadSetCookies(header)
	idx := slices.IndexFunc(setCookies, func(c *http.Cookie) bool {
		return c.Name == cookie.Name
	})
	if idx != -1 {
		// Replace existing cookie
		setCookies[idx] = cookie
	} else {
		// Add new cookie
		setCookies = append(setCookies, cookie)
	}
	header["Set-Cookie"] = nil // clear existing Set-Cookie headers
	for _, cookie := range setCookies {
		// see [http.SetCookie] for details on how to format cookies
		if v := cookie.String(); v != "" {
			header.Add("Set-Cookie", v)
		}
	}
}

func ReadSetCookies(h http.Header) []*http.Cookie {
	setCookie := h["Set-Cookie"]
	cookies := make([]*http.Cookie, 0, len(setCookie))
	for _, line := range setCookie {
		if cookie, err := http.ParseSetCookie(line); err == nil {
			cookies = append(cookies, cookie)
		}
	}
	return cookies
}
