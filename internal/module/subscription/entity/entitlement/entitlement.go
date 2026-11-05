// Package entitlement stores provider-authoritative subscription periods.
package entitlement

import "time"

// State is the last reconciled snapshot for one provider subscription. Revision
// is allocated by the verified billing ledger, never by notification arrival.
type State struct {
	ID              string `gorm:"primaryKey;type:char(64)"`
	Source          string `gorm:"type:varchar(32);not null"`
	Scope           string `gorm:"type:varchar(255);not null"`
	SubscriptionKey string `gorm:"type:varchar(255);not null"`
	UserID          int64  `gorm:"not null"`
	UserSubscribeID int64  `gorm:"not null;uniqueIndex"`
	Revision        int64  `gorm:"not null"`
	Fingerprint     string `gorm:"type:char(64);not null"`
	PeriodID        string `gorm:"type:char(64);not null"`
	Mode            string `gorm:"type:varchar(32);not null"`
	Status          string `gorm:"type:varchar(32);not null"`
	AutoRenew       bool   `gorm:"not null"`
	GraceUntil      *time.Time
	RevokedAt       *time.Time
	UpdatedAt       time.Time
}

func (State) TableName() string { return "subscription_entitlement" }

// Period records an absolute interval rather than an increment to replay.
// Its global provider transaction identity prevents cross-account reuse.
type Period struct {
	ID              string `gorm:"primaryKey;type:char(64)"`
	EntitlementID   string `gorm:"type:char(64);not null;index"`
	TransactionKey  string `gorm:"type:varchar(255);not null"`
	UserSubscribeID int64  `gorm:"not null;index"`
	OrderID         int64  `gorm:"not null"`
	PlanID          int64  `gorm:"not null"`
	// Empty for local orders; month or year for Apple auto-renewable products.
	BillingInterval string    `gorm:"type:varchar(16);not null;default:''"`
	TrafficLimit    int64     `gorm:"not null"`
	StartAt         time.Time `gorm:"not null"`
	EndAt           time.Time `gorm:"not null"`
	TrafficReset    bool      `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Period) TableName() string { return "subscription_period" }

// Revision preserves reconciled changes, including revocations and reversals.
// Unlike the operational inbox, this history is not subject to short retention.
type Revision struct {
	ID            string `gorm:"primaryKey;type:varchar(96)"`
	EntitlementID string `gorm:"type:char(64);not null;index"`
	Payload       string `gorm:"type:text;not null"`
	CreatedAt     time.Time
}

func (Revision) TableName() string { return "subscription_entitlement_revision" }
