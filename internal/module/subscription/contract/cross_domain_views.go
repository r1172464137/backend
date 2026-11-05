package dto

// Cross-domain view types are module-owned snapshots. They deliberately
// duplicate JSON shapes instead of coupling one module's API to another.
type CreateUserSubscribeRequest struct {
	UserId      int64 `json:"user_id"`
	ExpiredAt   int64 `json:"expired_at"`
	Traffic     int64 `json:"traffic"`
	SubscribeId int64 `json:"subscribe_id"`
}

type DeleteUserSubscribeRequest struct {
	UserSubscribeId int64 `json:"user_subscribe_id"`
}

type GetUserSubscribeByIdRequest struct {
	Id int64 `form:"id" validate:"required"`
}

type GetUserSubscribeDevicesRequest struct {
	Page        int   `form:"page" validate:"required,gt=0"`
	Size        int   `form:"size" validate:"required,gt=0,lte=100"`
	UserId      int64 `form:"user_id"`
	SubscribeId int64 `form:"subscribe_id"`
}

type GetUserSubscribeDevicesResponse struct {
	List  []SubscriptionUserDeviceSnapshot `json:"list"`
	Total int64                            `json:"total"`
}

type GetUserSubscribeListRequest struct {
	Page   int   `form:"page" validate:"required,gt=0"`
	Size   int   `form:"size" validate:"required,gt=0,lte=100"`
	UserId int64 `form:"user_id"`
}

type GetUserSubscribeListResponse struct {
	List  []UserSubscribe `json:"list"`
	Total int64           `json:"total"`
}

type GetUserSubscribeLogsRequest struct {
	Page        int   `form:"page" validate:"required,gt=0"`
	Size        int   `form:"size" validate:"required,gt=0,lte=100"`
	UserId      int64 `form:"user_id"`
	SubscribeId int64 `form:"subscribe_id,omitempty"`
}

type GetUserSubscribeLogsResponse struct {
	List  []UserSubscribeLog `json:"list"`
	Total int64              `json:"total"`
}

type GetUserSubscribeResetTrafficLogsRequest struct {
	Page            int   `form:"page" validate:"required,gt=0"`
	Size            int   `form:"size" validate:"required,gt=0,lte=100"`
	UserSubscribeId int64 `form:"user_subscribe_id"`
}

type GetUserSubscribeResetTrafficLogsResponse struct {
	List  []ResetSubscribeTrafficLog `json:"list"`
	Total int64                      `json:"total"`
}

type GetUserSubscribeTrafficLogsRequest struct {
	Page        int   `form:"page" validate:"required,gt=0"`
	Size        int   `form:"size" validate:"required,gt=0,lte=100"`
	UserId      int64 `form:"user_id"`
	SubscribeId int64 `form:"subscribe_id"`
	StartTime   int64 `form:"start_time"`
	EndTime     int64 `form:"end_time"`
}

type GetUserSubscribeTrafficLogsResponse struct {
	List  []TrafficLog `json:"list"`
	Total int64        `json:"total"`
}

type QueryUserSubscribeListResponse struct {
	List  []UserSubscribe `json:"list"`
	Total int64           `json:"total"`
}

type QueryUserSubscribeNodeListResponse struct {
	List []UserSubscribeInfo `json:"list"`
}

type ResetSubscribeTrafficLog struct {
	Id               int64  `json:"id"`
	Type             uint16 `json:"type"`
	UserSubscribeId  int64  `json:"user_subscribe_id"`
	OrderNo          string `json:"order_no,omitempty"`
	Timestamp        int64  `json:"timestamp"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type ResetUserSubscribeTokenRequest struct {
	UserSubscribeId int64 `json:"user_subscribe_id"`
}

type ResetUserSubscribeTrafficRequest struct {
	UserSubscribeId int64 `json:"user_subscribe_id"`
}

type SortItem struct {
	Id   int64 `json:"id" validate:"required"`
	Sort int64 `json:"sort" validate:"required"`
}

type SubscribeDiscount struct {
	Quantity int64   `json:"quantity"`
	Discount float64 `json:"discount"`
}

type SubscribeSortRequest struct {
	Sort []SortItem `json:"sort"`
}

type ToggleUserSubscribeStatusRequest struct {
	UserSubscribeId int64 `json:"user_subscribe_id"`
}

type TrafficLog struct {
	Id          int64 `json:"id"`
	ServerId    int64 `json:"server_id"`
	UserId      int64 `json:"user_id"`
	SubscribeId int64 `json:"subscribe_id"`
	Download    int64 `json:"download"`
	Upload      int64 `json:"upload"`
	Timestamp   int64 `json:"timestamp"`
}

type UpdateUserSubscribeNoteRequest struct {
	UserSubscribeId int64  `json:"user_subscribe_id" validate:"required"`
	Note            string `json:"note" validate:"max=500"`
}

type UpdateUserSubscribeRequest struct {
	UserSubscribeId int64 `json:"user_subscribe_id"`
	SubscribeId     int64 `json:"subscribe_id"`
	Traffic         int64 `json:"traffic"`
	ExpiredAt       int64 `json:"expired_at"`
	Upload          int64 `json:"upload"`
	Download        int64 `json:"download"`
}

type SubscriptionUserSnapshot struct {
	Id                    int64                                `json:"id"`
	Avatar                string                               `json:"avatar"`
	Balance               int64                                `json:"balance"`
	Commission            int64                                `json:"commission"`
	ReferralPercentage    uint8                                `json:"referral_percentage"`
	OnlyFirstPurchase     bool                                 `json:"only_first_purchase"`
	GiftAmount            int64                                `json:"gift_amount"`
	Telegram              int64                                `json:"telegram"`
	ReferCode             string                               `json:"refer_code"`
	RefererId             int64                                `json:"referer_id"`
	Enable                bool                                 `json:"enable"`
	IsAdmin               bool                                 `json:"is_admin,omitempty"`
	EnableBalanceNotify   bool                                 `json:"enable_balance_notify"`
	EnableLoginNotify     bool                                 `json:"enable_login_notify"`
	EnableSubscribeNotify bool                                 `json:"enable_subscribe_notify"`
	EnableTradeNotify     bool                                 `json:"enable_trade_notify"`
	AuthMethods           []SubscriptionUserAuthMethodSnapshot `json:"auth_methods"`
	UserDevices           []SubscriptionUserDeviceSnapshot     `json:"user_devices"`
	Rules                 []string                             `json:"rules"`
	CreatedAt             int64                                `json:"created_at"`
	UpdatedAt             int64                                `json:"updated_at"`
	DeletedAt             int64                                `json:"deleted_at,omitempty"`
} // @name dto.User

type SubscriptionUserAuthMethodSnapshot struct {
	AuthType       string `json:"auth_type"`
	AuthIdentifier string `json:"auth_identifier"`
	Verified       bool   `json:"verified"`
} // @name dto.UserAuthMethod

type SubscriptionUserDeviceSnapshot struct {
	Id         int64  `json:"id"`
	Ip         string `json:"ip"`
	Identifier string `json:"identifier"`
	UserAgent  string `json:"user_agent"`
	Online     bool   `json:"online"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
} // @name dto.UserDevice

