package taskqueue

import (
	"crypto/sha256"
	"encoding/hex"
)

// The order task types.
const (
	DeferCloseOrder                 = "defer:order:close"
	ForthwithActivateOrder          = "forthwith:order:activate"
	SchedulerReconcilePaidOrders    = "scheduler:order:reconcile-paid"
	SchedulerReconcilePendingOrders = "scheduler:order:reconcile-pending"
	SchedulerPublishOrderEvents     = "scheduler:order:publish-events"
	SchedulerCleanupOrderEvents     = "scheduler:order:cleanup-events"
	// SchedulerDailyOrderReport pushes the previous day's settlement summary
	// to the administrators bound on Telegram.
	SchedulerDailyOrderReport = "scheduler:order:daily-report"
)

type (
	// DeferCloseOrderPayload names the unpaid order DeferCloseOrder closes.
	DeferCloseOrderPayload struct {
		OrderNo string `json:"order_no"`
	}
	// ForthwithActivateOrderPayload names the paid order to activate.
	ForthwithActivateOrderPayload struct {
		OrderNo string `json:"order_no"`
	}
)

// ActivationTaskID is the task ID of an order's activation: every producer
// (payment callback, reconciliation) enqueues under the same ID, so a paid
// order has one activation task queued at a time, however often its payment
// is reported.
func ActivationTaskID(orderNo string) string {
	digest := sha256.Sum256([]byte(orderNo))
	return "order-activation:" + hex.EncodeToString(digest[:])
}
