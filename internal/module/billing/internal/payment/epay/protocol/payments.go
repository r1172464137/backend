package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type Payment struct {
	Type       string // alipay, wxpay, qqpay; empty permitted for page cashier / scan autodetection.
	OutTradeNo string
	NotifyURL  string
	ReturnURL  string
	Name       string
	Money      string // Yuan, e.g. "1.00". Never convert through float64.
	Param      string
	Extra      Params // merchant_id, channel_id, fee_mode, cert_no, cert_name, min_age, etc.
}

type CreatePaymentRequest struct {
	Payment
	Method   string // web, jump, jsapi, app, scan, applet.
	Device   string // pc, mobile, qq, wechat, alipay, douyin.
	ClientIP string
}

type PaymentResult struct {
	TradeNo string `json:"trade_no"`
	PayType string `json:"pay_type"`
	PayInfo string `json:"pay_info"`
}

// PageForm can be rendered with html/template. Fields are signed but not HTML-safe
// strings on their own; never concatenate them into HTML without escaping.
type PageForm struct {
	Action string
	Fields url.Values
}

func (f *PageForm) URL() string { return f.Action + "?" + f.Fields.Encode() }

func (p Payment) params() (Params, error) {
	if strings.TrimSpace(p.OutTradeNo) == "" || strings.TrimSpace(p.Name) == "" {
		return nil, errors.New("epay: out_trade_no and name are required")
	}
	if len([]byte(p.Name)) > 127 {
		return nil, errors.New("epay: name exceeds 127 UTF-8 bytes")
	}
	if p.Type != "" && p.Type != "alipay" && p.Type != "wxpay" && p.Type != "qqpay" {
		return nil, errors.New("epay: unsupported V2 payment type")
	}
	if err := positiveAmount(p.Money); err != nil {
		return nil, err
	}
	if err := callbackURL(p.NotifyURL); err != nil {
		return nil, fmt.Errorf("epay: notify_url: %w", err)
	}
	if p.ReturnURL != "" {
		if err := callbackURL(p.ReturnURL); err != nil {
			return nil, fmt.Errorf("epay: return_url: %w", err)
		}
	}
	params := Params{
		"type": p.Type, "out_trade_no": p.OutTradeNo, "notify_url": p.NotifyURL,
		"return_url": p.ReturnURL, "name": p.Name, "money": p.Money, "param": p.Param,
	}
	for key, value := range p.Extra {
		if _, exists := params[key]; exists {
			return nil, fmt.Errorf("epay: extra parameter overrides %s", key)
		}
		switch key {
		case "method", "device", "clientip":
			return nil, fmt.Errorf("epay: use the typed field for %s", key)
		}
		params[key] = value
	}
	return params, nil
}

func callbackURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return errors.New("expected an absolute HTTP(S) URL without credentials or fragment")
	}
	return nil
}

func (c *Client) PagePayment(payment Payment) (*PageForm, error) {
	params, err := payment.params()
	if err != nil {
		return nil, err
	}
	if payment.ReturnURL == "" {
		return nil, errors.New("epay: return_url is required for page payment")
	}
	form, err := c.form(Request{Params: params})
	if err != nil {
		return nil, err
	}
	return &PageForm{Action: c.baseURL + "/api/pay/submit", Fields: form}, nil
}

func (request CreatePaymentRequest) params() (Params, error) {
	params, err := request.Payment.params()
	if err != nil {
		return nil, err
	}
	switch request.Method {
	case "web", "jump", "jsapi", "app", "scan", "applet":
	default:
		return nil, errors.New("epay: unsupported payment method")
	}
	if request.Type == "" && request.Method != "scan" {
		return nil, errors.New("epay: type is required except for scan payments")
	}
	if net.ParseIP(request.ClientIP) == nil {
		return nil, errors.New("epay: clientip must be an IP address")
	}
	if request.Method == "scan" && params["auth_code"] == "" {
		return nil, errors.New("epay: auth_code is required for scan payments")
	}
	if request.Method == "jsapi" && (params["sub_openid"] == "" || (request.Type == "wxpay" && params["sub_appid"] == "")) {
		return nil, errors.New("epay: jsapi requires sub_openid; WeChat also requires sub_appid")
	}
	if request.Device != "" {
		switch request.Device {
		case "pc", "mobile", "qq", "wechat", "alipay", "douyin":
		default:
			return nil, errors.New("epay: unsupported device")
		}
	}
	params["method"], params["device"], params["clientip"] = request.Method, request.Device, request.ClientIP
	return params, nil
}

// Validate checks a create before the application persists its attempt fence.
func (request CreatePaymentRequest) Validate() error {
	_, err := request.params()
	return err
}

func (c *Client) CreatePayment(ctx context.Context, request CreatePaymentRequest) (*PaymentResult, error) {
	params, err := request.params()
	if err != nil {
		return nil, err
	}
	response, err := c.Call(ctx, "/api/pay/create", Request{Params: params})
	if err != nil {
		return nil, err
	}
	var result PaymentResult
	if err := response.decodeRequired(&result, "trade_no", "pay_type", "pay_info"); err != nil {
		return nil, err
	}
	return &result, nil
}

type OrderRef struct {
	TradeNo    string
	OutTradeNo string
}

