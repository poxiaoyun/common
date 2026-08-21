package api

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRequestHeaderCodecRoundTrip(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	want := AuthenticationInfo{
		Subject: Subject{
			ID:            "user-42",
			Name:          "alice",
			DisplayName:   "Alice Example",
			Email:         "alice@example.com",
			EmailVerified: true,
			Groups:        []string{"developers", "admins"},
		},
		Actor: &Subject{
			ID:            "orders-api",
			Name:          "orders",
			DisplayName:   "Orders API",
			Email:         "orders@example.com",
			EmailVerified: true,
			Groups:        []string{"services"},
		},
		Access: &AccessConstraints{
			Audiences: []string{"api", "metrics"},
			Scopes:    []string{"orders.read", "orders.write"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	codec.Encode(req, want)
	got, err := codec.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("Decode() = %#v, want %#v", got, &want)
	}
}

func TestRequestHeaderCodecDefaultProtocolHeaders(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	codec.Encode(req, AuthenticationInfo{
		Subject: Subject{ID: "user-42", Name: "alice", Groups: []string{"developers"}},
		Actor:   &Subject{ID: "client-7"},
		Access:  &AccessConstraints{Audiences: []string{"api"}, Scopes: []string{"read"}},
	})
	want := http.Header{
		"X-Remote-User":           {"alice"},
		"X-Remote-Uid":            {"user-42"},
		"X-Remote-Group":          {"developers"},
		"X-Remote-Extra-Actor":    {"client-7"},
		"X-Remote-Extra-Access":   {"oauth2"},
		"X-Remote-Extra-Audience": {"api"},
		"X-Remote-Extra-Scopes":   {"read"},
	}
	if !reflect.DeepEqual(req.Header, want) {
		t.Fatalf("headers = %#v, want %#v", req.Header, want)
	}
}

func TestRequestHeaderCodecCustomHeadersAndCleanup(t *testing.T) {
	codec := NewRequestHeaderCodec(&RequestHeaderAuthenticatorOptions{
		NameHeader:   "X-Authentication-Name",
		UserIDHeader: "X-Authentication-ID",
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Authentication-Name", "forged")
	req.Header.Set("X-Unrelated", "keep")
	codec.Encode(req, AuthenticationInfo{Subject: Subject{ID: "user-42", Name: "alice"}})
	if got := req.Header.Values("X-Authentication-Name"); !reflect.DeepEqual(got, []string{"alice"}) {
		t.Fatalf("name header = %#v", got)
	}
	if got := req.Header.Get("X-Unrelated"); got != "keep" {
		t.Fatalf("unrelated header = %q", got)
	}
}

func TestRequestHeaderCodecDoesNotValidateAuthenticationData(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	want := AuthenticationInfo{Subject: Subject{Name: "line one\nline two"}}
	codec.Encode(req, want)
	got, err := codec.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("Decode() = %#v, want %#v", got, &want)
	}
}

func TestRequestHeaderCodecDecode(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	tests := []struct {
		name        string
		header      http.Header
		notProvided bool
	}{
		{name: "absent", header: http.Header{}, notProvided: true},
		{name: "duplicate scalar", header: http.Header{"X-Remote-User": {"alice", "bob"}}},
		{name: "invalid boolean", header: http.Header{"X-Remote-Extra-Email-Verified": {"certainly"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header = tt.header
			_, err := codec.Decode(req)
			if tt.notProvided {
				if !errors.Is(err, ErrNotProvided) {
					t.Fatalf("Decode() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Decode() unexpectedly succeeded")
			}
		})
	}
}

func TestRequestHeaderCodecPreservesNilAndEmptyAccess(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	for _, access := range []*AccessConstraints{nil, {}} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		codec.Encode(req, AuthenticationInfo{Subject: Subject{ID: "alice"}, Access: access})
		got, err := codec.Decode(req)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Access, access) {
			t.Fatalf("Decode().Access = %#v, want %#v", got.Access, access)
		}
	}
}

func TestRequestHeaderAuthenticatorTrustVerifier(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	if _, err := NewRequestHeaderAuthenticator(codec, nil); err == nil {
		t.Fatal("constructor accepted a nil verifier")
	}
	calls := 0
	authenticator, err := NewRequestHeaderAuthenticator(codec, RequestHeaderTrustFunc(func(*http.Request) error {
		calls++
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Authenticate(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("verifier calls without assertion = %d", calls)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	codec.Encode(req, AuthenticationInfo{Subject: Subject{ID: "alice"}})
	info, err := authenticator.Authenticate(httptest.NewRecorder(), req)
	if err != nil || info.ID != "alice" || calls != 1 {
		t.Fatalf("Authenticate() = %#v, %v; calls = %d", info, err, calls)
	}
}

func TestCIDRRequestHeaderTrustVerifier(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:4321"
	for _, allowed := range [][]string{{"192.0.2.0/24"}, {"192.0.2.10"}, {"*"}} {
		if err := (CIDRRequestHeaderTrustVerifier{AllowedCIDRs: allowed}).VerifyRequest(req); err != nil {
			t.Fatalf("VerifyRequest(%v) error = %v", allowed, err)
		}
	}
	if err := (CIDRRequestHeaderTrustVerifier{AllowedCIDRs: []string{"198.51.100.0/24"}}).VerifyRequest(req); err == nil {
		t.Fatal("VerifyRequest() trusted an IP outside the allowlist")
	}
}

func TestTLSRequestHeaderTrustVerifier(t *testing.T) {
	verifiedClient := &x509.Certificate{Subject: pkix.Name{CommonName: "front-proxy"}}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{verifiedClient}}}
	for _, allowed := range [][]string{{"front-proxy"}, {"*"}} {
		if err := (TLSRequestHeaderTrustVerifier{AllowedNames: allowed}).VerifyRequest(request); err != nil {
			t.Fatalf("VerifyRequest(%v) error = %v", allowed, err)
		}
	}
	if err := (TLSRequestHeaderTrustVerifier{AllowedNames: []string{"other-proxy"}}).VerifyRequest(request); err == nil {
		t.Fatal("VerifyRequest() trusted a Common Name outside the allowlist")
	}
	request.TLS.VerifiedChains = nil
	if err := (TLSRequestHeaderTrustVerifier{AllowedNames: []string{"*"}}).VerifyRequest(request); err == nil {
		t.Fatal("VerifyRequest() trusted an unverified TLS client certificate")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestAuthenticationProxyRoundTrippers(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	var received *http.Request
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	rt := NewAuthenticationProxyRoundTripper(codec, AuthenticationInfo{Subject: Subject{ID: "alice"}}, transport)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Header.Set("X-Remote-User", "forged")
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if received == req || req.Header.Get("X-Remote-User") != "forged" {
		t.Fatal("RoundTrip mutated or forwarded the original request")
	}
	decoded, err := codec.Decode(received)
	if err != nil || decoded.ID != "alice" {
		t.Fatalf("fixed authentication = %#v, %v", decoded, err)
	}

	contextRT := NewContextAuthenticationProxyRoundTripper(codec, transport)
	contextReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	contextReq = contextReq.WithContext(WithAuthentication(contextReq.Context(), AuthenticationInfo{Subject: Subject{ID: "context-subject"}}))
	if _, err := contextRT.RoundTrip(contextReq); err != nil {
		t.Fatal(err)
	}
	decoded, err = codec.Decode(received)
	if err != nil || decoded.ID != "context-subject" {
		t.Fatalf("context authentication = %#v, %v", decoded, err)
	}

	sanitizingRT := NewRequestHeaderSanitizingRoundTripper(codec, transport)
	sanitizedReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	sanitizedReq.Header.Set("X-Remote-User", "forged")
	sanitizedReq.Header.Set("X-Unrelated", "keep")
	if _, err := sanitizingRT.RoundTrip(sanitizedReq); err != nil {
		t.Fatal(err)
	}
	if _, err := codec.Decode(received); !errors.Is(err, ErrNotProvided) {
		t.Fatalf("Decode() after sanitizing error = %v", err)
	}
	if got := received.Header.Get("X-Unrelated"); got != "keep" {
		t.Fatalf("unrelated header = %q", got)
	}
}

func TestAuthenticationProxyRoundTripperAuthenticateRequest(t *testing.T) {
	codec := NewRequestHeaderCodec(nil)
	rt := NewAuthenticationProxyRoundTripper(
		codec,
		AuthenticationInfo{Subject: Subject{ID: "alice", Name: "alice"}},
		http.DefaultTransport,
	)
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Header.Set("X-Remote-User", "forged")
	if err := rt.AuthenticateRequest(req); err != nil {
		t.Fatal(err)
	}
	info, err := codec.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "alice" || info.Name != "alice" {
		t.Fatalf("authentication = %#v", info)
	}
}
