package authn_test

import (
	"encoding/json"
	"testing"

	"xiaoshiai.cn/common/authn"
)

func TestSubjectJSONUsesCanonicalFields(t *testing.T) {
	subject := authn.Subject{
		Type:          authn.SubjectTypeUser,
		ID:            "subject-1",
		Name:          "alice",
		DisplayName:   "Alice",
		Email:         "alice@example.com",
		EmailVerified: true,
		Groups:        []string{"developers"},
	}
	encoded, err := json.Marshal(subject)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"subject-1","type":"user","name":"alice","displayName":"Alice","email":"alice@example.com","emailVerified":true,"groups":["developers"]}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}

func TestSubjectReferenceJSONUsesCanonicalIDField(t *testing.T) {
	encoded, err := json.Marshal(authn.SubjectReference{ID: "subject-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"subject-1"}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}

func TestAuthenticationJSONUsesCanonicalFields(t *testing.T) {
	info := authn.Authentication{
		Subject: authn.Subject{Type: authn.SubjectTypeUser, ID: "subject-1"},
		Actor:   &authn.Subject{ID: "service-1"},
		Token:   &authn.TokenInfo{Audiences: []string{"api"}, Scopes: []string{"read"}},
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"subject-1","type":"user","actor":{"id":"service-1"},"token":{"audiences":["api"],"scopes":["read"]}}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}
