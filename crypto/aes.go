package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

const DefaultCBCEncryptIV = "1234567890123456"

var ErrInvalidData = errors.New("invalid data")

// The key argument should be the AES key,
// either 16, 24, or 32 bytes to select
// AES-128, AES-192, or AES-256.
func AesCbcPKCS7EncryptBase64(key []byte, data []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	data = PKCS7Padding(data, block.BlockSize())
	crypted := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, []byte(DefaultCBCEncryptIV)).CryptBlocks(crypted, data)
	return base64.StdEncoding.EncodeToString(crypted), nil
}

func AesCbcPKCS7DecryptBase64(key []byte, b64data string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data)%block.BlockSize() != 0 {
		return nil, ErrInvalidData
	}
	origData := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, []byte(DefaultCBCEncryptIV)).CryptBlocks(origData, data)
	return PKCS7UnPadding(origData), nil
}

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

func RandomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
