package dto

type AppleLoginCallbackRequest struct {
	Code    string `form:"code"`
	IDToken string `form:"id_token"`
	State   string `form:"state"`
}

type AuthMethodConfig struct {
	Id      int64  `json:"id"`
	Method  string `json:"method"`
	Config  any    `json:"config"`
	Enabled bool   `json:"enabled"`
}

type BindOAuthCallbackRequest struct {
	Method   string `json:"method" validate:"required,oneof=google apple telegram github facebook"`
	Callback any    `json:"callback" validate:"required"`
}

type BindOAuthRequest struct {
	Method   string `json:"method" validate:"required,oneof=google apple telegram github facebook"`
	Redirect string `json:"redirect" validate:"required"`
}

type BindOAuthResponse struct {
	Redirect string `json:"redirect"`
}

type BindTelegramResponse struct {
	Url       string `json:"url"`
	ExpiredAt int64  `json:"expired_at"`
}

type CheckUserRequest struct {
	Email string `form:"email" validate:"required,email"`
}

type CheckUserResponse struct {
	Exist bool `json:"exist"`
}

type CheckVerificationCodeRequest struct {
	Method  string `json:"method" validate:"required,oneof=email mobile"`
	Account string `json:"account" validate:"required"`
	Code    string `json:"code" validate:"required"`
	Type    uint8  `json:"type" validate:"required,oneof=1 2"`
}

type CheckVerificationCodeResponse struct {
	Status bool `json:"status"`
} // @name dto.CheckVerificationCodeRespone

type DeviceLoginRequest struct {
	Identifier string `json:"identifier" validate:"required,max=255"`
	Invite     string `json:"invite"`
	CfToken    string `json:"cf_token"`
}

type GetAuthMethodConfigRequest struct {
	Method string `form:"method"`
}

type GetAuthMethodListResponse struct {
	List []AuthMethodConfig `json:"list"`
}

type GetOAuthMethodsResponse struct {
	Methods []UserAuthMethod `json:"methods"`
}

type LoginResponse struct {
	Token string `json:"token"`
	// ThirdPartyBindings lists the types of the third-party sign-in methods
	// (OAuth providers, Telegram) still bound to the account after a
	// password reset, so the client can show them: a binding made during a
	// compromise keeps signing in until its owner removes it. Empty for
	// every other sign-in.
	ThirdPartyBindings []string `json:"third_party_bindings,omitempty"`
}

type OAuthLoginRequest struct {
	Method   string `json:"method" validate:"required"` // google, facebook, apple, telegram, github etc.
	Redirect string `json:"redirect"`
	// Nonce is an optional random value the client generates and keeps (for
	// example in session storage) for this sign-in; it is presented again on
	// the token exchange, so a sign-in another browser started cannot be
	// completed in this one. At least 16 random characters are recommended.
	Nonce string `json:"nonce" validate:"max=128"`
} // @name dto.OAthLoginRequest

type OAuthLoginGetTokenRequest struct {
	Method   string `json:"method" validate:"required"` // google, facebook, apple, telegram, github etc.
	Callback any    `json:"callback" validate:"required"`
	Invite   string `json:"invite"`
	CfToken  string `json:"cf_token"`
	// Nonce is the value the client sent when it started this sign-in; a
	// sign-in started with a nonce is completed only with the same one, and
	// one started without a nonce only without one.
	Nonce string `json:"nonce" validate:"max=128"`
}

type OAuthLoginResponse struct {
	Redirect string `json:"redirect"`
}

type ResetPasswordRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,min=8,max=128"`
	Code       string `json:"code"`
	CfToken    string `json:"cf_token"`
}

type SendCodeRequest struct {
	Email string `json:"email" validate:"required,email"`
	Type  uint8  `json:"type" validate:"required,oneof=1 2"`
	// CfToken is the Turnstile response an anonymous request for a
	// registration code (type 1) carries while registration verification is
	// on; other requests leave it empty.
	CfToken string `json:"cf_token"`
}

type SendCodeResponse struct {
	Code   string `json:"code,omitempty"`
	Status bool   `json:"status"`
}

type SendSmsCodeRequest struct {
	Type              uint8  `json:"type" validate:"required,oneof=1 2"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	// CfToken is the Turnstile response an anonymous request for a
	// registration code (type 1) carries while registration verification is
	// on; other requests leave it empty.
	CfToken string `json:"cf_token"`
}

type TelephoneCheckUserRequest struct {
	Telephone         string `form:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
}

type TelephoneCheckUserResponse struct {
	Exist bool `json:"exist"`
}

type TelephoneLoginRequest struct {
	Identifier        string `json:"identifier"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneCode     string `json:"telephone_code"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	Password          string `json:"password"`
	CfToken           string `json:"cf_token"`
}

type TelephoneRegisterRequest struct {
	Identifier        string `json:"identifier"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	Password          string `json:"password" validate:"required,min=8,max=128"`
	Invite            string `json:"invite"`
	Code              string `json:"code"`
	CfToken           string `json:"cf_token"`
}

type TelephoneResetPasswordRequest struct {
	Identifier        string `json:"identifier"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	Password          string `json:"password" validate:"required,min=8,max=128"`
	Code              string `json:"code"`
	CfToken           string `json:"cf_token"`
}

type TestEmailSendRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type TestSmsSendRequest struct {
	AreaCode  string `json:"area_code" validate:"required"`
	Telephone string `json:"telephone" validate:"required"`
}

type UnbindOAuthRequest struct {
	Method string `json:"method"`
}

type UpdateAuthMethodConfigRequest struct {
	Id      int64  `json:"id"`
	Method  string `json:"method"`
	Config  any    `json:"config"`
	Enabled *bool  `json:"enabled"`
}

type UserLoginRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required"`
	CfToken    string `json:"cf_token"`
}

type UserRegisterRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,min=8,max=128"`
	Invite     string `json:"invite"`
	Code       string `json:"code"`
	CfToken    string `json:"cf_token"`
}

type VerifyEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
	Code  string `json:"code" validate:"required"`
}
