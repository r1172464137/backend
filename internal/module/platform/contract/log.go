package dto

// AdminActionLog is one administrator mutation: what was done to which
// object, by whom (UserId is the administrator's account) and from where
// (Source "http" carries the request's address and agent; "telegram" the
// bot command's sender).
type AdminActionLog struct {
	Id               int64  `json:"id"`
	UserId           int64  `json:"user_id"`
	Action           string `json:"action"`
	Object           string `json:"object,omitempty"`
	ObjectId         int64  `json:"object_id,omitempty"`
	Detail           string `json:"detail,omitempty"`
	Source           string `json:"source"`
	TelegramSenderId int64  `json:"telegram_sender_id,omitempty"`
	Timestamp        int64  `json:"timestamp"`
	CreatedAt        int64  `json:"created_at"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type FilterAdminActionLogRequest struct {
	FilterLogParams
	// UserId narrows the trail to one administrator.
	UserId int64 `form:"user_id"`
}

type FilterAdminActionLogResponse struct {
	Total int64            `json:"total"`
	List  []AdminActionLog `json:"list"`
}

// UnmatchedPaymentLog is a payment a gateway confirmed that could not settle
// its order; UserId is the payer when known.
type UnmatchedPaymentLog struct {
	Id               int64  `json:"id"`
	UserId           int64  `json:"user_id"`
	OrderNo          string `json:"order_no"`
	TradeNo          string `json:"trade_no"`
	Platform         string `json:"platform"`
	Amount           int64  `json:"amount"`
	Currency         string `json:"currency"`
	Reason           string `json:"reason"`
	Timestamp        int64  `json:"timestamp"`
	CreatedAt        int64  `json:"created_at"`
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

type FilterUnmatchedPaymentLogRequest struct {
	FilterLogParams
	// UserId narrows the list to one payer.
	UserId int64 `form:"user_id"`
}

type FilterUnmatchedPaymentLogResponse struct {
	Total int64                 `json:"total"`
	List  []UnmatchedPaymentLog `json:"list"`
}

type BalanceLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	Amount           int64  `json:"amount"`
	OrderNo          string `json:"order_no,omitempty"`
	Balance          int64  `json:"balance"`
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

type CommissionLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	Amount           int64  `json:"amount"`
	OrderNo          string `json:"order_no"`
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

type FilterBalanceLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id"`
}

type FilterBalanceLogResponse struct {
	Total int64        `json:"total"`
	List  []BalanceLog `json:"list"`
}

type FilterCommissionLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id"`
}

type FilterCommissionLogResponse struct {
	Total int64           `json:"total"`
	List  []CommissionLog `json:"list"`
}

type FilterEmailLogResponse struct {
	Total int64        `json:"total"`
	List  []MessageLog `json:"list"`
}

type FilterGiftLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id"`
}

type FilterGiftLogResponse struct {
	Total int64     `json:"total"`
	List  []GiftLog `json:"list"`
}

type FilterLogParams struct {
	Page      int    `form:"page" validate:"required,gt=0"`
	Size      int    `form:"size" validate:"required,gt=0,lte=100"`
	Date      string `form:"date"`
	StartDate string `form:"start_date" validate:"omitempty,datetime=2006-01-02"`
	EndDate   string `form:"end_date" validate:"omitempty,datetime=2006-01-02"`
	Search    string `form:"search"`
}

type FilterLoginLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id"`
}

type FilterLoginLogResponse struct {
	Total int64      `json:"total"`
	List  []LoginLog `json:"list"`
}

type FilterMobileLogResponse struct {
	Total int64        `json:"total"`
	List  []MessageLog `json:"list"`
}

type FilterOrderLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id"`
}

type FilterOrderLogResponse struct {
	Total int64      `json:"total"`
	List  []OrderLog `json:"list"`
}

type FilterRegisterLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id"`
}

type FilterRegisterLogResponse struct {
	Total int64         `json:"total"`
	List  []RegisterLog `json:"list"`
}

type FilterResetSubscribeLogRequest struct {
	FilterLogParams
	UserSubscribeId int64 `form:"user_subscribe_id"`
}

type FilterResetSubscribeLogResponse struct {
	Total int64               `json:"total"`
	List  []ResetSubscribeLog `json:"list"`
}

type FilterSubscribeLogRequest struct {
	FilterLogParams
	UserId          int64 `form:"user_id"`
	UserSubscribeId int64 `form:"user_subscribe_id"`
}

type FilterSubscribeLogResponse struct {
	Total int64          `json:"total"`
	List  []SubscribeLog `json:"list"`
}

type GetMessageLogListRequest struct {
	Page   int    `form:"page" validate:"required,gt=0"`
	Size   int    `form:"size" validate:"required,gt=0,lte=100"`
	Type   uint8  `form:"type" validate:"required,oneof=10 11"`
	Search string `form:"search"`
}

type GetMessageLogListResponse struct {
	Total int64        `json:"total"`
	List  []MessageLog `json:"list"`
}

type GiftLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	OrderNo          string `json:"order_no"`
	SubscribeId      int64  `json:"subscribe_id"`
	Amount           int64  `json:"amount"`
	Balance          int64  `json:"balance"`
	Remark           string `json:"remark,omitempty"`
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

type LogResponse struct {
	List any `json:"list"`
}

// LogSetting is the log retention: ClearDays may not go below seven days,
// so the login, registration and subscription logs outlive the time it
// takes to notice an incident.
type LogSetting struct {
	AutoClear *bool `json:"auto_clear" validate:"required"`
	ClearDays int64 `json:"clear_days" validate:"required,gte=7,lte=3650"`
}

type LoginLog struct {
	UserId           int64  `json:"user_id"`
	Method           string `json:"method"`
	LoginIP          string `json:"login_ip"`
	UserAgent        string `json:"user_agent"`
	Success          bool   `json:"success"`
	Timestamp        int64  `json:"timestamp"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type MessageLog struct {
	Id               int64  `json:"id"`
	Type             uint8  `json:"type"`
	Platform         string `json:"platform"`
	To               string `json:"to"`
	Subject          string `json:"subject"`
	Content          any    `json:"content"`
	Status           uint8  `json:"status"`
	CreatedAt        int64  `json:"created_at"`
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

type OrderLog struct {
	Id               int64  `json:"id"`
	UserId           int64  `json:"user_id"`
	OrderNo          string `json:"order_no"`
	OrderType        uint8  `json:"order_type"`
	Quantity         int64  `json:"quantity"`
	Price            int64  `json:"price"`
	Amount           int64  `json:"amount"`
	GiftAmount       int64  `json:"gift_amount"`
	Discount         int64  `json:"discount"`
	CouponDiscount   int64  `json:"coupon_discount"`
	PaymentId        int64  `json:"payment_id"`
	Method           string `json:"method"`
	FeeAmount        int64  `json:"fee_amount"`
	SubscribeId      int64  `json:"subscribe_id,omitempty"`
	Source           string `json:"source"`
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

type RegisterLog struct {
	UserId           int64  `json:"user_id"`
	AuthMethod       string `json:"auth_method"`
	Identifier       string `json:"identifier"`
	RegisterIP       string `json:"register_ip"`
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

type ResetSubscribeLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
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
