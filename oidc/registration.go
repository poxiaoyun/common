package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ClientMetadata is the OAuth 2.0 and OpenID Connect metadata registered for
// one client. AdditionalMetadata preserves extension and localized top-level
// JSON members across RFC 7592 replacement updates.
type ClientMetadata struct {
	RedirectURIs                 []string                   `json:"redirect_uris,omitempty"`
	ResponseTypes                []string                   `json:"response_types,omitempty"`
	GrantTypes                   []string                   `json:"grant_types,omitempty"`
	ApplicationType              string                     `json:"application_type,omitempty"`
	Contacts                     []string                   `json:"contacts,omitempty"`
	ClientName                   string                     `json:"client_name,omitempty"`
	LogoURI                      string                     `json:"logo_uri,omitempty"`
	ClientURI                    string                     `json:"client_uri,omitempty"`
	PolicyURI                    string                     `json:"policy_uri,omitempty"`
	TOSURI                       string                     `json:"tos_uri,omitempty"`
	JWKSURI                      string                     `json:"jwks_uri,omitempty"`
	JWKS                         json.RawMessage            `json:"jwks,omitempty"`
	SectorIdentifierURI          string                     `json:"sector_identifier_uri,omitempty"`
	SubjectType                  string                     `json:"subject_type,omitempty"`
	IDTokenSignedResponseAlg     string                     `json:"id_token_signed_response_alg,omitempty"`
	IDTokenEncryptedResponseAlg  string                     `json:"id_token_encrypted_response_alg,omitempty"`
	IDTokenEncryptedResponseEnc  string                     `json:"id_token_encrypted_response_enc,omitempty"`
	UserInfoSignedResponseAlg    string                     `json:"userinfo_signed_response_alg,omitempty"`
	UserInfoEncryptedResponseAlg string                     `json:"userinfo_encrypted_response_alg,omitempty"`
	UserInfoEncryptedResponseEnc string                     `json:"userinfo_encrypted_response_enc,omitempty"`
	RequestObjectSigningAlg      string                     `json:"request_object_signing_alg,omitempty"`
	RequestObjectEncryptionAlg   string                     `json:"request_object_encryption_alg,omitempty"`
	RequestObjectEncryptionEnc   string                     `json:"request_object_encryption_enc,omitempty"`
	TokenEndpointAuthMethod      string                     `json:"token_endpoint_auth_method,omitempty"`
	TokenEndpointAuthSigningAlg  string                     `json:"token_endpoint_auth_signing_alg,omitempty"`
	DefaultMaxAge                *int64                     `json:"default_max_age,omitempty"`
	RequireAuthTime              *bool                      `json:"require_auth_time,omitempty"`
	DefaultACRValues             []string                   `json:"default_acr_values,omitempty"`
	InitiateLoginURI             string                     `json:"initiate_login_uri,omitempty"`
	RequestURIs                  []string                   `json:"request_uris,omitempty"`
	PostLogoutRedirectURIs       []string                   `json:"post_logout_redirect_uris,omitempty"`
	Scope                        string                     `json:"scope,omitempty"`
	SoftwareID                   string                     `json:"software_id,omitempty"`
	SoftwareVersion              string                     `json:"software_version,omitempty"`
	SoftwareStatement            string                     `json:"software_statement,omitempty"`
	AdditionalMetadata           map[string]json.RawMessage `json:"-"`
}

var clientMetadataJSONNames = []string{
	"redirect_uris",
	"response_types",
	"grant_types",
	"application_type",
	"contacts",
	"client_name",
	"logo_uri",
	"client_uri",
	"policy_uri",
	"tos_uri",
	"jwks_uri",
	"jwks",
	"sector_identifier_uri",
	"subject_type",
	"id_token_signed_response_alg",
	"id_token_encrypted_response_alg",
	"id_token_encrypted_response_enc",
	"userinfo_signed_response_alg",
	"userinfo_encrypted_response_alg",
	"userinfo_encrypted_response_enc",
	"request_object_signing_alg",
	"request_object_encryption_alg",
	"request_object_encryption_enc",
	"token_endpoint_auth_method",
	"token_endpoint_auth_signing_alg",
	"default_max_age",
	"require_auth_time",
	"default_acr_values",
	"initiate_login_uri",
	"request_uris",
	"post_logout_redirect_uris",
	"scope",
	"software_id",
	"software_version",
	"software_statement",
}

