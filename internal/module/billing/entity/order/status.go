package order

import "time"

// PaymentWindowMinutes is how long a pending order waits for its payment
// before the deferred close task expires it; gateway checkouts are opened for
// the same window.
const PaymentWindowMinutes = 15

// PaymentWindow is PaymentWindowMinutes as a duration.
const PaymentWindow = PaymentWindowMinutes * time.Minute

// UnpaidCloseAge is the age from which a pending order is treated as
// abandoned: the expiry close releases an order a gateway that cannot cancel
// an issued payment page lists as unpaid, and a guest's pending order no
// longer counts toward the pending-order cap. It doubles the payment window
// so a payer who opened the gateway page near expiry still finishes.
const UnpaidCloseAge = 2 * PaymentWindow

// Order types, the values of the Type column.
const (
	TypeSubscribe    uint8 = 1
	TypeRenewal      uint8 = 2
	TypeResetTraffic uint8 = 3
	TypeRecharge     uint8 = 4
)

// Order statuses, the values of the Status column. An order moves from
// Pending to Paid when the payment is confirmed and from Paid to Finished once
// it is fulfilled; Closed and Failed end a Pending order.
const (
	StatusPending  uint8 = 1
	StatusPaid     uint8 = 2
	StatusClosed   uint8 = 3
	StatusFailed   uint8 = 4
	StatusFinished uint8 = 5
)

// The state machine's transition rules. Every flow that moves an order asks
// these predicates instead of comparing status numbers itself, so the
// callback, close, checkout and activation paths cannot drift apart.

// CanClose reports whether an order in status may still be closed: only a
// pending order can be given up.
func CanClose(status uint8) bool { return status == StatusPending }

// CanCheckout reports whether a payment may be started for an order in
// status; checkout is only offered while the order waits for payment.
func CanCheckout(status uint8) bool { return status == StatusPending }

// CanSettle reports whether a verified payment may settle an order in status.
// A pending order becomes paid; a paid order only re-enqueues its activation,
// which heals a settlement whose queue insertion failed.
func CanSettle(status uint8) bool { return status == StatusPending || status == StatusPaid }

// IsSettled reports whether the order's payment was collected: it is paid
// and awaiting fulfillment, or already finished.
func IsSettled(status uint8) bool { return status == StatusPaid || status == StatusFinished }

// IsTerminal reports whether no transition leaves status.
func IsTerminal(status uint8) bool {
	return status == StatusClosed || status == StatusFailed || status == StatusFinished
}

// SettledStatuses lists the statuses IsSettled accepts, for queries that
// select settled orders. Repositories must bind them as integers: a []uint8
// is a byte string to database/sql, not a list.
func SettledStatuses() []uint8 { return []uint8{StatusPaid, StatusFinished} }

// CouponUseStatuses lists the statuses of orders that count against a
// coupon's per-user limit: pending orders hold a reservation and settled
// orders consumed a use.
func CouponUseStatuses() []uint8 { return []uint8{StatusPending, StatusPaid, StatusFinished} }

// StatusName is the client-facing name of an order status in the V2 API.
func StatusName(status uint8) string {
	switch status {
	case StatusPending:
		return "pending_payment"
	case StatusPaid:
		return "paid"
	case StatusClosed:
		return "closed"
	case StatusFailed:
		return "failed"
	case StatusFinished:
		return "finished"
	default:
		return "unknown"
	}
}

// PaymentStatusName names the payment side of an order status, as carried by
// V2 snapshots and order events.
func PaymentStatusName(status uint8) string {
	switch status {
	case StatusPaid, StatusFinished:
		return "paid"
	case StatusClosed:
		return "closed"
	case StatusFailed:
		return "failed"
	default:
		return "pending"
	}
}

// FulfillmentStatusName names the fulfillment side of an order status, as
// carried by V2 snapshots and order events.
func FulfillmentStatusName(status uint8) string {
	switch status {
	case StatusPaid:
		return "pending"
	case StatusFinished:
		return "finished"
	default:
		return "not_started"
	}
}
