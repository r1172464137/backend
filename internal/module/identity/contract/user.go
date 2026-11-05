package dto

type BatchDeleteUserRequest struct {
	Ids []int64 `json:"ids" validate:"required"`
}

type CreateUserAuthMethodRequest struct {
	UserId         int64  `json:"user_id"`
	AuthType       string `json:"auth_type"`
	AuthIdentifier string `json:"auth_identifier"`
}

type CreateUserRequest struct {
	Email              string `json:"email"`
	Telephone          string `json:"telephone"`
	TelephoneAreaCode  string `json:"telephone_area_code"`
	Password           string `json:"password"`
	ProductId          int64  `json:"product_id"`
	Duration           int64  `json:"duration"`
	ReferralPercentage uint8  `json:"referral_percentage" validate:"lte=100"`
	OnlyFirstPurchase  bool   `json:"only_first_purchase"`
	RefererUser        string `json:"referer_user"`
	ReferCode          string `json:"refer_code"`
	Balance            int64  `json:"balance"`
	Commission         int64  `json:"commission"`
	GiftAmount         int64  `json:"gift_amount"`
	IsAdmin            bool   `json:"is_admin"`
}

type DeleteUserAuthMethodRequest struct {
	UserId   int64  `json:"user_id"`
	AuthType string `json:"auth_type"`
}

type DeleteUserDeviceRequest struct {
	Id int64 `json:"id"`
} // @name dto.DeleteUserDeivceRequest

type GetUserAuthMethodRequest struct {
	UserId int64 `json:"user_id"`
}

type GetUserAuthMethodResponse struct {
	AuthMethods []UserAuthMethod `json:"auth_methods"`
}

type GetUserListRequest struct {
	Page               int    `form:"page" validate:"required,gt=0"`
	Size               int    `form:"size" validate:"required,gt=0,lte=100"`
	Search             string `form:"search,omitempty"`
	UserId             *int64 `form:"user_id,omitempty"`
	Unscoped           bool   `form:"unscoped,omitempty"`
	SubscribeId        *int64 `form:"subscribe_id,omitempty"`
	UserSubscribeId    *int64 `form:"user_subscribe_id,omitempty"`
	UserSubscribeToken string `form:"user_subscribe_token,omitempty"`
}

type GetUserListResponse struct {
	Total int64  `json:"total"`
	List  []User `json:"list"`
}

type KickOfflineRequest struct {
	Id int64 `json:"id"`
}

type UnbindDeviceRequest struct {
	Id int64 `json:"id" validate:"required"`
}

type UpdateBindEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
	// Code is the register-type verification code sent to Email.
	Code string `json:"code" validate:"required"`
	// Password is the account's current password. Replacing an email the
	// account already has requires it when the account has a password; a
	// first binding does not.
	Password string `json:"password" validate:"max=128"`
	// CurrentCode is the security-type verification code sent to the email
	// the account already has. Replacing it requires the code when the
	// account has no password.
	CurrentCode string `json:"current_code"`
}

type UpdateBindMobileRequest struct {
	AreaCode string `json:"area_code" validate:"required"`
	Mobile   string `json:"mobile" validate:"required"`
	// Code is the register-type verification code sent to Mobile.
	Code string `json:"code" validate:"required"`
	// Password is the account's current password. Replacing a number the
	// account already has requires it when the account has a password; a
	// first binding does not.
	Password string `json:"password" validate:"max=128"`
	// CurrentCode is the security-type verification code sent to the number
	// the account already has. Replacing it requires the code when the
	// account has no password.
	CurrentCode string `json:"current_code"`
}

type UpdateUserAuthMethodRequest struct {
	UserId         int64  `json:"user_id"`
	AuthType       string `json:"auth_type"`
	AuthIdentifier string `json:"auth_identifier"`
}

type UpdateUserBasicInfoRequest struct {
	UserId   int64  `json:"user_id" validate:"required"`
	Password string `json:"password"`
	Avatar   string `json:"avatar"`
	// Balance, Commission and GiftAmount are wallet amounts to set; one
	// left out of the request leaves that amount as it is, so a client
	// sending only what the administrator edited cannot revert the money
	// movements made since the form was loaded.
	Balance            *int64 `json:"balance"`
	Commission         *int64 `json:"commission"`
	ReferralPercentage uint8  `json:"referral_percentage" validate:"lte=100"`
	OnlyFirstPurchase  bool   `json:"only_first_purchase"`
	GiftAmount         *int64 `json:"gift_amount"`
	Telegram           int64  `json:"telegram"`
	ReferCode          string `json:"refer_code"`
	RefererId          int64  `json:"referer_id"`
	Enable             bool   `json:"enable"`
	IsAdmin            bool   `json:"is_admin"`
} // @name dto.UpdateUserBasiceInfoRequest

type UpdateUserNotifyRequest struct {
	EnableBalanceNotify   *bool `json:"enable_balance_notify"`
	EnableLoginNotify     *bool `json:"enable_login_notify"`
	EnableSubscribeNotify *bool `json:"enable_subscribe_notify"`
	EnableTradeNotify     *bool `json:"enable_trade_notify"`
}

type UpdateUserNotifySettingRequest struct {
	UserId                int64 `json:"user_id" validate:"required"`
	EnableBalanceNotify   bool  `json:"enable_balance_notify"`
	EnableLoginNotify     bool  `json:"enable_login_notify"`
	EnableSubscribeNotify bool  `json:"enable_subscribe_notify"`
	EnableTradeNotify     bool  `json:"enable_trade_notify"`
}

type UpdateUserPasswordRequest struct {
	// OldPassword is required once the account has a password.
	OldPassword string `json:"old_password" validate:"max=128"`
	Password    string `json:"password" validate:"required,min=8,max=128"`
	// CurrentCode is the security-type verification code sent to the email
	// or phone number the account has bound. Setting the first password of
	// an account that has one bound requires it; an account with neither a
	// password nor a bound email or phone number (OAuth or device sign-in
	// only) sets its first password without it.
	CurrentCode string `json:"current_code"`
}

// UpdateUserPasswordResponse reports what a password change leaves in place.
type UpdateUserPasswordResponse struct {
	// ThirdPartyBindings lists the types of the third-party sign-in methods
	// (OAuth providers, Telegram) still bound to the account, so the client
	// can show them: a binding made during a compromise keeps signing in
	// until its owner removes it.
	ThirdPartyBindings []string `json:"third_party_bindings"`
}

type UpdateUserRulesRequest struct {
	Rules []string `json:"rules" validate:"required"`
}

type UserAuthMethod struct {
	AuthType       string `json:"auth_type"`
	AuthIdentifier string `json:"auth_identifier"`
	Verified       bool   `json:"verified"`
}

type UserDevice struct {
	Id         int64  `json:"id"`
	Ip         string `json:"ip"`
	Identifier string `json:"identifier"`
	UserAgent  string `json:"user_agent"`
	Online     bool   `json:"online"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}
