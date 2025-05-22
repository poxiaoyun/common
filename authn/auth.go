package authn

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
)

const (
	LoginErrorReasonNeedCaptcha    errors.StatusReason = "NeedCaptcha"
	LoginErrorReasonInvalidCaptcha errors.StatusReason = "InvalidCaptcha"
)

// ErrorNeedCaptcha is the error when the login requires a captcha
var ErrorNeedCaptcha = errors.NewCustomError(http.StatusBadRequest, LoginErrorReasonNeedCaptcha, "Need captcha")

var ErrorInvalidCaptcha = errors.NewCustomError(http.StatusBadRequest, LoginErrorReasonInvalidCaptcha, "Invalid captcha")

var ErrorUnauthorized = errors.NewUnauthorized("Unauthorized")

var ErrorInvalidUsernameOrPassword = errors.NewUnauthorized("Invalid account or password")

var ErrorInvalidVerificationCode = errors.NewUnauthorized("Invalid verification code")

var ErrorAlreadyLoggedIn = errors.NewCustomError(http.StatusBadRequest, "AlreadyLoggedIn", "Already logged in")

type LoginConfiguration struct {
	AllowSignup bool          `json:"allowSignup"`
	Methods     []LoginMethod `json:"methods"`
}

type LoginMethodType string

const (
	LoginMethodTypePassword LoginMethodType = "Password"
	LoginMethodTypeOauth2   LoginMethodType = "OAuth2" // Third-party login
	LoginMethodTypeOIDC     LoginMethodType = "OIDC"   // OpenID Connect
	LoginMethodTypeOTP      LoginMethodType = "OTP"    // One Time Password (OTP) / SMS / Email
	LoginMethodTypeWebAuthn LoginMethodType = "WebAuthn"
)

type LoginMethod struct {
	Type     LoginMethodType      `json:"type,omitempty"`
	Password *PasswordLoginConfig `json:"password,omitempty"`
	Oauth2   *Oauth2LoginConfig   `json:"oauth2,omitempty"`
	OTP      *OTPLoginConfig      `json:"otp,omitempty"`
	OIDC     *OIDCLoginConfig     `json:"oidc,omitempty"`
}

type PasswordAlgorithm string

const (
	PasswordAlgorithmPlainText PasswordAlgorithm = "PlainText"
	PasswordAlgorithmBCrypt    PasswordAlgorithm = "BCrypt"
)

type (
	PasswordLoginConfig struct {
		Algorithm PasswordAlgorithm `json:"algorithm,omitempty"`
	}
	OTPLoginConfig struct {
		Provider string            `json:"provider,omitempty"`
		Params   map[string]string `json:"params,omitempty"`
	}
	Oauth2LoginConfig struct {
		Name         string `json:"name,omitempty"`
		Provider     string `json:"provider,omitempty"`
		AuthorizeURL string `json:"authorizeURL,omitempty"`
		TokenURL     string `json:"tokenURL,omitempty"`
		UserInfoURL  string `json:"userInfoURL,omitempty"`
		ClientID     string `json:"clientID,omitempty"`
		Scope        string `json:"scope,omitempty"`
		ResponseType string `json:"responseType,omitempty"`
		// ClientSecret is only used for the client credential flow
		ClientSecret string `json:"clientSecret,omitempty"`
	}
	OIDCLoginConfig struct {
		Provider        string          `json:"provider,omitempty"`
		Issuer          string          `json:"issuer,omitempty"`
		ClientID        string          `json:"clientID,omitempty"`
		ClientSecret    string          `json:"clientSecret,omitempty"`
		Scope           string          `json:"scope,omitempty"`
		ResponseType    string          `json:"responseType,omitempty"`
		UserInfoMapping UserInfoMapping `json:"userInfoMapping,omitempty"`
	}
)

type UserInfoMapping struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

func (a *API) GetLoginConfig(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		options := GetConfigurationOptions{
			Platform: api.Query(r, "platform", ""),
		}
		return a.Provider.GetConfiguration(ctx, options)
	})
}

