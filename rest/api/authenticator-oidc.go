package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

func NewDefaultOIDCOptions() *OIDCOptions {
	return &OIDCOptions{
		Scope:          []string{oidc.ScopeOpenID, "profile"},
		UsernameClaims: []string{"name"},
	}
}

type OIDCOptions struct {
	Issuer   string `json:"issuer" description:"oidc issuer url"`
	Insecure bool   `json:"insecure" description:"skip issuer and audience verification"`

	// ClientID is the OAuth2 client ID for this server.
	ClientID string `json:"clientID" description:"oidc client id"`

	// ClientSecret is the secret for the client ID. If no secret is provided,
	// the client is assumed to be a public client and authentication will
	// proceed without a client secret.
	ClientSecret string `json:"clientSecret" description:"oidc client secret"`

	// Scopes is the set of scopes to request.
	Scope []string `json:"scope" description:"oidc scope"`

	// UsernameClaims is the list of claims to check for a username.
	UsernameClaims []string `json:"usernameClaims,omitempty" description:"username claims, default is 'name'"`
}

type OIDCAuthenticator struct {
	Verifier               *oidc.IDTokenVerifier
	UsernameClaimCandidate []string
	EmailClaimCandidate    []string
	GroupsClaimCandidate   []string
	EmailToUsername        func(email string) string
}

var _ TokenAuthenticator = &OIDCAuthenticator{}

func NewOIDCAuthenticator(ctx context.Context, opts *OIDCOptions) (*OIDCAuthenticator, error) {
	if opts.Issuer == "" {
		return nil, fmt.Errorf("oidc issuer is required")
	}
	ctx = oidc.InsecureIssuerURLContext(ctx, opts.Issuer)
	provider, err := oidc.NewProvider(ctx, opts.Issuer)
	if err != nil {
		return nil, fmt.Errorf("init oidc provider: %v", err)
	}
	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: opts.ClientID == "",
		SkipIssuerCheck:   true,
		ClientID:          opts.ClientID,
	})
	return &OIDCAuthenticator{
		Verifier:               verifier,
		UsernameClaimCandidate: opts.UsernameClaims,
		EmailClaimCandidate:    []string{"email"},
		GroupsClaimCandidate:   []string{"groups", "roles"},
		EmailToUsername: func(email string) string {
			return strings.Split(email, "@")[0]
		},
	}, nil
}

func (o *OIDCAuthenticator) Authenticate(ctx context.Context, token string) (*AuthenticateInfo, error) {
	if token == "" {
		return nil, fmt.Errorf("no token found")
	}
	token = strings.TrimPrefix(token, "Bearer ")
	idToken, err := o.Verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify token: %v", err)
	}
	var c claims
	if err := idToken.Claims(&c); err != nil {
		return nil, fmt.Errorf("oidc: parse claims: %v", err)
	}
	// username
	var username string
	for _, candidate := range o.UsernameClaimCandidate {
		if err := c.unmarshalClaim(candidate, &username); err != nil {
			continue
		}
		if username != "" {
			break
		}
	}
	// email
	var email string
	for _, candidate := range o.EmailClaimCandidate {
		if err := c.unmarshalClaim(candidate, &email); err != nil {
			continue
		}
		// If the email_verified claim is present, ensure the email is valid.
		// https://openid.net/specs/openid-connect-core-1_0.html#StandardClaims
		if hasEmailVerified := c.hasClaim("email_verified"); hasEmailVerified {
			var emailVerified bool
			if err := c.unmarshalClaim("email_verified", &emailVerified); err != nil {
				return nil, fmt.Errorf("oidc: parse 'email_verified' claim: %v", err)
			}
			// If the email_verified claim is present we have to verify it is set to `true`.
			if !emailVerified {
				return nil, fmt.Errorf("oidc: email not verified")
			}
		}
		if email != "" {
			break
		}
	}
	// if no username, use email as username
	if username == "" {
		if email != "" {
			username = o.EmailToUsername(email)
		} else {
			return nil, fmt.Errorf("oidc: no username/email claim found")
		}
	}
	// groups
	var groups stringOrArray
	for _, candidate := range o.GroupsClaimCandidate {
		if c.hasClaim(candidate) {
			if err := c.unmarshalClaim(candidate, &groups); err != nil {
				return nil, fmt.Errorf("oidc: parse groups claim %q: %v", candidate, err)
			}
			break
		}
	}
	info := UserInfo{
		ID:     idToken.Subject,
		Name:   username,
		Email:  email,
		Groups: groups,
	}
	return &AuthenticateInfo{Audiences: idToken.Audience, User: info}, nil
}

type claims map[string]json.RawMessage

func (c claims) unmarshalClaim(name string, v interface{}) error {
	val, ok := c[name]
	if !ok {
		return fmt.Errorf("claim not present")
	}
	return json.Unmarshal([]byte(val), v)
}

func (c claims) hasClaim(name string) bool {
	if _, ok := c[name]; !ok {
		return false
	}
	return true
}

type stringOrArray []string

func (s *stringOrArray) UnmarshalJSON(b []byte) error {
	var a []string
	if err := json.Unmarshal(b, &a); err == nil {
		*s = a
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	*s = []string{str}
	return nil
}
