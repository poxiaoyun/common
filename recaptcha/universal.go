package recaptcha

import "context"

type RecaptchaOptions struct {
	Google *GoogleRecaptchaOptions `json:"google,omitempty"`
}

func NewUniversalRecaptcha(ctx context.Context, options *RecaptchaOptions) (*UniversalRecaptcha, error) {
	providers := []Recaptcha{}
	if options.Google != nil {
		google, err := NewGoogleRecaptcha(ctx, options.Google)
		if err != nil {
			return nil, err
		}
		providers = append(providers, google)
	}
	return &UniversalRecaptcha{Providers: providers, Options: options}, nil
}

type UniversalRecaptcha struct {
	Providers []Recaptcha
	Options   *RecaptchaOptions
}

// GetConfig implements Recaptcha.
func (u *UniversalRecaptcha) Config() RecaptchaConfig {
	for _, p := range u.Providers {
		return p.Config()
	}
	return RecaptchaConfig{}
}

// Verify implements Recaptcha.
func (u *UniversalRecaptcha) Verify(ctx context.Context, token string, action string) error {
	var lasterr error
	for _, p := range u.Providers {
		if err := p.Verify(ctx, token, action); err != nil {
			lasterr = err
		} else {
			return nil
		}
	}
	return lasterr
}
