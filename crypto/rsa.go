package crypto

import (
	"crypto/rand"
	"crypto/rsa"
)

func GenerateRSAKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}
