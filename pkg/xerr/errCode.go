package xerr

/** (The first 3 digits represent the business, and the last three digits represent the specific function) **/

// Domain bands for NEW error codes (ADR-001: modules evolve toward separate
// services, so codes must not collide across independently-released
// domains). Every code introduced from 2026-07-25 on must be allocated
// inside its owning module's band; the pre-existing codes below are frozen
// where they are — clients switch on the numeric values, renumbering is a
// breaking change. TestErrorCodeSegmentation enforces both rules.
const (
	BandShared       uint32 = 100000 // cross-cutting: transport, database, generic
	BandIdentity     uint32 = 110000 // accounts, auth, devices, verification
	BandBilling      uint32 = 120000 // orders, payments, coupons, wallet
	BandSubscription uint32 = 130000 // plans, user subscriptions, quota
	BandNetwork      uint32 = 140000 // servers, nodes, traffic
	BandSupport      uint32 = 150000 // tickets, announcements, ads, documents
	BandPlatform     uint32 = 160000 // system settings, tasks, logs
	BandNotification uint32 = 170000 // telegram, broadcast channels
	bandWidth        uint32 = 10000  // each band spans [Band, Band+bandWidth)
)

// General error code
const (
	SUCCESS uint32 = 200
	ERROR   uint32 = 500
)

// Database error
const (
	DatabaseQueryError   uint32 = 10001
	DatabaseUpdateError  uint32 = 10002
	DatabaseInsertError  uint32 = 10003
	DatabaseDeletedError uint32 = 10004
)

// User error
const (
	UserExist               uint32 = 20001
	UserNotExist            uint32 = 20002
	UserPasswordError       uint32 = 20003
	UserDisabled            uint32 = 20004
	InsufficientBalance     uint32 = 20005
	StopRegister            uint32 = 20006
	TelegramNotBound        uint32 = 20007
	UserNotBindOauth        uint32 = 20008
	InviteCodeError         uint32 = 20009
	UserCommissionNotEnough uint32 = 20010
)

// Node error
const (
	NodeExist         uint32 = 30001
	NodeNotExist      uint32 = 30002
	NodeGroupExist    uint32 = 30003
	NodeGroupNotExist uint32 = 30004
	NodeGroupNotEmpty uint32 = 30005
)

// Node routing capability negotiation (network domain).
const NodeRoutingUpgradeRequired uint32 = 140001
const NetworkRoutingRevisionConflict uint32 = 140002
const PlatformRoutingRevisionConflict uint32 = 160001
const PlatformLegacyRoutingRemoved uint32 = 160003
const NetworkLegacyRoutingRemoved uint32 = 140003
const NetworkNodeProtocolInvalid uint32 = 140004

// Request error
const (
	InvalidParams     uint32 = 400
	TooManyRequests   uint32 = 401
	ErrorTokenEmpty   uint32 = 40002
	ErrorTokenInvalid uint32 = 40003
	ErrorTokenExpire  uint32 = 40004
	InvalidAccess     uint32 = 40005
	InvalidCiphertext uint32 = 40006
	SecretIsEmpty     uint32 = 40007
)

//coupon error

const (
	CouponNotExist          uint32 = 50001 // Coupon does not exist
	CouponAlreadyUsed       uint32 = 50002 // Coupon has already been used
	CouponNotApplicable     uint32 = 50003 // Coupon does not match the order or conditions
	CouponInsufficientUsage uint32 = 50004 // Coupon has insufficient remaining uses
	CouponExpired           uint32 = 50005 // Coupon is expired
	CouponDisabled          uint32 = 50006 // Coupon is disabled
)

// Subscribe

const (
	SubscribeExpired                uint32 = 60001
	SubscribeNotAvailable           uint32 = 60002
	UserSubscribeExist              uint32 = 60003
	SubscribeIsUsedError            uint32 = 60004
	SingleSubscribeModeExceedsLimit uint32 = 60005
	SubscribeQuotaLimit             uint32 = 60006
	SubscribeOutOfStock             uint32 = 60007
)

// Auth error

const (
	VerifyCodeError uint32 = 70001
)

// equipment error

const (
	QueueEnqueueError uint32 = 80001
)

// System error

const (
	DebugModeError uint32 = 90001
)

const (
	SendSmsError    uint32 = 90002
	SmsNotEnabled   uint32 = 90003
	EmailNotEnabled uint32 = 90004
)

const (
	GetAuthenticatorError          uint32 = 90005
	AuthenticatorNotSupportedError uint32 = 90006
	TelephoneAreaCodeIsEmpty       uint32 = 90007
	TodaySendCountExceedsLimit     uint32 = 90015
)

const (
	PasswordIsEmpty                    uint32 = 90008
	AreaCodeIsEmpty                    uint32 = 90009
	PasswordOrVerificationCodeRequired uint32 = 90010
	EmailExist                         uint32 = 90011
	TelephoneExist                     uint32 = 90012
	DeviceExist                        uint32 = 90013
	TelephoneError                     uint32 = 90014
)
const (
	DeviceNotExist uint32 = 90017
	UseridNotMatch uint32 = 90018
)

const (
	OrderNotExist         uint32 = 61001
	PaymentMethodNotFound uint32 = 61002
	OrderStatusError      uint32 = 61003
	InsufficientOfPeriod  uint32 = 61004
	ExistAvailableTraffic uint32 = 61005
)
