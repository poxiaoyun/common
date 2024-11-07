package rand

import (
	"crypto/rand"
	"math/big"
)

func RandomAlphaNumeric(size int) string {
	return RandomFromCandidates(size, "abcdefghijklmnopqrstuvwxyz0123456789") // 36 characters
}

func RandomNumeric(size int) string {
	return RandomFromCandidates(size, "0123456789") // 10 characters
}

func RandomPassword(size int) string {
	candidates := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.,!@#$%^&*()-_=+" // 72 characters
	return RandomFromCandidates(size, candidates)
}

func RandomHex(size int) string {
	return RandomFromCandidates(size, "0123456789abcdef") // 16 characters
}

func RandomFromCandidates(size int, candidates string) string {
	result := make([]byte, size)
	for i := 0; i < size; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
		if err != nil {
			panic(err) // handle error appropriately
		}
		result[i] = candidates[n.Int64()]
	}
	return string(result)
}
