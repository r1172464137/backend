// Package user holds the identity module's account entities: the user row,
// its sign-in identities, devices and their online records, the filters of
// the account queries, and the account's cache keys and request context.
package user

import (
	"time"

	"gorm.io/gorm"
)

// User is an account row.
type User struct {
	Id                    int64          `gorm:"primaryKey"`
	Password              string         `gorm:"type:varchar(255);not null;comment:User Password"`
	Algo                  string         `gorm:"type:varchar(20);default:'default';comment:Encryption Algorithm"`
	Salt                  string         `gorm:"type:varchar(20);default:null;comment:Password Salt"`
	Avatar                string         `gorm:"type:MEDIUMTEXT;comment:User Avatar"`
	ReferCode             string         `gorm:"type:varchar(20);default:'';comment:Referral Code"`
	RefererId             int64          `gorm:"index:idx_referer;comment:Referrer ID"`
	ReferralPercentage    uint8          `gorm:"default:0;comment:Referral"`                        // Referral Percentage
	OnlyFirstPurchase     *bool          `gorm:"default:true;not null;comment:Only First Purchase"` // Only First Purchase Referral
	Enable                *bool          `gorm:"default:true;not null;comment:Is Account Enabled"`
	IsAdmin               *bool          `gorm:"default:false;not null;comment:Is Admin"`
	EnableBalanceNotify   *bool          `gorm:"default:false;not null;comment:Enable Balance Change Notifications"`
	EnableLoginNotify     *bool          `gorm:"default:false;not null;comment:Enable Login Notifications"`
	EnableSubscribeNotify *bool          `gorm:"default:false;not null;comment:Enable Subscription Notifications"`
	EnableTradeNotify     *bool          `gorm:"default:false;not null;comment:Enable Trade Notifications"`
	AuthMethods           []AuthMethods  `gorm:"foreignKey:UserId;references:Id"`
	UserDevices           []Device       `gorm:"foreignKey:UserId;references:Id"`
	Rules                 string         `gorm:"type:TEXT;comment:User Rules"`
	CreatedAt             time.Time      `gorm:"<-:create;comment:Creation Time"`
	UpdatedAt             time.Time      `gorm:"comment:Update Time"`
	DeletedAt             gorm.DeletedAt `gorm:"index;comment:Deletion Time"`
}

// AccountState is the minimal account gate used on request hot paths that do
// not need credentials, devices, notification settings, or auth methods.
type AccountState struct {
	Id     int64
	Enable *bool
	// IsAdmin is read by request authentication (FindAccountStateForAuth);
	// the cached node-side gate leaves it nil.
	IsAdmin   *bool
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (*User) TableName() string {
	return "user"
}

// AuthMethods is a login identity of a user. An identifier is unique within
// its auth type (idx_auth_type_identifier), not across types.
type AuthMethods struct {
	Id             int64     `gorm:"primaryKey"`
	UserId         int64     `gorm:"index:idx_user_id;not null;comment:User ID"`
	AuthType       string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_auth_type_identifier,priority:1;comment:Auth Type 1: apple 2: google 3: github 4: facebook 5: telegram 6: email 7: mobile 8: device"`
	AuthIdentifier string    `gorm:"type:varchar(255);uniqueIndex:idx_auth_type_identifier,priority:2;not null;comment:Auth Identifier"`
	Verified       bool      `gorm:"default:false;not null;comment:Is Verified"`
	CreatedAt      time.Time `gorm:"<-:create;comment:Creation Time"`
	UpdatedAt      time.Time `gorm:"comment:Update Time"`
}

func (*AuthMethods) TableName() string {
	return "user_auth_methods"
}

// Device is a device bound to an account, which signs in with its
// identifier.
type Device struct {
	Id         int64     `gorm:"primaryKey"`
	Ip         string    `gorm:"type:varchar(255);not null;comment:Device IP"`
	UserId     int64     `gorm:"index:idx_user_id;not null;comment:User ID"`
	UserAgent  string    `gorm:"default:null;comment:UserAgent."`
	Identifier string    `gorm:"type:varchar(255);unique;index:idx_identifier;default:'';comment:Device Identifier"`
	Online     bool      `gorm:"default:false;not null;comment:Online"`
	Enabled    bool      `gorm:"default:true;not null;comment:Enabled"`
	CreatedAt  time.Time `gorm:"<-:create;comment:Creation Time"`
	UpdatedAt  time.Time `gorm:"comment:Update Time"`
}

func (*Device) TableName() string {
	return "user_device"
}

// DeviceOnlineRecord is the online time of one device connection.
type DeviceOnlineRecord struct {
	Id            int64     `gorm:"primaryKey"`
	UserId        int64     `gorm:"type:bigint;not null;comment:User ID"`
	Identifier    string    `gorm:"type:varchar(255);not null;comment:Device Identifier"`
	OnlineTime    time.Time `gorm:"comment:Online Time"` // The time when the device goes online
	OfflineTime   time.Time `gorm:"comment:Offline Time"`
	OnlineSeconds int64     `gorm:"comment:Offline Seconds"`
	DurationDays  int64     `gorm:"comment:Duration Days"`
	CreatedAt     time.Time `gorm:"<-:create;comment:Creation Time"`
}

func (DeviceOnlineRecord) TableName() string {
	return "user_device_online_record"
}

// LoginLogFilterParams filters user login logs.
type LoginLogFilterParams struct {
	IP        string
	UserId    int64
	UserAgent string
	Success   *bool
}

// UserFilterParams filters users in paginated queries.
type UserFilterParams struct {
	Search             string
	UserId             *int64
	SubscribeId        *int64
	UserSubscribeId    *int64
	UserSubscribeToken string
	Order              string // Order by id, e.g., "desc"
	Unscoped           bool   // Whether to include soft-deleted records
}

// EmailRecipientFilter filters email recipients.
type EmailRecipientFilter struct {
	Scope             int8
	RegisterStartTime int64
	RegisterEndTime   int64
}

// UserStatisticsWithDate holds aggregated user statistics per day/month.
type UserStatisticsWithDate struct {
	Date              string
	Register          int64
	NewOrderUsers     int64
	RenewalOrderUsers int64
}
