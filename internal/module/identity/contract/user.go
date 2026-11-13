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
	ReferralPercentage uint8  `json:"referral_percentage"`
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

type DeleteUserDeivceRequest struct {
	Id int64 `json:"id"`
}

type GetUserAuthMethodRequest struct {
	UserId int64 `json:"user_id"`
}

type GetUserAuthMethodResponse struct {
	AuthMethods []UserAuthMethod `json:"auth_methods"`
}

type GetUserListRequest struct {
	Page               int     `form:"page" validate:"required,gt=0"`
	Size               int     `form:"size" validate:"required,gt=0,lte=100"`
	Search             string  `form:"search,omitempty"`
	UserId             *int64  `form:"user_id,omitempty"`
	Ids                []int64 `form:"ids,omitempty" validate:"omitempty,max=100" collectionFormat:"multi"`
	Unscoped           bool    `form:"unscoped,omitempty"`
	SubscribeId        *int64  `form:"subscribe_id,omitempty"`
	UserSubscribeId    *int64  `form:"user_subscribe_id,omitempty"`
	UserSubscribeToken string  `form:"user_subscribe_token,omitempty"`
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
}

type UpdateBindMobileRequest struct {
	AreaCode string `json:"area_code" validate:"required"`
	Mobile   string `json:"mobile" validate:"required"`
	Code     string `json:"code" validate:"required"`
}

type UpdateUserAuthMethodRequest struct {
	UserId         int64  `json:"user_id"`
	AuthType       string `json:"auth_type"`
	AuthIdentifier string `json:"auth_identifier"`
}

type UpdateUserBasiceInfoRequest struct {
	UserId             int64  `json:"user_id" validate:"required"`
	Password           string `json:"password"`
	Avatar             string `json:"avatar"`
	Balance            int64  `json:"balance"`
	Commission         int64  `json:"commission"`
	ReferralPercentage uint8  `json:"referral_percentage"`
	OnlyFirstPurchase  bool   `json:"only_first_purchase"`
	GiftAmount         int64  `json:"gift_amount"`
	Telegram           int64  `json:"telegram"`
	ReferCode          string `json:"refer_code"`
	RefererId          int64  `json:"referer_id"`
	Enable             bool   `json:"enable"`
	IsAdmin            bool   `json:"is_admin"`
}

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
	Password string `json:"password" validate:"required,min=8,max=128"`
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
