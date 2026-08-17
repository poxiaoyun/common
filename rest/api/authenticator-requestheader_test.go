package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRequestHeaderCodecRoundTrip(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := AuthenticationInfo{
		Subject: Subject{
			ID:            "user-42",
			Name:          "alice",
			Email:         "alice@example.com",
			EmailVerified: true,
			Groups:        []string{"developers", "admins"},
		},
		Actor:  &Subject{ID: "orders-api", Name: "Orders API", Groups: []string{"services"}},
		Access: &AccessConstraints{Audiences: []string{"api"}, Scopes: []string{"orders.read"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := codec.Encode(req, want); err != nil {
		t.Fatal(err)
	}
	got, err := codec.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("Decode() = %#v, want %#v", got, &want)
	}
}

func TestRequestHeaderCodecCustomHeaderAndCleanup(t *testing.T) {
	codec, err := NewRequestHeaderCodec(&RequestHeaderAuthenticatorOptions{Header: "X-Authentication"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header["x-authentication"] = []string{"forged"}
	req.Header.Set("X-Unrelated", "keep")
	if err := codec.Encode(req, AuthenticationInfo{Subject: Subject{ID: "alice"}}); err != nil {
		t.Fatal(err)
	}
	if values := codec.headerValues(req.Header); len(values) != 1 || values[0] == "forged" {
		t.Fatalf("authentication header = %#v", values)
	}
	if got := req.Header.Get("X-Unrelated"); got != "keep" {
		t.Fatalf("unrelated header = %q", got)
	}
}

func TestRequestHeaderCodecValidatesBeforeMutation(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Remote-Authentication", "existing")
	if err := codec.Encode(req, AuthenticationInfo{}); err == nil {
		t.Fatal("Encode() accepted an empty subject ID")
	}
	if got := req.Header.Get("X-Remote-Authentication"); got != "existing" {
		t.Fatalf("request was mutated on validation failure: %q", got)
	}
}

func TestRequestHeaderCodecDecodeValidation(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	unknown := base64.RawURLEncoding.EncodeToString([]byte(`{"id":"alice","unknown":true}`))
	tests := []struct {
		name        string
		header      http.Header
		notProvided bool
	}{
		{name: "absent", header: http.Header{}, notProvided: true},
		{name: "duplicate", header: http.Header{"X-Remote-Authentication": {"one", "two"}}},
		{name: "invalid base64", header: http.Header{"X-Remote-Authentication": {"%"}}},
		{name: "unknown field", header: http.Header{"X-Remote-Authentication": {unknown}}},
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

func TestRequestHeaderAuthenticatorTrustVerifier(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := codec.Encode(req, AuthenticationInfo{Subject: Subject{ID: "alice"}}); err != nil {
		t.Fatal(err)
	}
	info, err := authenticator.Authenticate(httptest.NewRecorder(), req)
	if err != nil || info.ID != "alice" || calls != 1 {
		t.Fatalf("Authenticate() = %#v, %v; calls = %d", info, err, calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestAuthenticationProxyRoundTrippers(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	var received *http.Request
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	rt, err := NewAuthenticationProxyRoundTripper(codec, AuthenticationInfo{Subject: Subject{ID: "alice"}}, transport)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Header.Set("X-Remote-Authentication", "forged")
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if received == req || req.Header.Get("X-Remote-Authentication") != "forged" {
		t.Fatal("RoundTrip mutated or forwarded the original request")
	}
	decoded, err := codec.Decode(received)
	if err != nil || decoded.ID != "alice" {
		t.Fatalf("fixed authentication = %#v, %v", decoded, err)
	}

	contextRT, err := NewContextAuthenticationProxyRoundTripper(codec, transport)
	if err != nil {
		t.Fatal(err)
	}
	contextReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	contextReq = contextReq.WithContext(WithAuthentication(contextReq.Context(), AuthenticationInfo{Subject: Subject{ID: "context-subject"}}))
	if _, err := contextRT.RoundTrip(contextReq); err != nil {
		t.Fatal(err)
	}
	decoded, err = codec.Decode(received)
	if err != nil || decoded.ID != "context-subject" {
		t.Fatalf("context authentication = %#v, %v", decoded, err)
	}
}
