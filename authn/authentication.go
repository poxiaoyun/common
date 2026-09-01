// Package authn defines canonical authenticated identities and successful
// authentication results shared across protocol and domain adapters.
package authn

// SubjectType identifies the identity-owner-established classification of an
// authenticated subject. It is immutable but is not part of the Subject ID.
// The zero value is the canonical type used when authentication does not
// distinguish subject kinds.
type SubjectType string

const (
	// SubjectTypeUser identifies a human user.
	SubjectTypeUser SubjectType = "user"
	// SubjectTypeAnonymous identifies an unauthenticated subject.
	SubjectTypeAnonymous SubjectType = "anonymous"
)

// Subject is the authenticated identity a request is about. ID is its stable,
// globally unique identity for authorization, ownership, and audit.
type Subject struct {
	// ID is the globally unique stable identifier used by authorization,
	// ownership, and audit correlation.
	ID string `json:"id"`
	// Type is the immutable classification of the Subject identified by ID. Its
	// zero value is canonical when authentication does not distinguish kinds.
	Type SubjectType `json:"type,omitempty"`
	// Name is the provider-verified username or principal name within the
	// authentication domain. It is not a stable ownership key.
	Name string `json:"name,omitempty"`
	// DisplayName is a human-facing, non-unique label.
	DisplayName string `json:"displayName,omitempty"`
	// Email is the authenticated subject email when the authentication method
	// provides one.
	Email string `json:"email,omitempty"`
	// EmailVerified reports whether the authentication provider verified Email.
	EmailVerified bool `json:"emailVerified,omitempty"`
	// Groups contains authorization groups assigned to the subject.
	Groups []string `json:"groups,omitempty"`
}

// SubjectReference is the stable cross-module reference to one Subject.
// Subject IDs are globally unique, so Type is classification rather than part
// of the reference identity.
type SubjectReference struct {
	ID string `json:"id"`
}

// Authentication is the verified identity and credential context produced
// by successful authentication.
type Authentication struct {
	// Subject is the effective identity of the authenticated operation.
	Subject `json:",inline"`
	// Actor is the authenticated identity that initiated the operation on behalf
	// of Subject. It does not replace Subject as the effective identity and is nil
	// for direct authentication. An authenticator may set Actor only from verified
	// delegation or impersonation information, never from an unverified request
	// value or by inferring it from a client identifier.
	Actor *Subject `json:"actor,omitempty"`
	// Token contains verified access-token metadata. Nil means authentication did
	// not use an access token; a non-nil empty value remains meaningful.
	Token *TokenInfo `json:"token,omitempty"`
}

// TokenInfo contains metadata established while verifying an access token.
type TokenInfo struct {
	// Audiences are the audiences validated by the authenticator.
	Audiences []string `json:"audiences,omitempty"`
	// Scopes are the access scopes carried by the verified token.
	Scopes []string `json:"scopes,omitempty"`
}
