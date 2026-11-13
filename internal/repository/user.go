package repository

import (
	"context"
	"time"

	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"gorm.io/gorm"
)

// UserRepo provides user profile, account, reporting, and marketing queries.
// Related authentication, subscription, device, cache, withdrawal, and traffic
// operations live behind their own focused repository interfaces below.
type UserRepo interface {
	Insert(ctx context.Context, data *user.User, tx ...*gorm.DB) error
	FindOne(ctx context.Context, id int64) (*user.User, error)
	FindAccountState(ctx context.Context, id int64) (*user.AccountState, error)
	FindOneForUpdate(ctx context.Context, id int64) (*user.User, error)
	FindOneByEmail(ctx context.Context, email string) (*user.User, error)
	FindOneByReferCode(ctx context.Context, referCode string) (*user.User, error)
	Update(ctx context.Context, data *user.User, tx ...*gorm.DB) error
	UpgradePasswordHash(ctx context.Context, id int64, currentHash, password, algo, salt string) (bool, error)
	Delete(ctx context.Context, id int64, tx ...*gorm.DB) error
	BatchDeleteUser(ctx context.Context, ids []int64, tx ...*gorm.DB) error
	QueryPageList(ctx context.Context, page, size int, filter *user.UserFilterParams) ([]*user.User, int64, error)
	FindUsersByIds(ctx context.Context, ids []int64) ([]*user.User, error)
	FindEnabledUserIDs(ctx context.Context, ids []int64) ([]int64, error)
	CountAffiliates(ctx context.Context, refererId int64) (int64, error)
	QueryAffiliateList(ctx context.Context, refererId int64, page, size int) ([]*user.User, int64, error)
	QueryAdminUsers(ctx context.Context) ([]*user.User, error)
	CountEnabledUsers(ctx context.Context) (int64, error)
	QueryResisterUserTotal(ctx context.Context) (int64, error)
	QueryResisterUserTotalByDate(ctx context.Context, date time.Time) (int64, error)
	QueryResisterUserTotalByMonthly(ctx context.Context, date time.Time) (int64, error)
	QueryEmailRecipients(ctx context.Context, filter *user.EmailRecipientFilter) ([]string, error)
	CountEmailRecipients(ctx context.Context, filter *user.EmailRecipientFilter) (int64, error)
	QueryDailyUserStatisticsList(ctx context.Context, date time.Time) ([]user.UserStatisticsWithDate, error)
	QueryMonthlyUserStatisticsList(ctx context.Context, date time.Time) ([]user.UserStatisticsWithDate, error)
}

// UserAuthRepo manages external authentication identities linked to users.
type UserAuthRepo interface {
	FindUserAuthMethods(ctx context.Context, userId int64) ([]*user.AuthMethods, error)
	FindUserAuthMethodsByUserIds(ctx context.Context, method string, userIds []int64) ([]*user.AuthMethods, error)
	FindUserAuthMethodByOpenID(ctx context.Context, method, openID string) (*user.AuthMethods, error)
	ValidateEmailIdentityUniqueness(ctx context.Context) error
	FindUserAuthMethodByPlatform(ctx context.Context, userId int64, platform string) (*user.AuthMethods, error)
	FindUserAuthMethodByUserId(ctx context.Context, method string, userId int64) (*user.AuthMethods, error)
	InsertUserAuthMethods(ctx context.Context, data *user.AuthMethods, tx ...*gorm.DB) error
	UpdateUserAuthMethods(ctx context.Context, data *user.AuthMethods, tx ...*gorm.DB) error
	DeleteUserAuthMethods(ctx context.Context, userId int64, platform string, tx ...*gorm.DB) error
	UpdateUserAuthMethodOwner(ctx context.Context, authType, identifier string, userId int64, tx ...*gorm.DB) error
	DeleteUserAuthMethodByIdentifier(ctx context.Context, authType, identifier string, tx ...*gorm.DB) error
	UpsertUserAuthMethod(ctx context.Context, data *user.AuthMethods) error
}

