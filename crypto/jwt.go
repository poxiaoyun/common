package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type JWTHeader struct {
	Typ string `json:"typ"`
	Alg string `json:"alg"`
}

func NewJWTRS256(cliams any, signKey *rsa.PrivateKey) (string, error) {
	headerBytes, err := json.Marshal(JWTHeader{Typ: "JWT", Alg: "RS256"})
	if err != nil {
		return "", err
	}
	headerSection := base64.URLEncoding.EncodeToString(headerBytes)
	claimsBytes, err := json.Marshal(cliams)
	if err != nil {
		return "", err
	}
	claimsSection := base64.URLEncoding.EncodeToString(claimsBytes)
	tosign := headerSection + "." + claimsSection

	hasher := crypto.SHA256.New()
	hasher.Write([]byte(tosign))
	// Sign the string and return the encoded bytes
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, signKey, crypto.SHA256, hasher.Sum(nil))
	if err != nil {
		return "", nil
	}
	signatureSection := base64.URLEncoding.EncodeToString(sigBytes)
	return tosign + "." + signatureSection, nil
}

func ValidateJWTRS256(token string, verifyKey *rsa.PublicKey, claims any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid token")
	}
	headerSection, claimsSection, signatureSection := parts[0], parts[1], parts[2]

	header := JWTHeader{}
	headerBytes, err := base64.URLEncoding.DecodeString(headerSection)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return err
	}
	if header.Typ != "JWT" || header.Alg != "RS256" {
		return fmt.Errorf("unsupported header: %v", header)
	}
	sigBytes, err := base64.URLEncoding.DecodeString(signatureSection)
	if err != nil {
		return err
	}
	hasher := crypto.SHA256.New()
	hasher.Write([]byte(headerSection + "." + claimsSection))
	if err = rsa.VerifyPKCS1v15(verifyKey, crypto.SHA256, hasher.Sum(nil), sigBytes); err != nil {
		return err
	}
	claimsBytes, err := base64.URLEncoding.DecodeString(claimsSection)
	if err != nil {
		return err
	}
	return json.Unmarshal(claimsBytes, claims)
}
