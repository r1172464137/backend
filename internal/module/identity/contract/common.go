// Package dto holds the identity module's contract: the request and
// response types of its facade and HTTP handlers. Their JSON and Swagger
// names are the public API.
package dto

type GetDetailRequest struct {
	Id int64 `form:"id" validate:"required"`
}

type GetDeviceListResponse struct {
	List  []UserDevice `json:"list"`
	Total int64        `json:"total"`
}

type User struct {
	Id                    int64            `json:"id"`
	Avatar                string           `json:"avatar"`
	Balance               int64            `json:"balance"`
	Commission            int64            `json:"commission"`
	ReferralPercentage    uint8            `json:"referral_percentage" validate:"lte=100"`
	OnlyFirstPurchase     bool             `json:"only_first_purchase"`
	GiftAmount            int64            `json:"gift_amount"`
	Telegram              int64            `json:"telegram"`
	ReferCode             string           `json:"refer_code"`
	RefererId             int64            `json:"referer_id"`
	Enable                bool             `json:"enable"`
	IsAdmin               bool             `json:"is_admin,omitempty"`
	EnableBalanceNotify   bool             `json:"enable_balance_notify"`
	EnableLoginNotify     bool             `json:"enable_login_notify"`
	EnableSubscribeNotify bool             `json:"enable_subscribe_notify"`
	EnableTradeNotify     bool             `json:"enable_trade_notify"`
	AuthMethods           []UserAuthMethod `json:"auth_methods"`
	UserDevices           []UserDevice     `json:"user_devices"`
	Rules                 []string         `json:"rules"`
	CreatedAt             int64            `json:"created_at"`
	UpdatedAt             int64            `json:"updated_at"`
	DeletedAt             int64            `json:"deleted_at,omitempty"`
}