// UserSubscriptionRepo manages user subscription records and their lifecycle.
type UserSubscriptionRepo interface {
	// LockUserSerial serializes subscription-creating flows per user inside
	// the current transaction (seed-and-lock on the subscription domain's
	// serial table). It replaces the fulfillment stage's cross-domain
	// user-row lock.
	LockUserSerial(ctx context.Context, userID int64) error
	InsertSubscribe(ctx context.Context, data *usersub.Subscribe, tx ...*gorm.DB) error
	FindOneSubscribe(ctx context.Context, id int64) (*usersub.Subscribe, error)
	FindOneSubscribeForUpdate(ctx context.Context, id int64) (*usersub.Subscribe, error)
	FindOneSubscribeByOrderId(ctx context.Context, orderId int64) (*usersub.Subscribe, error)
	FindOneSubscribeByToken(ctx context.Context, token string) (*usersub.Subscribe, error)
	FindOneSubscribeByTokenForUpdate(ctx context.Context, token string) (*usersub.Subscribe, error)
	UpdateSubscribe(ctx context.Context, data *usersub.Subscribe, tx ...*gorm.DB) error
	DeleteSubscribe(ctx context.Context, token string, tx ...*gorm.DB) error
	DeleteSubscribeById(ctx context.Context, id int64, tx ...*gorm.DB) error
	UpdateUserSubscribeWithTraffic(ctx context.Context, id, download, upload int64, tx ...*gorm.DB) error
	BatchUpdateUserSubscribeWithTraffic(ctx context.Context, deltas []trafficEntity.SubscribeTrafficDelta, tx ...*gorm.DB) error
	FindUsersSubscribeBySubscribeId(ctx context.Context, subscribeId int64) ([]*usersub.Subscribe, error)
	FindUsersSubscribeBySubscribeIds(ctx context.Context, subscribeIds []int64) ([]*usersub.Subscribe, error)
	FindUserSubscribesByStatus(ctx context.Context, status ...int64) ([]*usersub.Subscribe, error)
	FindSubscribesByIds(ctx context.Context, ids []int64) ([]*usersub.Subscribe, error)
	ActivatePendingSubscribesBySubscribeId(ctx context.Context, subscribeId int64) error
	ActivatePendingSubscribesBySubscribeIds(ctx context.Context, subscribeIds []int64) error
	CountQuotaConsumingSubscriptions(ctx context.Context, userId, subscribeId int64) (int64, error)
	HasBlockingSubscription(ctx context.Context, userId int64) (bool, error)
	CountUserSubscribesBySubscribeIdAndStatus(ctx context.Context, subscribeId int64, status ...int64) (int64, error)
	QueryActiveSubscriptions(ctx context.Context, subscribeId ...int64) (map[int64]int64, error)
	QueryUserSubscribe(ctx context.Context, userId int64, status ...int64) ([]*usersub.SubscribeDetails, error)
	FindOneSubscribeDetailsById(ctx context.Context, id int64) (*usersub.SubscribeDetails, error)
	FindSubscribeDetailsByIds(ctx context.Context, ids []int64) ([]*usersub.SubscribeDetails, error)
	FindOneUserSubscribe(ctx context.Context, id int64) (*usersub.SubscribeDetails, error)
	FindTrafficExceededSubscribes(ctx context.Context) ([]*usersub.Subscribe, error)
	FindExpiredSubscribes(ctx context.Context, now time.Time) ([]*usersub.Subscribe, error)
	// FindExpiringSubscribes returns active subscriptions expiring inside the
	// window, for the pre-expiry reminder.
	FindExpiringSubscribes(ctx context.Context, from, to time.Time) ([]*usersub.Subscribe, error)
	MarkSubscribesFinished(ctx context.Context, ids []int64, status uint8, finishedAt time.Time, tx ...*gorm.DB) error
	QuerySubscribeIdsByFilter(ctx context.Context, filter *usersub.SubscribeFilter) ([]int64, error)
	CountSubscribesByFilter(ctx context.Context, filter *usersub.SubscribeFilter) (int64, error)
}

// UserDeviceRepo manages registered devices and their online records.
type UserDeviceRepo interface {
	InsertDevice(ctx context.Context, data *user.Device, tx ...*gorm.DB) error
	FindOneDevice(ctx context.Context, id int64) (*user.Device, error)
	// FindDeviceForAuth always reads current state, bypassing cached projections.
	FindDeviceForAuth(ctx context.Context, id int64) (*user.Device, error)
	// TouchDevice only updates access metadata for the same enabled owner.
	TouchDevice(ctx context.Context, id, userID int64, ip, userAgent string) (bool, error)
	FindOneDeviceByIdentifier(ctx context.Context, id string) (*user.Device, error)
	SetDeviceEnabled(ctx context.Context, id int64, enabled bool) error
	SetDeviceOnline(ctx context.Context, id int64, online bool) error
	DeleteDevice(ctx context.Context, id int64, tx ...*gorm.DB) error
	QueryDeviceList(ctx context.Context, userid int64) ([]*user.Device, int64, error)
	QueryDevicePageList(ctx context.Context, userid, subscribeId int64, page, size int) ([]*user.Device, int64, error)
	FindDeviceOnlineRecord(ctx context.Context, userId int64, startTime, endTime string) (*user.DeviceOnlineRecord, error)
	InsertDeviceOnlineRecord(ctx context.Context, data *user.DeviceOnlineRecord, tx ...*gorm.DB) error
}

// UserWithdrawalRepo manages affiliate withdrawal records.
type UserWithdrawalRepo interface {
	InsertWithdrawal(ctx context.Context, data *walletEntity.Withdrawal, tx ...*gorm.DB) error
	FindWithdrawalForUpdate(ctx context.Context, id int64) (*walletEntity.Withdrawal, error)
	QueryWithdrawalList(ctx context.Context, userID int64, status *uint8, page, size int) ([]*walletEntity.Withdrawal, int64, error)
	UpdateWithdrawalStatus(ctx context.Context, id int64, from, to uint8, reason string) (bool, error)
}

// SubscriptionTrafficRepo manages scheduled subscription traffic resets.
type SubscriptionTrafficRepo interface {
	QueryMonthlyResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error)
	QueryFirstResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error)
	QueryYearlyResetSubscribeIds(ctx context.Context, subscribeIds []int64, now time.Time) ([]int64, error)
	ResetSubscribeTrafficByIds(ctx context.Context, ids []int64, tx ...*gorm.DB) error
}

// UserCacheRepo manages cached user-related projections.
type UserCacheRepo interface {
	ClearUserCache(ctx context.Context, data ...*user.User) error
	ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error
	ClearDeviceCache(ctx context.Context, data ...*user.Device) error
	ClearAuthMethodCache(ctx context.Context, data ...*user.AuthMethods) error
	BatchClearRelatedCache(ctx context.Context, data *user.User) error
	UpdateUserCache(ctx context.Context, data *user.User) error
	UpdateUserSubscribeCache(ctx context.Context, data *usersub.Subscribe) error
}

// The identity-family contracts. Their implementations live in the owning
// modules (identity, subscription, billing) and reach the store through the
// per-module builders in builders.go.
