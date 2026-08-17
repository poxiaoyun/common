package oidc

import (
	"encoding/json"
	"testing"
)

func TestRequestBindingJSONUsesDomainFieldNames(t *testing.T) {
	encoded, err := json.Marshal(AuthorizationCodeFlow{
		URL:          "https://provider.example/authorize",
		State:        "state",
		Nonce:        "nonce",
		CodeVerifier: "verifier",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"url":"https://provider.example/authorize","state":"state","nonce":"nonce","codeVerifier":"verifier"}` {
		t.Fatalf("AuthorizationCodeFlow JSON = %s", encoded)
	}
	encoded, err = json.Marshal(RPInitiatedLogout{
		URL:   "https://provider.example/logout",
		State: "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"url":"https://provider.example/logout","state":"state"}` {
		t.Fatalf("RPInitiatedLogout JSON = %s", encoded)
	}
}
