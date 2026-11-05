package order

// RefundBasis is what was paid for the subscription term: the original order
// plus its paid renewals. Traffic resets buy traffic, not time, and are not
// refunded. The refund quote and the refund settlement both measure against
// it.
//
// An order's Amount is what was paid with money (through a gateway or from
// the wallet balance) and its GiftAmount the gift credit it consumed, at
// creation and at a balance checkout alike; the two never hold the same
// unit, so their sum is the value paid and refunding each to its own
// balance returns exactly that value.
func (d *Details) RefundBasis() int64 {
	basis := d.Amount + d.GiftAmount
	for _, subOrder := range d.SubOrders {
		if subOrder.IsPaidRenewal() {
			basis += subOrder.Amount + subOrder.GiftAmount
		}
	}
	return basis
}

// IsPaidRenewal reports whether the order renews a subscription and its
// payment was collected.
func (o *Order) IsPaidRenewal() bool {
	return o.Type == TypeRenewal && IsSettled(o.Status)
}
