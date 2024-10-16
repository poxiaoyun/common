package otp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
)

var DefaultHash = sha1.New

// https://datatracker.ietf.org/doc/html/rfc4226
// GenerateHOTP generates an HMAC-SHA1 6-digit HOTP value
func GenerateHOTP(key string, counter uint64) string {
	// Step 1: Generate an HMAC-SHA-1 value Let HS = HMAC-SHA-1(K,C)
	c := make([]byte, 8)
	binary.BigEndian.PutUint64(c, counter)
	mac := hmac.New(DefaultHash, []byte(key))
	mac.Write(c)
	hs := mac.Sum(nil)
	// Step 2: Generate a 4-byte string (Dynamic Truncation)
	offset := hs[19] & 0xf
	sbits := uint32(hs[offset]&0x7f)<<24 | uint32(hs[offset+1])<<16 |
		uint32(hs[offset+2])<<8 | uint32(hs[offset+3])
	// Step 3: Compute an HOTP value
	return fmt.Sprintf("%06d", sbits%1e6)
}

func ValidateHOTP(key string, counter uint64, otp string) bool {
	// avoid timing attacks
	return subtle.ConstantTimeCompare([]byte(otp), []byte(GenerateHOTP(key, counter))) == 1
}
