package openapi_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"xiaoshiai.cn/common/openapi"
)

func TestConfigureAuthenticationSecurity(t *testing.T) {
	document := openapi.NewAPIDocPlugin().OpenAPI
	openapi.ConfigureAuthenticationSecurity(document, openapi.AuthenticationSecurityOptions{
		OpenIDConnect: openapi.OpenIDConnectSecurityOptions{Issuer: "https://identity.example.com/tenant/"},
		OAuth2: openapi.OAuth2SecurityOptions{
			AuthorizationCode: openapi.OAuth2AuthorizationCodeFlowOptions{
				AuthorizationEndpoint: "https://identity.example.com/oauth2/authorize",
				TokenEndpoint:         "https://identity.example.com/oauth2/token",
				RefreshEndpoint:       "https://identity.example.com/oauth2/refresh",
				Scopes:                []string{"read:applications", "write:applications"},
			},
			ClientCredentials: openapi.OAuth2ClientCredentialsFlowOptions{
				TokenEndpoint: "https://identity.example.com/oauth2/token",
				Scopes:        []string{"read:applications"},
			},
		},
		Bearer:        true,
		Basic:         true,
		SessionCookie: "user_session",
		ProxyHeader:   openapi.ProxyHeaderSecurityOptions{Header: "X-Remote-Authentication"},
		Anonymous:     true,
	})

	require.NoError(t, document.Validate(context.Background()))
	require.Equal(t, "openIdConnect", document.Components.SecuritySchemes["OpenIDConnect"].Value.Type)
	require.Equal(t, "https://identity.example.com/tenant/.well-known/openid-configuration", document.Components.SecuritySchemes["OpenIDConnect"].Value.OpenIdConnectUrl)
	oauth2 := document.Components.SecuritySchemes["OAuth2"].Value
	require.Equal(t, "oauth2", oauth2.Type)
	require.Equal(t, "https://identity.example.com/oauth2/authorize", oauth2.Flows.AuthorizationCode.AuthorizationURL)
	require.Equal(t, "https://identity.example.com/oauth2/refresh", oauth2.Flows.AuthorizationCode.RefreshURL)
	require.Contains(t, oauth2.Flows.AuthorizationCode.Scopes, "write:applications")
	require.Equal(t, "https://identity.example.com/oauth2/token", oauth2.Flows.ClientCredentials.TokenURL)
	require.Equal(t, "bearer", document.Components.SecuritySchemes["Bearer"].Value.Scheme)
	require.Equal(t, "basic", document.Components.SecuritySchemes["Basic"].Value.Scheme)
	require.Equal(t, "cookie", document.Components.SecuritySchemes["SessionCookie"].Value.In)
	require.Equal(t, "user_session", document.Components.SecuritySchemes["SessionCookie"].Value.Name)
	require.Equal(t, "X-Remote-Authentication", document.Components.SecuritySchemes["ProxyHeader"].Value.Name)
	require.Len(t, document.Security, 7)
	require.Contains(t, document.Security[0], "OpenIDConnect")
	require.Contains(t, document.Security[1], "OAuth2")
	require.Contains(t, document.Security[2], "Bearer")
	require.Contains(t, document.Security[3], "Basic")
	require.Contains(t, document.Security[4], "SessionCookie")
	require.Contains(t, document.Security[5], "ProxyHeader")
	require.Empty(t, document.Security[6])
}

func TestConfigureAuthenticationSecurityOmitsZeroValueMechanisms(t *testing.T) {
	document := openapi.NewAPIDocPlugin().OpenAPI
	openapi.ConfigureAuthenticationSecurity(document, openapi.AuthenticationSecurityOptions{})

	require.Empty(t, document.Components.SecuritySchemes)
	require.Empty(t, document.Security)
}

func TestConfigureAuthenticationSecurityOmitsIncompleteOAuth2Flows(t *testing.T) {
	document := openapi.NewAPIDocPlugin().OpenAPI
	openapi.ConfigureAuthenticationSecurity(document, openapi.AuthenticationSecurityOptions{
		OAuth2: openapi.OAuth2SecurityOptions{
			AuthorizationCode: openapi.OAuth2AuthorizationCodeFlowOptions{
				RefreshEndpoint: "https://identity.example.com/oauth2/refresh",
				Scopes:          []string{"read:applications"},
			},
			ClientCredentials: openapi.OAuth2ClientCredentialsFlowOptions{
				Scopes: []string{"read:applications"},
			},
		},
	})

	require.NoError(t, document.Validate(context.Background()))
	require.NotContains(t, document.Components.SecuritySchemes, "OAuth2")
	require.Empty(t, document.Security)
}

func TestOpenAPIPluginConstructionOptions(t *testing.T) {
	configured := false
	plugin := openapi.NewAPIDocPlugin().
		WithPath("/docs").
		ConfigureDocument(func(document *openapi.Document) {
			configured = true
			document.Info.Title = "Public API"
		})

	require.Equal(t, "/docs", plugin.Path)
	require.True(t, configured)
	require.Equal(t, "Public API", plugin.OpenAPI.Info.Title)
}
