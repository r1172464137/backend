// Package wallet holds the billing domain's money entities: the wallet row
// (the single source of truth for balance/gift/commission since migration
// 02144) and withdrawal records (ADR-001 step 5).
package wallet

import "time"

type Wallet struct {
	UserId     int64 `gorm:"primaryKey"`
	Balance    int64 `gorm:"not null;default:0;comment:User Balance Amount"`
	GiftAmount int64 `gorm:"not null;default:0;comment:User Gift Amount"`
	Commission int64 `gorm:"not null;default:0;comment:Commission Amount"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Wallet) TableName() string {
	return "user_wallet"
}

// Adjustment is an administrator's edit of a wallet: each amount that is set
// becomes the wallet's; a nil amount is left as it is, so a form that only
// carries the amounts the administrator edited cannot revert the purchases,
// refunds and claw-backs that happened since it was loaded.
type Adjustment struct {
	UserId     int64
	Balance    *int64
	GiftAmount *int64
	Commission *int64
}

type Withdrawal struct {
	Id        int64     `gorm:"primaryKey"`
	UserId    int64     `gorm:"index:idx_user_id;not null;comment:User ID"`
	Amount    int64     `gorm:"not null;comment:Withdrawal Amount"`
	Content   string    `gorm:"type:text;comment:Withdrawal Content"`
	Status    uint8     `gorm:"type:tinyint(1);default:0;comment:Withdrawal Status: 0: Pending 1: Approved 2: Rejected"`
	Reason    string    `gorm:"type:varchar(500);default:'';comment:Rejection Reason"`
	CreatedAt time.Time `gorm:"<-:create;comment:Creation Time"`
	UpdatedAt time.Time `gorm:"comment:Update Time"`
}

const (
	WithdrawalStatusPending uint8 = iota
	WithdrawalStatusApproved
	WithdrawalStatusRejected
)

func (*Withdrawal) TableName() string {
	return "withdrawals"
}
