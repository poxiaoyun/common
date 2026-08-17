package oidc

import (
	"context"
	"fmt"
	"net/url"

	"golang.org/x/oauth2"
)

// AuthorizationCodeFlow contains the values binding an Authorization Code
// request to its callback. The caller must persist it until
// CompleteAuthorizationCodeFlow runs.
type AuthorizationCodeFlow struct {
	URL          string `json:"url"`
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"codeVerifier"`
}

// AuthorizationCodeFlowOptions configures one Authorization Code request. Its
// zero value lets the client generate all request-binding values.
type AuthorizationCodeFlowOptions struct {
	State string
}

// BeginAuthorizationCodeFlow creates an Authorization Code request protected
// by state, nonce, and PKCE. When State is empty, a random value is generated.
func (c *Client) BeginAuthorizationCodeFlow(ctx context.Context, options AuthorizationCodeFlowOptions) (AuthorizationCodeFlow, error) {
	_, configuration, err := c.authorizationCodeConfiguration(ctx)
	if err != nil {
		return AuthorizationCodeFlow{}, err
	}
	state := options.State
	if state == "" {
		state = oauth2.GenerateVerifier()
	}
	nonce := oauth2.GenerateVerifier()
	verifier := oauth2.GenerateVerifier()
	authorizationURL := configuration.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	return AuthorizationCodeFlow{
		URL:          authorizationURL,
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
	}, nil
}

// CompleteAuthorizationCodeFlow validates the callback, exchanges its code,
// and verifies the returned ID Token against the original request.
func (c *Client) CompleteAuthorizationCodeFlow(ctx context.Context, callback url.Values, expected AuthorizationCodeFlow) (*TokenSet, error) {
	if callback.Get("state") != expected.State {
		return nil, ErrStateMismatch
	}
	if code := callback.Get("error"); code != "" {
		return nil, &EndpointError{
			Code:        code,
			Description: callback.Get("error_description"),
			URI:         callback.Get("error_uri"),
		}
	}
	code := callback.Get("code")
	if code == "" {
		return nil, fmt.Errorf("oidc: authorization response is missing code")
	}
	current, configuration, err := c.authorizationCodeConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	verifier, err := current.GetIDTokenVerifier()
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, current.HTTPClient)
	token, err := configuration.Exchange(ctx, code, oauth2.VerifierOption(expected.CodeVerifier))
	if err != nil {
		return nil, EndpointTokenError(err)
	}
	set, err := TokenSetFromOAuth2(token)
	if err != nil {
		return nil, err
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if rawIDToken == "" {
		return nil, fmt.Errorf("oidc: token response is missing ID Token")
	}
	idToken, err := verifier.Verify(ctx, rawIDToken, IDTokenChecks{
		Nonce:       expected.Nonce,
		AccessToken: set.AccessToken,
	})
	if err != nil {
		return nil, err
	}
	set.IDToken = idToken
	return set, nil
}

func (c *Client) authorizationCodeConfiguration(ctx context.Context) (*ClientConfiguration, oauth2.Config, error) {
	configuration, err := c.getClientConfiguration(ctx)
	if err != nil {
		return nil, oauth2.Config{}, err
	}
	authorization, err := configuration.GetAuthorizationCodeConfiguration()
	if err != nil {
		return nil, oauth2.Config{}, err
	}
	return configuration, authorization, nil
}
