package rand

import (
	"testing"
)

func TestRandomPassword(t *testing.T) {
	size := 12
	pw := RandomPassword(size)
	if len(pw) != size {
		t.Fatalf("expected length %d, got %d", size, len(pw))
	}

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false
	specialSet := ".,!@#$%^&*()-_=+"

	for i := 0; i < len(pw); i++ {
		c := pw[i]
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			for j := 0; j < len(specialSet); j++ {
				if byte(specialSet[j]) == c {
					hasSpecial = true
					break
				}
			}
		}
	}

	if !hasLower || !hasUpper || !hasDigit || !hasSpecial {
		t.Fatalf("password missing categories: lower=%v upper=%v digit=%v special=%v pw=%s", hasLower, hasUpper, hasDigit, hasSpecial, pw)
	}
}
