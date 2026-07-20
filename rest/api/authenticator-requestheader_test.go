package api

import (
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
	want := AuthenticateInfo{
		Audiences: []string{"api", "metrics"},
		User: UserInfo{
			ID:            "user-42",
			Name:          "alice",
			Email:         "alice@example.com",
			EmailVerified: true,
			Groups:        []string{"developers", "admins"},
			Extra: map[string][]string{
				"Example.com/角色%": {"owner", "reviewer"},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := codec.Encode(req, want); err != nil {
		t.Fatal(err)
	}
	got, err := codec.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	want.User.Extra = map[string][]string{"example.com/角色%": {"owner", "reviewer"}}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("Decode() = %#v, want %#v", got, &want)
	}
}

func TestRequestHeaderCodecCustomOptionsAndCleanup(t *testing.T) {
	codec, err := NewRequestHeaderCodec(&RequestHeaderAuthenticatorOptions{
		NameHeader:        "X-Authenticated-User",
		GroupsHeader:      "X-Authenticated-Group",
		ExtraHeaderPrefix: "X-Authenticated-Extra-",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header["x-authenticated-user"] = []string{"forged"}
	req.Header["x-authenticated-extra-old"] = []string{"forged"}
	req.Header.Set("X-Unrelated", "keep")
	info := AuthenticateInfo{User: UserInfo{Name: "alice", Groups: []string{"team"}}}
	if err := codec.Encode(req, info); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("X-Authenticated-User"); got != "alice" {
		t.Fatalf("user header = %q, want alice", got)
	}
	if got := req.Header.Get("X-Authenticated-Extra-Old"); got != "" {
		t.Fatalf("stale extra header was not cleared: %q", got)
	}
	if got := req.Header.Get("X-Unrelated"); got != "keep" {
		t.Fatalf("unrelated header = %q, want keep", got)
	}
	decoded, err := codec.Decode(req)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.User.Name != "alice" || !reflect.DeepEqual(decoded.User.Groups, []string{"team"}) {
		t.Fatalf("unexpected decoded identity: %#v", decoded)
	}
}

func TestRequestHeaderCodecValidatesBeforeMutation(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Remote-User", "existing")
	err = codec.Encode(req, AuthenticateInfo{User: UserInfo{Name: "bad\nname"}})
	if err == nil {
		t.Fatal("Encode() succeeded with an invalid header value")
	}
	if got := req.Header.Get("X-Remote-User"); got != "existing" {
		t.Fatalf("request was mutated on validation failure: %q", got)
	}
}

func TestRequestHeaderCodecOptionValidation(t *testing.T) {
	_, err := NewRequestHeaderCodec(&RequestHeaderAuthenticatorOptions{GroupsHeader: "x-remote-user"})
	if err == nil {
		t.Fatal("NewRequestHeaderCodec() accepted conflicting header names")
	}
	_, err = NewRequestHeaderCodec(&RequestHeaderAuthenticatorOptions{ExtraHeaderPrefix: "bad prefix"})
	if err == nil {
		t.Fatal("NewRequestHeaderCodec() accepted an invalid extra prefix")
	}
	_, err = NewRequestHeaderCodec(&RequestHeaderAuthenticatorOptions{ExtraHeaderPrefix: "X-Remote-User-"})
	if err == nil {
		t.Fatal("NewRequestHeaderCodec() accepted a fixed header inside the extra prefix")
	}
}

func TestRequestHeaderCodecDecodeValidation(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		header      http.Header
		notProvided bool
		wantSuccess bool
	}{
		{name: "absent", header: http.Header{}, notProvided: true},
		{name: "rolling upgrade", header: http.Header{"X-Remote-User": {"alice"}}, wantSuccess: true},
		{name: "missing name", header: http.Header{"X-Remote-Group": {"team"}}},
		{name: "duplicate name", header: http.Header{"X-Remote-User": {"alice", "bob"}}},
		{name: "invalid boolean", header: http.Header{"X-Remote-User": {"alice"}, "X-Remote-User-Email-Verified": {"1"}}},
		{name: "invalid escape", header: http.Header{"X-Remote-User": {"alice"}, "X-Remote-Extra-%zz": {"value"}}},
		{name: "extra collision", header: http.Header{"X-Remote-User": {"alice"}, "X-Remote-Extra-Foo": {"one"}, "x-remote-extra-%66oo": {"two"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header = tt.header
			info, err := codec.Decode(req)
			if tt.notProvided {
				if !errors.Is(err, ErrNotProvided) {
					t.Fatalf("Decode() error = %v, want ErrNotProvided", err)
				}
				return
			}
			if tt.wantSuccess {
				if err != nil || info.User.Name != "alice" || info.User.EmailVerified {
					t.Fatalf("Decode() = %#v, %v", info, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Decode() unexpectedly succeeded: %#v", info)
			}
		})
	}
}

func TestRequestHeaderAuthenticatorTrustVerifier(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRequestHeaderAuthenticator(nil, RequestHeaderTrustFunc(func(*http.Request) error { return nil })); err == nil {
		t.Fatal("constructor accepted a nil codec")
	}
	if _, err := NewRequestHeaderAuthenticator(codec, nil); err == nil {
		t.Fatal("constructor accepted a nil verifier")
	}
	var nilVerifier RequestHeaderTrustFunc
	if _, err := NewRequestHeaderAuthenticator(codec, nilVerifier); err == nil {
		t.Fatal("constructor accepted a typed nil verifier")
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
		t.Fatalf("Authenticate() error = %v, want ErrNotProvided", err)
	}
	if calls != 0 {
		t.Fatalf("verifier called without identity headers: %d", calls)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Remote-User", "alice")
	info, err := authenticator.Authenticate(httptest.NewRecorder(), req)
	if err != nil || info.User.Name != "alice" || calls != 1 {
		t.Fatalf("Authenticate() = %#v, %v; verifier calls = %d", info, err, calls)
	}

	untrusted := errors.New("untrusted peer")
	rejecting, err := NewRequestHeaderAuthenticator(codec, RequestHeaderTrustFunc(func(*http.Request) error { return untrusted }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rejecting.Authenticate(httptest.NewRecorder(), req); !errors.Is(err, untrusted) {
		t.Fatalf("Authenticate() error = %v, want wrapped verifier error", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestAuthProxyRoundTrippers(t *testing.T) {
	codec, err := NewRequestHeaderCodec(nil)
	if err != nil {
		t.Fatal(err)
	}
	originalInfo := AuthenticateInfo{
		Audiences: []string{"api"},
		User:      UserInfo{Name: "alice", Groups: []string{"team"}, Extra: map[string][]string{"tenant": {"acme"}}},
	}
	var received *http.Request
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = req
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	rt, err := NewAuthProxyRoundTripper(codec, originalInfo, transport)
	if err != nil {
		t.Fatal(err)
	}
	// The constructor owns a deep copy of fixed identity data.
	originalInfo.User.Name = "mallory"
	originalInfo.User.Groups[0] = "other"
	originalInfo.User.Extra["tenant"][0] = "other"
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Header.Set("X-Remote-User", "forged")
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if received == req {
		t.Fatal("RoundTrip forwarded the original request instead of a clone")
	}
	if got := req.Header.Get("X-Remote-User"); got != "forged" {
		t.Fatalf("original request was mutated: %q", got)
	}
	decoded, err := codec.Decode(received)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.User.Name != "alice" || !reflect.DeepEqual(decoded.User.Groups, []string{"team"}) || decoded.User.Extra["tenant"][0] != "acme" {
		t.Fatalf("fixed identity was not copied: %#v", decoded)
	}

	contextRT, err := NewContextAuthProxyRoundTripper(codec, transport)
	if err != nil {
		t.Fatal(err)
	}
	contextInfo := AuthenticateInfo{User: UserInfo{Name: "context-user", ID: "id-1"}}
	contextReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	contextReq = contextReq.WithContext(WithAuthenticate(contextReq.Context(), contextInfo))
	if _, err := contextRT.RoundTrip(contextReq); err != nil {
		t.Fatal(err)
	}
	decoded, err = codec.Decode(received)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.User.Name != "context-user" || decoded.User.ID != "id-1" {
		t.Fatalf("context identity = %#v", decoded)
	}
}
