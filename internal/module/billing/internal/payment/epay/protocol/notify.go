package protocol

import (
	"errors"
	"fmt"
	"net/url"
)

type Notification struct {
	MerchantID  string
	TradeNo     string
	OutTradeNo  string
	APITradeNo  string
	Type        string
	Money       string
	Param       string
	Buyer       string
	CreatedAt   string
	CompletedAt string
	Fields      Params // Includes future scalar fields used for verification.
}

// VerifyNotification validates authenticity, age, merchant and successful trade
// status. The caller must still match the stored order amount/type and atomically
// commit payment/delivery idempotently before replying with the literal "success".
func (c *Client) VerifyNotification(values url.Values) (*Notification, error) {
	params := make(Params, len(values))
	for key, items := range values {
		if len(items) != 1 {
			return nil, fmt.Errorf("epay: notification parameter %s must occur once", key)
		}
		params[key] = items[0]
	}
	if err := c.verify(params); err != nil {
		return nil, err
	}
	if params["pid"] != c.merchantID || params["trade_status"] != "TRADE_SUCCESS" {
		return nil, errors.New("epay: notification merchant or trade status mismatch")
	}
	for _, key := range []string{"trade_no", "out_trade_no", "type", "money"} {
		if params[key] == "" {
			return nil, fmt.Errorf("epay: notification missing %s", key)
		}
	}
	if err := positiveAmount(params["money"]); err != nil {
		return nil, err
	}
	return &Notification{
		MerchantID: params["pid"], TradeNo: params["trade_no"], OutTradeNo: params["out_trade_no"],
		APITradeNo: params["api_trade_no"], Type: params["type"], Money: params["money"],
		Param: params["param"], Buyer: params["buyer"], CreatedAt: params["addtime"],
		CompletedAt: params["endtime"], Fields: params,
	}, nil
}