type UserSubscribe struct {
	EntitlementSource string `json:"entitlement_source,omitempty"`

	Id          int64     `json:"id"`
	UserId      int64     `json:"user_id"`
	OrderId     int64     `json:"order_id"`
	SubscribeId int64     `json:"subscribe_id"`
	Subscribe   Subscribe `json:"subscribe"`
	StartTime   int64     `json:"start_time"`
	ExpireTime  int64     `json:"expire_time"`
	FinishedAt  int64     `json:"finished_at"`
	ResetTime   int64     `json:"reset_time"`
	Traffic     int64     `json:"traffic"`
	Download    int64     `json:"download"`
	Upload      int64     `json:"upload"`
	Token       string    `json:"token"`
	Status      uint8     `json:"status"`
	Short       string    `json:"short"`
	CreatedAt   int64     `json:"created_at"`
	UpdatedAt   int64     `json:"updated_at"`
}

type UserSubscribeDetail struct {
	EntitlementSource string `json:"entitlement_source,omitempty"`

	Id          int64                    `json:"id"`
	UserId      int64                    `json:"user_id"`
	User        SubscriptionUserSnapshot `json:"user"`
	OrderId     int64                    `json:"order_id"`
	SubscribeId int64                    `json:"subscribe_id"`
	Subscribe   Subscribe                `json:"subscribe"`
	StartTime   int64                    `json:"start_time"`
	ExpireTime  int64                    `json:"expire_time"`
	ResetTime   int64                    `json:"reset_time"`
	Traffic     int64                    `json:"traffic"`
	Download    int64                    `json:"download"`
	Upload      int64                    `json:"upload"`
	Token       string                   `json:"token"`
	Status      uint8                    `json:"status"`
	CreatedAt   int64                    `json:"created_at"`
	UpdatedAt   int64                    `json:"updated_at"`
}

type UserSubscribeInfo struct {
	EntitlementSource string `json:"entitlement_source,omitempty"`

	Id          int64                    `json:"id"`
	UserId      int64                    `json:"user_id"`
	OrderId     int64                    `json:"order_id"`
	SubscribeId int64                    `json:"subscribe_id"`
	StartTime   int64                    `json:"start_time"`
	ExpireTime  int64                    `json:"expire_time"`
	FinishedAt  int64                    `json:"finished_at"`
	ResetTime   int64                    `json:"reset_time"`
	Traffic     int64                    `json:"traffic"`
	Download    int64                    `json:"download"`
	Upload      int64                    `json:"upload"`
	Token       string                   `json:"token"`
	Status      uint8                    `json:"status"`
	CreatedAt   int64                    `json:"created_at"`
	UpdatedAt   int64                    `json:"updated_at"`
	IsTryOut    bool                     `json:"is_try_out"`
	Nodes       []*UserSubscribeNodeInfo `json:"nodes"`
}

type UserSubscribeLog struct {
	Id               int64  `json:"id"`
	UserId           int64  `json:"user_id"`
	UserSubscribeId  int64  `json:"user_subscribe_id"`
	Token            string `json:"token"`
	IP               string `json:"ip"`
	UserAgent        string `json:"user_agent"`
	Timestamp        int64  `json:"timestamp"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type UserSubscribeNodeInfo struct {
	Id        int64    `json:"id"`
	Name      string   `json:"name"`
	Uuid      string   `json:"uuid"`
	Protocol  string   `json:"protocol"`
	Port      uint16   `json:"port"`
	Address   string   `json:"address"`
	Tags      []string `json:"tags"`
	Country   string   `json:"country"`
	City      string   `json:"city"`
	CreatedAt int64    `json:"created_at"`
}