func (r OrderRef) params() (Params, error) {
	if (r.TradeNo == "") == (r.OutTradeNo == "") {
		return nil, errors.New("epay: provide exactly one of trade_no and out_trade_no")
	}
	return Params{"trade_no": r.TradeNo, "out_trade_no": r.OutTradeNo}, nil
}

type OrderStatus int

const (
	OrderUnpaid OrderStatus = iota
	OrderPaid
	OrderRefunded
	OrderFrozen
	OrderPreauthorized
)

type Order struct {
	TradeNo     string      `json:"trade_no"`
	OutTradeNo  string      `json:"out_trade_no"`
	APITradeNo  string      `json:"api_trade_no"`
	Type        string      `json:"type"`
	Status      OrderStatus `json:"status"`
	Name        string      `json:"name"`
	Money       string      `json:"money"`
	RefundMoney string      `json:"refundmoney"`
	Param       string      `json:"param"`
	Buyer       string      `json:"buyer"`
	ClientIP    string      `json:"clientip"`
	CreatedAt   string      `json:"addtime"`
	CompletedAt string      `json:"endtime"`
}

func (c *Client) QueryOrder(ctx context.Context, ref OrderRef) (*Order, error) {
	params, err := ref.params()
	if err != nil {
		return nil, err
	}
	response, err := c.Call(ctx, "/api/pay/query", Request{Params: params})
	if err != nil {
		return nil, err
	}
	var order Order
	if err := response.decodeRequired(&order, "pid", "trade_no", "out_trade_no", "type", "status", "money"); err != nil {
		return nil, err
	}
	if (ref.TradeNo != "" && order.TradeNo != ref.TradeNo) || (ref.OutTradeNo != "" && order.OutTradeNo != ref.OutTradeNo) {
		return nil, errors.New("epay: response order does not match the requested order")
	}
	if err := positiveAmount(order.Money); err != nil {
		return nil, err
	}
	return &order, nil
}

func (c *Client) CloseOrder(ctx context.Context, ref OrderRef) error {
	params, err := ref.params()
	if err != nil {
		return err
	}
	_, err = c.Call(ctx, "/api/pay/close", Request{Params: params})
	return err
}

type RefundRequest struct {
	OrderRef
	Money       string
	OutRefundNo string // Required by this SDK, although optional in the upstream API.
}

type RefundResult struct {
	RefundNo    string `json:"refund_no"`
	OutRefundNo string `json:"out_refund_no"`
	TradeNo     string `json:"trade_no"`
	Money       string `json:"money"`
	ReduceMoney string `json:"reducemoney"`
}

func (c *Client) Refund(ctx context.Context, request RefundRequest) (*RefundResult, error) {
	params, err := request.OrderRef.params()
	if err != nil {
		return nil, err
	}
	if err := positiveAmount(request.Money); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.OutRefundNo) == "" {
		return nil, errors.New("epay: out_refund_no is required for safe refund reconciliation")
	}
	params["money"], params["out_refund_no"] = request.Money, request.OutRefundNo
	response, err := c.Call(ctx, "/api/pay/refund", Request{Params: params})
	if err != nil {
		return nil, err
	}
	var result RefundResult
	if err := response.decodeRequired(&result, "refund_no", "out_refund_no", "trade_no", "money"); err != nil {
		return nil, err
	}
	requestedCents, _ := ParseAmount(request.Money)
	returnedCents, amountErr := ParseAmount(result.Money)
	if result.OutRefundNo != request.OutRefundNo || (request.TradeNo != "" && result.TradeNo != request.TradeNo) || amountErr != nil || requestedCents != returnedCents {
		return nil, errors.New("epay: refund response does not match the request; reconcile before retrying")
	}
	return &result, nil
}

type RefundRef struct {
	RefundNo    string
	OutRefundNo string
}

type RefundStatus struct {
	RefundResult
	OutTradeNo string `json:"out_trade_no"`
	Status     int    `json:"status"` // 0 failed, 1 succeeded; separate from API code.
	CreatedAt  string `json:"addtime"`
}

func (c *Client) QueryRefund(ctx context.Context, ref RefundRef) (*RefundStatus, error) {
	if (ref.RefundNo == "") == (ref.OutRefundNo == "") {
		return nil, errors.New("epay: provide exactly one of refund_no and out_refund_no")
	}
	response, err := c.Call(ctx, "/api/pay/refundquery", Request{Params: Params{
		"refund_no": ref.RefundNo, "out_refund_no": ref.OutRefundNo,
	}})
	if err != nil {
		return nil, err
	}
	var result RefundStatus
	if err := response.decodeRequired(&result, "refund_no", "out_refund_no", "trade_no", "money", "status"); err != nil {
		return nil, err
	}
	if (ref.RefundNo != "" && result.RefundNo != ref.RefundNo) || (ref.OutRefundNo != "" && result.OutRefundNo != ref.OutRefundNo) {
		return nil, errors.New("epay: response refund does not match the requested refund")
	}
	return &result, nil
}

func (r *Response) decodeRequired(destination any, required ...string) error {
	return decodeRequired(r.raw, destination, required...)
}

func decodeRequired(data []byte, destination any, required ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range required {
		raw, exists := fields[key]
		if !exists || string(raw) == "null" || string(raw) == `""` || len(raw) == 0 || raw[0] == '{' || raw[0] == '[' {
			return fmt.Errorf("epay: missing or non-scalar response field %s", key)
		}
	}
	return json.Unmarshal(data, destination)
}
