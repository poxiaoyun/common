package otp

import "time"

const (
	// We RECOMMEND a default time-step size of 30 seconds.  This default
	// value of 30 seconds is selected as a balance between security and
	// usability.
	DefaultTimeStep = 30
)

// https://datatracker.ietf.org/doc/html/rfc6238
func GenerateTOTP(key string, time time.Time) string {
	return GenerateHOTP(key, uint64(time.Unix()/DefaultTimeStep))
}

// ValidateTOTP validates a TOTP value
// key is the pre shared secret
// otp is the one time password
// time is the current time
// window is the number of intervals to check before and after the current time
func ValidateTOTP(key string, otp string, time time.Time, skew time.Duration) bool {
	counter := uint64(time.Unix() / DefaultTimeStep)
	if ValidateHOTP(key, counter, otp) {
		return true
	}
	for i := 1; i <= int(skew/DefaultTimeStep); i++ {
		if ValidateHOTP(key, counter+uint64(i), otp) || ValidateHOTP(key, counter-uint64(i), otp) {
			return true
		}
	}
	return false
}