type LoginData struct {
	Type      LoginMethodType `json:"type" validate:"required"`
	Username  string          `json:"username"`
	RemeberMe bool            `json:"remeberMe"`
	Captcha   CaptchData      `json:"captcha"`

	//  Password is the password of the user
	Password PasswordData `json:"password"`
	// Auth2Code is the code from the oauth2 service
	// It is used to exchange the access token and verify the user
	Oauth2 Oauth2Data `json:"oauth2,omitempty"`
	// OTPCode is the one-time password
	OTP OTPData `json:"otp,omitempty"`
}

type Oauth2Data struct {
	Provider    string `json:"provider"`
	Code        string `json:"code"`
	RedirectURI string `json:"redirectURI"`
}

type PasswordData struct {
	Value     string            `json:"value"`
	Algorithm PasswordAlgorithm `json:"algorithm"`
}

type NextLoginType string

const (
	NextLoginTypeMFA NextLoginType = "MFA"
)

type LoginResponse struct {
	// Next is the next step for the login
	// If the next is empty, the login is successful
	// If the next is MFA, the login requires MFA
	Next NextLoginType `json:"next,omitempty"`
	// MFA is the MFA configuration for next login
	MFA *MFAConfig `json:"mfa,omitempty"`
	// Token is the token for an successful login
	Token          string `json:"token,omitempty"`
	TokenExpiresIn int    `json:"tokenExpiresIn,omitempty"`
}

func (a *API) SignIn(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		login := &LoginData{}
		if err := api.Body(r, login); err != nil {
			return nil, err
		}
		if auditlog := api.AuditLogFromContext(ctx); auditlog != nil {
			auditlog.Subject = login.Username
		}
		return a.Provider.Signin(ctx, session, *login)
	})
}

func (a *API) SignOut(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		if auditlog := api.AuditLogFromContext(ctx); auditlog != nil {
			profile, _ := a.Provider.GetCurrentProfile(ctx, session)
			if profile != nil {
				auditlog.Subject = profile.Name
			}
		}
		if err := a.Provider.Signout(ctx, session); err != nil {
			return nil, err
		}
		api.UnsetCookie(w, SessionCookieKey)
		return errors.NewOK(), nil
	})
}

type SignUpData struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	//  Password is the password of the user
	Password PasswordData `json:"password"`
	// Email is the email of the user
	Email EmailData `json:"email"`
	// Phone is the phone number of the user
	Phone PhoneData `json:"phone"`
	// Agreement is the agreement of the user
	Agreement bool `json:"agreement"`
	// Captcha is the captcha for this request
	Captcha CaptchData `json:"captcha"`
}

type EmailData struct {
	// Value is the email address
	Value string `json:"value"`
	// Code is the verification code of the email
	Code string `json:"code"`
}

type PhoneData struct {
	// Value is the phone number
	Value string `json:"value"`
	// Code is the verification code of the phone
	Code string `json:"code"`
}

func (a *API) SignUp(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := SignUpData{}
		if err := api.Body(r, &data); err != nil {
			return nil, err
		}
		return nil, a.Provider.Signup(ctx, session, data)
	})
}

func (a *API) GetCurrentProfile(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		account, err := a.Provider.GetCurrentProfile(ctx, session)
		if err != nil {
			return nil, err
		}
		return account, nil
	})
}

func (a *API) UpdateCurrentProfile(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := &UserProfile{}
		if err := api.Body(r, data); err != nil {
			return nil, err
		}
		return nil, a.Provider.UpdateCurrentProfile(ctx, session, *data)
	})
}

type ResetPasswordData struct {
	Password    string `json:"password"`
	NewPassword string `json:"newPassword"`
}

func (a *API) ResetPassword(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := ResetPasswordData{}
		if err := api.Body(r, &data); err != nil {
			return nil, err
		}
		return nil, a.Provider.ResetPassword(ctx, session, data)
	})
}

type ResetEmailData struct {
	NewEmail string `json:"newEmail"`
	Code     string `json:"code"`
}

func (a *API) ResetEmail(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := ResetEmailData{}
		if err := api.Body(r, &data); err != nil {
			return nil, err
		}
		return nil, a.Provider.ResetEmail(ctx, session, data)
	})
}

