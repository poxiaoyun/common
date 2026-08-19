package api

// AuthenticationReviewSpec contains exactly one credential to authenticate.
// Audiences applies only to Token credentials.
type AuthenticationReviewSpec struct {
	Token        string `json:"token,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	SSHPublicKey string `json:"sshPublicKey,omitempty"`
	// Audiences is used only when the caller delegates Resource Server audience
	// validation to the review service. Leave it empty when the caller, as Apps
	// and Cloud currently do, validates OAuth access-token audiences locally.
	Audiences []string `json:"audiences,omitempty"`
}

// AuthenticationReviewStatus is the result of an authentication review.
type AuthenticationReviewStatus struct {
	Authenticated  bool                `json:"authenticated"`
	Authentication *AuthenticationInfo `json:"authentication,omitempty"`
	// Audiences is the validated intersection of the requested and token
	// audiences. It is omitted when the request did not delegate validation.
	Audiences []string `json:"audiences,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// AuthenticationReview requests authentication without persisting a resource.
type AuthenticationReview struct {
	Spec   *AuthenticationReviewSpec   `json:"spec,omitempty"`
	Status *AuthenticationReviewStatus `json:"status,omitempty"`
}

// AuthorizationReviewSpec describes the identity and operation to authorize.
type AuthorizationReviewSpec struct {
	Authentication AuthenticationInfo `json:"authentication"`
	Attributes     Attributes         `json:"attributes"`
}

// AuthorizationReviewStatus is the result of an authorization review.
type AuthorizationReviewStatus struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// AuthorizationReview requests an authorization decision without persisting a resource.
type AuthorizationReview struct {
	Spec   *AuthorizationReviewSpec   `json:"spec,omitempty"`
	Status *AuthorizationReviewStatus `json:"status,omitempty"`
}
