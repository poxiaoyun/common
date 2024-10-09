package rand

import "golang.org/x/exp/rand"

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

func RandomFromCandidates(size int, candidates string) string {
	password := make([]byte, size)
	for i := 0; i < size; i++ {
		password[i] = candidates[rand.Intn(len(candidates))]
	}
	return string(password)
}
