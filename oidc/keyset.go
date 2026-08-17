package oidc

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	jose "github.com/go-jose/go-jose/v4"
)

// KeySet verifies signatures with keys loaded from a JWKS endpoint.
type KeySet struct {
	URL    string
	Client *http.Client

	mu   sync.Mutex
	keys jose.JSONWebKeySet
}

// JWTHeader contains the protected header values used during verification.
type JWTHeader struct {
	Type string
}

// Verify verifies a compact JWS using the cached provider keys. It reloads the
// JWKS once when the current keys do not verify the signature.
func (k *KeySet) Verify(ctx context.Context, raw string, algorithms []string) ([]byte, JWTHeader, error) {
	allowed := make([]jose.SignatureAlgorithm, len(algorithms))
	for index, algorithm := range algorithms {
		allowed[index] = jose.SignatureAlgorithm(algorithm)
	}
	signed, err := jose.ParseSignedCompact(raw, allowed)
	if err != nil {
		return nil, JWTHeader{}, err
	}
	signedHeader := signed.Signatures[0].Header
	typeValue, _ := signedHeader.ExtraHeaders[jose.HeaderType].(string)
	header := JWTHeader{
		Type: typeValue,
	}
	k.mu.Lock()
	defer k.mu.Unlock()

	if payload, ok := verifyWithKeys(signed, k.keys); ok {
		return payload, header, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, k.URL, nil)
	if err != nil {
		return nil, JWTHeader{}, err
	}
	response, err := k.Client.Do(request)
	if err != nil {
		return nil, JWTHeader{}, err
	}
	var keys jose.JSONWebKeySet
	if err := DecodeEndpointResponse(response, &keys); err != nil {
		return nil, JWTHeader{}, err
	}
	k.keys = keys
	if payload, ok := verifyWithKeys(signed, k.keys); ok {
		return payload, header, nil
	}
	return nil, JWTHeader{}, fmt.Errorf("oidc: JWT signature is invalid")
}

func verifyWithKeys(signed *jose.JSONWebSignature, set jose.JSONWebKeySet) ([]byte, bool) {
	kid := signed.Signatures[0].Header.KeyID
	keys := set.Keys
	if kid != "" {
		keys = set.Key(kid)
	}
	for _, key := range keys {
		payload, err := signed.Verify(key.Key)
		if err != nil {
			continue
		}
		return payload, true
	}
	return nil, false
}