func (a *API) ResetPhone(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := ResetPhoneData{}
		if err := api.Body(r, &data); err != nil {
			return nil, err
		}
		return nil, a.Provider.ResetPhone(ctx, session, data)
	})
}

type ResetPhoneData struct {
	NewPhone string `json:"newPhone"`
	Code     string `json:"code"`
}

type OTPData struct {
	Provider string `json:"provider"`
	Code     string `json:"code"`
}

func (a *API) SendOTPCode(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		send := &SendOTPCode{}
		if err := api.Body(r, send); err != nil {
			return nil, err
		}
		if err := a.Provider.SendOTPCode(ctx, *send); err != nil {
			return nil, err
		}
		return errors.NewOK(), nil
	})
}

type VerrifyMFAData struct {
	// Action is the action for the MFA
	// bind: bind the MFA
	// login: login with the MFA
	Action   string `json:"action"`
	Provider string `json:"provider"`
	Code     string `json:"code"`
	//  RecoveryCode is an optional string use to recover the MFA
	// it is another type of MFA "code"
	RecoveryCode string `json:"recoveryCode"`
}

type MFAConfig struct {
	// Enabled is the MFA enabled status for the current user
	// if user not enable the MFA, client should guide the user to enable the MFA
	// if user enable the MFA, client should show the MFA code input
	Enabled  bool              `json:"enabled"`
	Provider MFAProvider       `json:"provider,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

type InitMFAResponse struct {
	MFAInitConfig `json:",inline"`
	// URL is an otpauth URL for the MFA
	// https://github.com/google/google-authenticator/wiki/Key-Uri-Format
	// https://datatracker.ietf.org/doc/draft-linuxgemini-otpauth-uri/01/
	URL string `json:"url,omitempty"`
}

func (a *API) InitMFA(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := &InitMFAOptions{}
		if err := api.Body(r, data); err != nil {
			return nil, err
		}
		config, err := a.Provider.InitMFA(ctx, session, *data)
		if err != nil {
			return nil, err
		}
		// https://github.com/google/google-authenticator/wiki/Key-Uri-Format
		issuer := "BOB"
		otpauth := url.URL{
			Scheme: "otpauth",
			// must be totp
			// https://tools.ietf.org/html/rfc6238
			Path:     fmt.Sprintf("totp/%s: %s", issuer, config.Username),
			RawQuery: url.Values{"secret": []string{config.Secret}, "issuer": []string{issuer}}.Encode(),
		}
		ret := InitMFAResponse{
			MFAInitConfig: *config,
			URL:           otpauth.String(),
		}
		return ret, nil
	})
}

func (a *API) VerfiyMFA(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := &VerrifyMFAData{}
		if err := api.Body(r, data); err != nil {
			return nil, err
		}
		if err := a.Provider.VerifyMFA(ctx, session, *data); err != nil {
			return nil, err
		}
		return errors.NewOK(), nil
	})
}

func (a *API) RemoveMFA(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		data := &RemoveMFAOptions{}
		if err := api.Body(r, data); err != nil {
			return nil, err
		}
		if err := a.Provider.RemoveMFA(ctx, session, *data); err != nil {
			return nil, err
		}
		return errors.NewOK(), nil
	})
}

const (
	CaptchaTypeNone CaptchaProvider = "None"
	// CaptchaTypeGraphic is the normal graphic captcha
	// The user should input the captcha code to verify the login
	CaptchaTypeGraphic CaptchaProvider = "Graphic"
	// CaptchaTypeRecaptcha is the google recaptcha
	// The user should click the recaptcha to verify the login
	// client should send the recaptcha token to the server
	CaptchaTypeRecaptcha CaptchaProvider = "Recaptcha"
)

type CaptchaProvider string

// CaptchData is the data for the captcha result
// client should send the token to the server
type CaptchData struct {
	Name     string          `json:"name"`
	Provider CaptchaProvider `json:"provider"`
	Key      string          `json:"key"`
	Code     string          `json:"code"`
}

func (a *API) GetCaptcha(w http.ResponseWriter, r *http.Request) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		options := GetCaptchaConfigOption{
			Username: api.Query(r, "username", ""),
		}
		return a.Provider.GetCaptcha(ctx, options)
	})
}

func (a *API) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		keys, err := a.Provider.ListAPIKeys(ctx, session)
		if err != nil {
			return nil, err
		}
		return keys, nil
	})
}

func (a *API) GenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		options := GenerateAPIKeyOptions{}
		if err := api.Body(r, &options); err != nil {
			return nil, err
		}
		key, err := a.Provider.GenerateAPIKey(ctx, session, options)
		if err != nil {
			return nil, err
		}
		return key, nil
	})
}

func (a *API) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	a.OnSession(w, r, func(ctx context.Context, session string) (any, error) {
		id := api.Path(r, "accesskey", "")
		return nil, a.Provider.DeleteAPIKey(ctx, session, id)
	})
}

func (a *API) AuthProviderGroup() api.Group {
	return api.
		NewGroup("").
		Tag("Auth").
		Route(
			api.GET("/login-config").
				Operation("get login configuration").
				To(a.GetLoginConfig).
				Param(
					api.QueryParam("platform", "platform for the login configuration").In(
						"user", "admin", "app",
					),
				).
				Response(&LoginConfiguration{}),

			api.POST("/send-code").
				Operation("send OTP code").
				To(a.SendOTPCode).
				Param(api.BodyParam("send", SendOTPCode{})),

			api.GET("/captcha").
				Operation("get captcha").
				To(a.GetCaptcha).
				Param(api.QueryParam("username", "username for the captcha").Optional()).
				Response(&CaptchaConfig{}),

			api.GET("/current/profile").
				Operation("get current profile").
				To(a.GetCurrentProfile).
				Response(&UserProfile{}),

			api.PUT("/current/profile").
				Operation("update current profile").
				To(a.UpdateCurrentProfile).
				Param(api.BodyParam("data", UserProfile{})),

			api.POST("/current/reset-password").
				Operation("reset password").
				To(a.ResetPassword).
				Param(api.BodyParam("data", ResetPasswordData{})),

			api.POST("/current/reset-email").
				Operation("reset email").
				To(a.ResetEmail).
				Param(api.BodyParam("data", ResetEmailData{})),

			api.POST("/current/reset-phone").
				Operation("reset phone").
				To(a.ResetPhone).
				Param(api.BodyParam("data", ResetPhoneData{})),

			api.GET("/current/apikeys").
				Operation("get api keys").
				To(a.ListAPIKeys).
				Response(&[]APIKey{}),

			api.POST("/current/apikeys").
				Operation("generate api key").
				To(a.GenerateAPIKey).
				Param(
					api.BodyParam("options", GenerateAPIKeyOptions{}),
				).
				Response(&APIKey{}),

			api.DELETE("/current/apikeys/{accesskey}").
				Operation("delete api key").
				To(a.DeleteAPIKey),

			api.POST("/register").
				To(a.SignUp).
				Param(api.BodyParam("data", SignUpData{})).
				ResponseStatus(http.StatusOK, errors.NewOK()).
				ResponseStatus(http.StatusBadRequest, ErrorNeedCaptcha),

			api.POST("/login").
				Operation("sign in").
				To(a.SignIn).
				Param(api.BodyParam("login", LoginData{})).
				Response(LoginResponse{}).
				ResponseStatus(http.StatusOK,
					LoginResponse{Next: NextLoginTypeMFA, MFA: &MFAConfig{Provider: MFAProviderAPP, Enabled: true}},
					"user enabled mfa").
				ResponseStatus(http.StatusBadRequest, ErrorNeedCaptcha),

			api.POST("/verify-mfa").
				Operation("verify MFA").
				To(a.VerfiyMFA).
				Param(api.BodyParam("data", VerrifyMFAData{})),

			api.POST("/init-mfa").
				Operation("init MFA").
				To(a.InitMFA).
				Param(api.BodyParam("data", InitMFAOptions{})).
				Response(InitMFAResponse{}),

			api.POST("/remove-mfa").
				Operation("remove MFA").
				To(a.RemoveMFA).
				Param(api.BodyParam("data", RemoveMFAOptions{})),

			api.POST("/logout").
				Operation("sign out").
				To(a.SignOut),
		)
}
