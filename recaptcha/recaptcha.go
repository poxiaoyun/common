package recaptcha

import (
	"context"
)

const (
	RecaptchaActionLogin       = "login"
	RecaptchaActionRegister    = "register"
	RecaptchaActionEmailVerify = "email-verify"
)

type RecaptchaConfig struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	SiteKey     string            `json:"siteKey"`
}

type Recaptcha interface {
	// Config returns the configuration for the recaptcha provider.
	// It used to render the recaptcha widget at frontend.
	Config() RecaptchaConfig
	Verify(ctx context.Context, token string, action string) error
}

