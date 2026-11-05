package dto

import "time"

// ReconcileEntitlementCommand is an internal, trusted billing-to-subscription
// contract. It must NOT be bound to an HTTP request. Billing must authenticate
// the provider, resolve ownership and reconcile history before issuing it.
// Revision is a durable monotonic revision of that reconciled ledger, NOT a
// local receipt timestamp, raw Apple signedDate, or notification sequence.
type ReconcileEntitlementCommand struct {
	Source string
	// Scope isolates app and environment (for example bundleID:Production).
	Scope           string
	SubscriptionKey string
	TransactionKey  string
	UserID          int64
	PlanID          int64
	OrderID         int64
	Revision        int64
	// Mode must be auto_renewable. Fixed-term IAP is outside the supported scope.
	Mode string
	// BillingInterval is month or year, taken from the verified product mapping.
	// It describes the standard product interval, not a duration to add to
	// PeriodStart; actual trial, renewal and extension dates come from Apple.
	BillingInterval string
	// Status is active, grace, billing_retry, expired, or revoked.
	Status      string
	PeriodStart time.Time
	PeriodEnd   time.Time
	GraceUntil  *time.Time
	RevokedAt   *time.Time
	AutoRenew   bool
	// ResetTraffic applies at most once for this transaction's current
	// period, never for an expired period or a plan whose traffic is reset
	// by a separate calendar schedule (ResetCycle != 0).
	ResetTraffic bool
}

type EntitlementResult struct {
	UserSubscribeID int64
	Revision        int64
	Applied         bool
	AccessUntil     time.Time
}
