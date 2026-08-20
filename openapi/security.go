package openapi

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// AuthenticationSecurityOptions selects the HTTP authentication mechanisms
// represented by an OpenAPI document.
type AuthenticationSecurityOptions struct {
	OpenIDConnect OpenIDConnectSecurityOptions
	OAuth2        OAuth2SecurityOptions
	ProxyHeader   ProxyHeaderSecurityOptions
	Bearer        bool
	Basic         bool
	SessionCookie string
	Anonymous     bool
}

// OpenIDConnectSecurityOptions describes an OpenID Connect provider.
type OpenIDConnectSecurityOptions struct {
	Issuer string
}

// OAuth2SecurityOptions configures standard OAuth 2.0 flows documented by an
// API.
type OAuth2SecurityOptions struct {
	AuthorizationCode OAuth2AuthorizationCodeFlowOptions
	ClientCredentials OAuth2ClientCredentialsFlowOptions
}

// OAuth2AuthorizationCodeFlowOptions configures an OAuth 2.0 authorization
// code flow. The flow is represented only when both authorization and token
// endpoints are present.
type OAuth2AuthorizationCodeFlowOptions struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
	RefreshEndpoint       string
	Scopes                []string
}

// OAuth2ClientCredentialsFlowOptions configures an OAuth 2.0 client
// credentials flow. The flow is represented only when its token endpoint is
// present.
type OAuth2ClientCredentialsFlowOptions struct {
	TokenEndpoint string
	Scopes        []string
}

// ProxyHeaderSecurityOptions describes a trusted proxy authentication header.
type ProxyHeaderSecurityOptions struct {
	Header string
}

// ConfigureAuthenticationSecurity adds the selected authentication schemes
// and document-level alternatives to an OpenAPI document.
func ConfigureAuthenticationSecurity(document *Document, options AuthenticationSecurityOptions) {
	if document.Components.SecuritySchemes == nil {
		document.Components.SecuritySchemes = openapi3.SecuritySchemes{}
	}
	document.Security = openapi3.SecurityRequirements{}
	if options.OpenIDConnect.Issuer != "" {
		document.Components.SecuritySchemes["OpenIDConnect"] = &openapi3.SecuritySchemeRef{
			Value: openapi3.NewOIDCSecurityScheme(openIDConnectDiscoveryEndpoint(options.OpenIDConnect.Issuer)).
				WithDescription("OpenID Connect"),
		}
		document.Security = append(document.Security, authenticationRequirement("OpenIDConnect"))
	}
	if oauth2AuthorizationCodeFlowConfigured(options.OAuth2.AuthorizationCode) || oauth2ClientCredentialsFlowConfigured(options.OAuth2.ClientCredentials) {
		flows := &openapi3.OAuthFlows{}
		if oauth2AuthorizationCodeFlowConfigured(options.OAuth2.AuthorizationCode) {
			flow := options.OAuth2.AuthorizationCode
			flows.AuthorizationCode = &openapi3.OAuthFlow{
				AuthorizationURL: flow.AuthorizationEndpoint,
				TokenURL:         flow.TokenEndpoint,
				RefreshURL:       flow.RefreshEndpoint,
				Scopes:           openAPIScopes(flow.Scopes),
			}
		}
		if oauth2ClientCredentialsFlowConfigured(options.OAuth2.ClientCredentials) {
			flow := options.OAuth2.ClientCredentials
			flows.ClientCredentials = &openapi3.OAuthFlow{
				TokenURL: flow.TokenEndpoint,
				Scopes:   openAPIScopes(flow.Scopes),
			}
		}
		document.Components.SecuritySchemes["OAuth2"] = &openapi3.SecuritySchemeRef{
			Value: &openapi3.SecurityScheme{
				Type:        "oauth2",
				Description: "OAuth 2.0",
				Flows:       flows,
			},
		}
		document.Security = append(document.Security, authenticationRequirement("OAuth2"))
	}
	if options.Bearer {
		document.Components.SecuritySchemes["Bearer"] = &openapi3.SecuritySchemeRef{
			Value: openapi3.NewSecurityScheme().
				WithType("http").
				WithScheme("bearer").
				WithDescription("Bearer token"),
		}
		document.Security = append(document.Security, authenticationRequirement("Bearer"))
	}
	if options.Basic {
		document.Components.SecuritySchemes["Basic"] = &openapi3.SecuritySchemeRef{
			Value: openapi3.NewSecurityScheme().
				WithType("http").
				WithScheme("basic").
				WithDescription("HTTP Basic credentials"),
		}
		document.Security = append(document.Security, authenticationRequirement("Basic"))
	}
	if options.SessionCookie != "" {
		document.Components.SecuritySchemes["SessionCookie"] = &openapi3.SecuritySchemeRef{
			Value: openapi3.NewSecurityScheme().
				WithType("apiKey").
				WithIn("cookie").
				WithName(options.SessionCookie).
				WithDescription("Authentication session cookie"),
		}
		document.Security = append(document.Security, authenticationRequirement("SessionCookie"))
	}
	if options.ProxyHeader.Header != "" {
		document.Components.SecuritySchemes["ProxyHeader"] = &openapi3.SecuritySchemeRef{
			Value: openapi3.NewSecurityScheme().
				WithType("apiKey").
				WithIn("header").
				WithName(options.ProxyHeader.Header).
				WithDescription("Trusted proxy authentication assertion"),
		}
		document.Security = append(document.Security, authenticationRequirement("ProxyHeader"))
	}
	if options.Anonymous {
		document.Security = append(document.Security, openapi3.NewSecurityRequirement())
	}
}

func oauth2AuthorizationCodeFlowConfigured(options OAuth2AuthorizationCodeFlowOptions) bool {
	return options.AuthorizationEndpoint != "" && options.TokenEndpoint != ""
}

func oauth2ClientCredentialsFlowConfigured(options OAuth2ClientCredentialsFlowOptions) bool {
	return options.TokenEndpoint != ""
}

func authenticationRequirement(name string) openapi3.SecurityRequirement {
	requirement := openapi3.NewSecurityRequirement()
	return requirement.Authenticate(name)
}

func openIDConnectDiscoveryEndpoint(issuer string) string {
	return strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
}

func openAPIScopes(names []string) map[string]string {
	scopes := make(map[string]string, len(names))
	for _, scope := range names {
		scopes[scope] = ""
	}
	return scopes
}