// MarshalJSON encodes standard, extension, and localized metadata as members
// of one protocol-level JSON object.
func (m ClientMetadata) MarshalJSON() ([]byte, error) {
	type clientMetadataWire ClientMetadata
	standardData, err := json.Marshal(clientMetadataWire(m))
	if err != nil {
		return nil, err
	}
	standard := map[string]json.RawMessage{}
	if err := json.Unmarshal(standardData, &standard); err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	for name, value := range m.AdditionalMetadata {
		result[name] = value
	}
	for _, name := range clientMetadataJSONNames {
		delete(result, name)
	}
	for name, value := range standard {
		result[name] = value
	}
	return json.Marshal(result)
}

// UnmarshalJSON decodes standard metadata and retains every unknown top-level
// member in AdditionalMetadata.
func (m *ClientMetadata) UnmarshalJSON(data []byte) error {
	type clientMetadataWire ClientMetadata
	if err := json.Unmarshal(data, (*clientMetadataWire)(m)); err != nil {
		return err
	}
	additional := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &additional); err != nil {
		return err
	}
	for _, name := range clientMetadataJSONNames {
		delete(additional, name)
	}
	if len(additional) == 0 {
		additional = nil
	}
	m.AdditionalMetadata = additional
	return nil
}

// ClientRegistration is the server's registered client information and the
// credentials used to access its RFC 7592 Client Configuration Endpoint.
type ClientRegistration struct {
	Metadata                ClientMetadata `json:"-"`
	ClientID                string         `json:"client_id"`
	ClientSecret            string         `json:"client_secret,omitempty"`
	ClientIDIssuedAt        *int64         `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt   *int64         `json:"client_secret_expires_at,omitempty"`
	RegistrationAccessToken string         `json:"registration_access_token,omitempty"`
	RegistrationClientURI   string         `json:"registration_client_uri,omitempty"`
}

var clientRegistrationJSONNames = []string{
	"client_id",
	"client_secret",
	"client_id_issued_at",
	"client_secret_expires_at",
	"registration_access_token",
	"registration_client_uri",
}

// MarshalJSON encodes client information and registered metadata as one
// protocol-level JSON object.
func (r ClientRegistration) MarshalJSON() ([]byte, error) {
	metadataData, err := json.Marshal(r.Metadata)
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	if err := json.Unmarshal(metadataData, &result); err != nil {
		return nil, err
	}
	type clientRegistrationWire ClientRegistration
	informationData, err := json.Marshal(clientRegistrationWire(r))
	if err != nil {
		return nil, err
	}
	information := map[string]json.RawMessage{}
	if err := json.Unmarshal(informationData, &information); err != nil {
		return nil, err
	}
	for name, value := range information {
		result[name] = value
	}
	return json.Marshal(result)
}

// UnmarshalJSON decodes client information and registered metadata from one
// protocol-level JSON object.
func (r *ClientRegistration) UnmarshalJSON(data []byte) error {
	type clientRegistrationWire ClientRegistration
	information := clientRegistrationWire{}
	if err := json.Unmarshal(data, &information); err != nil {
		return err
	}
	metadata := ClientMetadata{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return err
	}
	for _, name := range clientRegistrationJSONNames {
		delete(metadata.AdditionalMetadata, name)
	}
	if len(metadata.AdditionalMetadata) == 0 {
		metadata.AdditionalMetadata = nil
	}
	*r = ClientRegistration(information)
	r.Metadata = metadata
	return nil
}

// ClientRegistrationOptions configures one RFC 7591 registration request.
type ClientRegistrationOptions struct {
	RegistrationEndpoint string
	InitialAccessToken   string
	HTTPClient           *http.Client
}

// RegisterClient creates a dynamic client registration using RFC 7591.
func RegisterClient(ctx context.Context, options ClientRegistrationOptions, metadata ClientMetadata) (*ClientRegistration, error) {
	if options.RegistrationEndpoint == "" {
		return nil, fmt.Errorf("%w: registration", ErrUnsupportedEndpoint)
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, options.RegistrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if options.InitialAccessToken != "" {
		request.Header.Set("Authorization", "Bearer "+options.InitialAccessToken)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	result := &ClientRegistration{}
	if err := DecodeEndpointResponseStatus(response, result, http.StatusCreated); err != nil {
		return nil, err
	}
	if err := ValidateClientRegistration(result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetClientRegistration reads an RFC 7592 client registration. The returned
// value replaces current because the server may rotate its credentials.
func GetClientRegistration(ctx context.Context, current *ClientRegistration, httpClient *http.Client) (*ClientRegistration, error) {
	request, err := newClientRegistrationRequest(ctx, http.MethodGet, current, nil)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	result := &ClientRegistration{}
	if err := DecodeEndpointResponseStatus(response, result, http.StatusOK); err != nil {
		return nil, err
	}
	return mergeClientRegistration(current, result)
}

// UpdateClientRegistration replaces all RFC 7592 client metadata. Callers
// should start from the metadata returned by the preceding registration,
// read, or update operation so server-provisioned fields are retained.
func UpdateClientRegistration(ctx context.Context, current *ClientRegistration, metadata ClientMetadata, httpClient *http.Client) (*ClientRegistration, error) {
	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for _, name := range clientRegistrationJSONNames {
		delete(payload, name)
	}
	clientID, err := json.Marshal(current.ClientID)
	if err != nil {
		return nil, err
	}
	payload["client_id"] = clientID
	body, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := newClientRegistrationRequest(ctx, http.MethodPut, current, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	result := &ClientRegistration{}
	if err := DecodeEndpointResponseStatus(response, result, http.StatusOK); err != nil {
		return nil, err
	}
	return mergeClientRegistration(current, result)
}

// DeleteClientRegistration deletes an RFC 7592 dynamic client registration.
func DeleteClientRegistration(ctx context.Context, current *ClientRegistration, httpClient *http.Client) error {
	request, err := newClientRegistrationRequest(ctx, http.MethodDelete, current, nil)
	if err != nil {
		return err
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	return DecodeEndpointResponseStatus(response, nil, http.StatusNoContent)
}

// ValidateClientRegistration validates the required relationships in an RFC
// 7591 client information response.
func ValidateClientRegistration(registration *ClientRegistration) error {
	if registration.ClientID == "" {
		return fmt.Errorf("oidc: client registration response is missing client_id")
	}
	if registration.ClientSecret != "" && registration.ClientSecretExpiresAt == nil {
		return fmt.Errorf("oidc: client registration response is missing client_secret_expires_at")
	}
	if (registration.RegistrationAccessToken == "") != (registration.RegistrationClientURI == "") {
		return fmt.Errorf("oidc: client registration response has incomplete management credentials")
	}
	return nil
}

func newClientRegistrationRequest(ctx context.Context, method string, current *ClientRegistration, body []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, current.RegistrationClientURI, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+current.RegistrationAccessToken)
	return request, nil
}

func mergeClientRegistration(current, result *ClientRegistration) (*ClientRegistration, error) {
	if err := ValidateClientRegistration(result); err != nil {
		return nil, err
	}
	if result.ClientID != current.ClientID {
		return nil, fmt.Errorf("oidc: client registration response changed client_id")
	}
	// RFC 7592 sections 2.1 and 2.2 require callers to immediately replace a
	// client secret or registration access token returned by the server. If a
	// response omits either credential, the previously issued value remains the
	// effective credential.
	if result.ClientSecret == "" {
		result.ClientSecret = current.ClientSecret
		result.ClientSecretExpiresAt = current.ClientSecretExpiresAt
	}
	if result.ClientIDIssuedAt == nil {
		result.ClientIDIssuedAt = current.ClientIDIssuedAt
	}
	if result.RegistrationAccessToken == "" {
		result.RegistrationAccessToken = current.RegistrationAccessToken
		result.RegistrationClientURI = current.RegistrationClientURI
	}
	return result, nil
}
