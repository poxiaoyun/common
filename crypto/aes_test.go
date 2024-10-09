package crypto

import (
	"bytes"
	"testing"
)

func TestAesCbcPKCS7Decrypt(t *testing.T) {
	data := []byte("hello world")
	key := RandomBytes(32)

	encrypted, err := AesCbcPKCS7EncryptBase64(key, data)
	if err != nil {
		t.Errorf("AesCbcPKCS7Decrypt() error = %v", err)
		return
	}
	decodedData, err := AesCbcPKCS7DecryptBase64(key, encrypted)
	if err != nil {
		t.Errorf("AesCbcPKCS7Decrypt() error = %v", err)
		return
	}
	if !bytes.Equal(data, decodedData) {
		t.Errorf("AesCbcPKCS7Decrypt() = %v, want %v", decodedData, data)
	}
}
