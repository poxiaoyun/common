package recaptcha

import (
	"context"
	"fmt"

	recaptcha "cloud.google.com/go/recaptchaenterprise/v2/apiv1"
	recaptchapb "cloud.google.com/go/recaptchaenterprise/v2/apiv1/recaptchaenterprisepb"
)

const DefaultConfidence = 0.7

type GoogleRecaptchaOptions struct {
	ProjectID string `json:"projectID,omitempty"`
	SiteKey   string `json:"siteKey,omitempty"`
	// Confidence is the minimum confidence score for the assessment to be considered successful.
	Confindence float32 `json:"confindence,omitempty"`
}

type GoogleRecaptcha struct {
	Options *GoogleRecaptchaOptions
	Client  *recaptcha.Client
}

func NewGoogleRecaptcha(ctx context.Context, options *GoogleRecaptchaOptions) (*GoogleRecaptcha, error) {
	client, err := recaptcha.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	if options.Confindence < 0 || options.Confindence > 1 {
		return nil, fmt.Errorf("confidence must be between 0 and 1")
	}
	if options.Confindence == 0 {
		options.Confindence = DefaultConfidence
	}
	return &GoogleRecaptcha{Options: options, Client: client}, nil
}

func (r *GoogleRecaptcha) Config() RecaptchaConfig {
	return RecaptchaConfig{SiteKey: r.Options.SiteKey}
}

func (r *GoogleRecaptcha) Verify(ctx context.Context, token string, action string) error {
	if token == "" {
		return fmt.Errorf("recaptcha token is empty")
	}
	if action == "" {
		return fmt.Errorf("recaptcha action is empty")
	}
	request := &recaptchapb.CreateAssessmentRequest{
		Assessment: &recaptchapb.Assessment{
			Event: &recaptchapb.Event{
				Token:   token,
				SiteKey: r.Options.SiteKey,
			},
		},
		Parent: fmt.Sprintf("projects/%s", r.Options.ProjectID),
	}
	response, err := r.Client.CreateAssessment(ctx, request)
	if err != nil {
		return err
	}
	if !response.TokenProperties.Valid {
		return fmt.Errorf("token was invalid for the following reasons: %v", response.TokenProperties.InvalidReason)
	}
	if response.TokenProperties.Action != action {
		return fmt.Errorf("The action %s does not match the action %s", response.TokenProperties.Action, action)
	}
	// https://cloud.google.com/recaptcha-enterprise/docs/interpret-assessment
	if response.RiskAnalysis.Score < r.Options.Confindence {
		resones := ""
		for _, reason := range response.RiskAnalysis.Reasons {
			resones += reason.String() + ";"
		}
		return fmt.Errorf("The reCAPTCHA score is below the minimum confidence score of %f,resonse: %s", r.Options.Confindence, resones)
	}
	return nil
}
