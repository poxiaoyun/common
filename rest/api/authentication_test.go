package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
	"xiaoshiai.cn/common/authn"
	"xiaoshiai.cn/common/rest/api"
)

func TestHTTPAuthenticatorReceivesHTTPObjects(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	authenticator := api.HTTPAuthenticatorFunc(func(gotResponse http.ResponseWriter, gotRequest *http.Request) (*authn.Authentication, error) {
		if gotResponse != response || gotRequest != request {
			t.Fatal("AuthenticateHTTP() did not receive the supplied HTTP objects")
		}
		return &authn.Authentication{Subject: authn.Subject{ID: "subject-1"}}, nil
	})

	if _, err := authenticator.AuthenticateHTTP(response, request); err != nil {
		t.Fatal(err)
	}
}

type recordingSSHAuthenticator struct {
	username string
}

func (authenticator *recordingSSHAuthenticator) AuthenticateBasic(context.Context, string, string) (*authn.Authentication, error) {
	return nil, api.ErrNotProvided
}

func (authenticator *recordingSSHAuthenticator) AuthenticateSSHPublicKey(_ context.Context, username string, _ ssh.PublicKey) (*authn.Authentication, error) {
	authenticator.username = username
	return &authn.Authentication{Subject: authn.Subject{ID: "subject-1"}}, nil
}

func TestSSHAuthenticatorChainForwardsPublicKeyUsername(t *testing.T) {
	authenticator := &recordingSSHAuthenticator{}
	if _, err := (api.SSHAuthenticatorChain{authenticator}).AuthenticateSSHPublicKey(t.Context(), "alice", nil); err != nil {
		t.Fatal(err)
	}
	if authenticator.username != "alice" {
		t.Fatalf("username = %q, want alice", authenticator.username)
	}
}

func TestAuthenticationAudienceContext(t *testing.T) {
	want := []string{"urn:apps:api", "urn:cloud:api"}
	ctx := api.WithAuthenticationAudiences(context.Background(), want)
	if got := api.AuthenticationAudiencesFromContext(ctx); !reflect.DeepEqual(got, want) {
		t.Fatalf("AuthenticationAudiencesFromContext() = %#v, want %#v", got, want)
	}
}
