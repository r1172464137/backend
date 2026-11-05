package activation

// The labels of the Telegram order notices. Operators read these texts in
// the admin group; they are kept exactly as the notices always showed them.
const (
	// noticeRechargeName names a balance recharge in the plan column.
	noticeRechargeName = "余额充值"
	// noticeResetTrafficName names a traffic reset in the plan column.
	noticeResetTrafficName = "流量重置"
	// noticeStatusPaid is the order status every notice reports.
	noticeStatusPaid = "已支付"
	// noticeTimeLayout formats the order and expiry times.
	noticeTimeLayout = "2006-01-02 15:04:05"
)
