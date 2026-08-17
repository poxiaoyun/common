package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestBeginAuthorizationCodeFlowGeneratesStateWhenOmitted(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(response, request)
			return
		}
		WriteProviderMetadata(t, response, server.URL)
	}))
	defer server.Close()

	client := NewTestClient(t, server, ClientOptions{
		Authentication: ClientAuthentication{ClientID: "client"},
	})
	authorization, err := client.BeginAuthorizationCodeFlow(context.Background(), AuthorizationCodeFlowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if authorization.State == "" {
		t.Fatal("state was not generated")
	}
}

func TestCompleteAuthorizationCodeFlowReturnsProtocolErrors(t *testing.T) {
	client := &Client{}
	_, err := client.CompleteAuthorizationCodeFlow(context.Background(), url.Values{
		"error":             {"access_denied"},
		"error_description": {"user declined"},
		"state":             {"expected"},
	}, AuthorizationCodeFlow{State: "expected"})
	var endpoint *EndpointError
	if !errors.As(err, &endpoint) || endpoint.Code != "access_denied" {
		t.Fatalf("error = %#v", err)
	}
	_, err = client.CompleteAuthorizationCodeFlow(context.Background(), url.Values{
		"error": {"access_denied"},
		"state": {"other"},
	}, AuthorizationCodeFlow{State: "expected"})
	if !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("error = %v", err)
	}
}
