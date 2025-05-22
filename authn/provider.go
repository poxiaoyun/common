package authn

import (
	"context"
	"time"

	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

type Provider interface {
	AuthProvider
	UserProvider
}

type Session struct {
	Value   string
	Expires time.Time
}

// CaptchaConfig is the configuration for the captcha
// client render the captcha based on the configuration
// and generate the token for the captcha
// client should send the token to the server for verification
type CaptchaConfig struct {
	// Name     string          `json:"name"`
	Provider CaptchaProvider `json:"provider"`
	Action   string          `json:"action"`
	Key      string          `json:"key"`
	// Params is the additional parameters for the captcha
	Params map[string]string `json:"params"`
}

type OTPType string

const (
	OTPTypeEmail OTPType = "Email"
	OtpTypePhone OTPType = "Phone"
)

type OTPAction string

const (
	OPTActionLogin    OTPAction = "login"
	OPTActionRegister OTPAction = "register"
	OPTActionReset    OTPAction = "reset"
)

type SendOTPCode struct {
	// Username is the username for the OTP code
	Username string `json:"username"`
	// Type email or phone
	Type OTPType `json:"type"`
	// Target is the target for the OTP code
	// The target could be the `email` or `phone number`
	Target string `json:"target"`
	// Action is the action for the OTP code request
	// The action could be `login`, `register`, `reset`
	Action OTPAction `json:"action"`
	// Captcha is the captcha for this request
	Captcha CaptchData `json:"captcha"`
}

type GetCaptchaConfigOption struct {
	Username string `json:"username"`
}

type MFAProvider string

const (
	MFAProviderAPP MFAProvider = "app"
)

type MFAInitConfig struct {
	Provider MFAProvider `json:"provider,omitempty"`
	Username string      `json:"username,omitempty"`
	Secret   string      `json:"secret,omitempty"`
	// RecoveryCodes allow the user to recover the MFA if the user lost the device
	RecoveryCodes []string          `json:"recoveryCodes,omitempty"`
	Params        map[string]string `json:"params,omitempty"`
}

type InitMFAOptions struct {
	Provider MFAProvider `json:"provider"`
	Password string      `json:"password"`
}

type BindMFAOptions struct {
	Code string `json:"code"`
}

type OauthCallbackData struct {
	Code     string `json:"code,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type RemoveMFAOptions struct {
	Provider MFAProvider `json:"provider"`
}

type GetConfigurationOptions struct {
	// Platform is the platform for the login configuration
	// The platform could be `user`, `admin`, `app`
	Platform string `json:"platform,omitempty"`
}

type AuthProvider interface {
	CheckSession(ctx context.Context, session string) (*Session, error)
	GetConfiguration(ctx context.Context, options GetConfigurationOptions) (*LoginConfiguration, error)

	SendOTPCode(ctx context.Context, config SendOTPCode) error
	GetCaptcha(ctx context.Context, option GetCaptchaConfigOption) (*CaptchaConfig, error)

	VerifyMFA(ctx context.Context, session string, data VerrifyMFAData) error
	InitMFA(ctx context.Context, session string, config InitMFAOptions) (*MFAInitConfig, error)
	RemoveMFA(ctx context.Context, session string, config RemoveMFAOptions) error

	Signin(ctx context.Context, session string, config LoginData) (*LoginResponse, error)
	Signout(ctx context.Context, session string) error
	Signup(ctx context.Context, session string, data SignUpData) error

	ResetPassword(ctx context.Context, session string, data ResetPasswordData) error
	ResetEmail(ctx context.Context, session string, data ResetEmailData) error
	ResetPhone(ctx context.Context, session string, data ResetPhoneData) error

	GetCurrentProfile(ctx context.Context, session string) (*UserProfile, error)
	// UpdateProfile updates the profile of the user
	// some fields are not allowed to update by the user should be ignored
	UpdateCurrentProfile(ctx context.Context, session string, data UserProfile) error

	LoginAsUser(ctx context.Context, username string) (*Session, error)

	GenerateAPIKey(ctx context.Context, session string, options GenerateAPIKeyOptions) (*APIKey, error)
	ListAPIKeys(ctx context.Context, session string) ([]APIKey, error)
	DeleteAPIKey(ctx context.Context, session string, accesskey string) error

	CheckAPIKey(ctx context.Context, key APIKey) (*User, error)
}

type GenerateAPIKeyOptions struct {
	Name    string    `json:"name,omitempty"`
	Expires time.Time `json:"expires,omitempty"`
}

type APIKey struct {
	Name      string `json:"name,omitempty"`
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

type ListUserOptions struct {
	api.ListOptions
}

type User struct {
	store.ObjectMeta `json:",inline"`
	// Subject
	Subject       string   `json:"subject,omitempty"`
	DisplayName   string   `json:"displayName,omitempty"`
	Email         string   `json:"email,omitempty"`
	EmailVerified bool     `json:"emailVerified,omitempty"`
	Phone         string   `json:"phone,omitempty"`
	PhoneVerified bool     `json:"phoneVerified,omitempty"`
	Groups        []string `json:"groups,omitempty"`
}

type UserProfile struct {
	User       `json:",inline"`
	FirstName  string    `json:"firstName,omitempty"`
	MiddleName string    `json:"middleName,omitempty"`
	LastName   string    `json:"lastName,omitempty"`
	Picture    string    `json:"picture,omitempty"`
	Website    string    `json:"website,omitempty"`
	Birthdate  string    `json:"birthdate,omitempty"`
	Company    string    `json:"company,omitempty"`
	Gender     string    `json:"gender,omitempty"`
	Country    string    `json:"country,omitempty"`
	State      string    `json:"state,omitempty"`
	City       string    `json:"city,omitempty"`
	Street     string    `json:"street,omitempty"`
	PostalCode string    `json:"postalCode,omitempty"`
	Languages  []string  `json:"languages,omitempty"`
	MFA        MFAConfig `json:"mfa,omitempty"`
}

type UserProvider interface {
	CountUsers(ctx context.Context) (int, error)

	ListUsers(ctx context.Context, options ListUserOptions) (api.Page[UserProfile], error)
	CreateUser(ctx context.Context, user *UserProfile, password string) error
	GetUser(ctx context.Context, username string) (*UserProfile, error)
	UpdateUser(ctx context.Context, data *UserProfile) error
	DeleteUser(ctx context.Context, username string) (*UserProfile, error)

	GetUserProfile(ctx context.Context, username string) (*UserProfile, error)
	UpdateUserProfile(ctx context.Context, profile *UserProfile) error

	SetUserDisabled(ctx context.Context, username string, disabled bool) error
	SetUserPassword(ctx context.Context, username, password string) error
}

type API struct {
	Provider Provider
}

func NewAPI(provider Provider) *API {
	return &API{Provider: provider}
}

func (a *API) PublicGroup() api.Group {
	return api.
		NewGroup("").
		SubGroup(
			a.AuthProviderGroup(),
		)
}

func (a *API) Group() api.Group {
	return api.
		NewGroup("").
		SubGroup(
			a.UserProviderGroup(),
		)
}
