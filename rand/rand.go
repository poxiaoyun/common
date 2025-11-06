package rand

import (
	crand "crypto/rand"
	"math/big"
)

const (
	Numbers           = "0123456789"
	LowerLetters      = "abcdefghijklmnopqrstuvwxyz"
	UpperLetters      = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	SpecialCharacters = ".,!@#$%^&*()-_=+"
)

func RandomAlphaNumeric(size int) string {
	return RandomFromCandidates(size, LowerLetters+Numbers) // 36 characters
}

func RandomNumeric(size int) string {
	return RandomFromCandidates(size, Numbers) // 10 characters
}

// RandomPassword generates a password that includes at least one lowercase letter, one uppercase letter, one digit, and one special character.
// If the provided size is less than 4, it will be increased to 4 to ensure each character category is represented.
func RandomPassword(size int) string {
	size = max(size, 4)
	result := make([]byte, size)
	// 确保每类字符至少包含一个
	result[0] = RandomPickByte(LowerLetters)
	result[1] = RandomPickByte(UpperLetters)
	result[2] = RandomPickByte(Numbers)
	result[3] = RandomPickByte(SpecialCharacters)
	combined := LowerLetters + UpperLetters + Numbers + SpecialCharacters
	for i := 4; i < size; i++ {
		result[i] = RandomPickByte(combined)
	}
	ShuffleBytes(result)
	return string(result)
}

func RandomHex(size int) string {
	return RandomFromCandidates(size, "0123456789abcdef") // 16 characters
}

func RandomFromCandidates(size int, candidates string) string {
	if size <= 0 {
		return ""
	}
	result := make([]byte, size)
	for i := range result {
		result[i] = RandomPickByte(candidates)
	}
	return string(result)
}

// Fisher-Yates shuffle using crypto/rand
func ShuffleBytes(b []byte) {
	for i := len(b) - 1; i > 0; i-- {
		j := int(RandomInt(int64(i + 1)))
		b[i], b[j] = b[j], b[i]
	}
}

// --- helpers ---
func RandomInt(max int64) int64 {
	n, err := crand.Int(crand.Reader, big.NewInt(max))
	if err != nil {
		panic(err)
	}
	return n.Int64()
}

func RandomPickByte(candidates string) byte {
	if len(candidates) == 0 {
		return 0
	}
	idx := int(RandomInt(int64(len(candidates))))
	return candidates[idx]
}
