package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type V1Payment struct {
	Type       string // Empty selects the cashier for PagePayment.
	OutTradeNo string
	NotifyURL  string
	ReturnURL  string
	Name       string
	Money      string
	Param      string
}

func (p V1Payment) params() (Params, error) {
	if strings.TrimSpace(p.OutTradeNo) == "" || strings.TrimSpace(p.Name) == "" || len(p.Name) > 127 {
		return nil, errors.New("epay v1: out_trade_no and name are required; name must not exceed 127 UTF-8 bytes")
	}
	switch p.Type {
	case "", "alipay", "wxpay", "qqpay", "bank", "jdpay", "paypal", "douyinpay":
	default:
		return nil, errors.New("epay v1: unsupported payment type")
	}
	if err := positiveAmount(p.Money); err != nil {
		return nil, err
	}
	if err := callbackURL(p.NotifyURL); err != nil {
		return nil, err
	}
	if p.ReturnURL != "" {
		if err := callbackURL(p.ReturnURL); err != nil {
			return nil, err
		}
	}
	return Params{
		"type": p.Type, "out_trade_no": p.OutTradeNo, "notify_url": p.NotifyURL,
		"return_url": p.ReturnURL, "name": p.Name, "money": p.Money, "param": p.Param,
	}, nil
}

func (c *V1Client) PagePayment(payment V1Payment) (*PageForm, error) {
	params, err := payment.params()
	if err != nil {
		return nil, err
	}
	if payment.ReturnURL == "" {
		return nil, errors.New("epay v1: return_url is required for page payment")
	}
	signed, err := c.SignedParams(params)
	if err != nil {
		return nil, err
	}
	return &PageForm{Action: c.baseURL + "/submit.php", Fields: v1Values(signed)}, nil
}

type V1CreatePaymentRequest struct {
	V1Payment
	ClientIP string
	Device   string // pc, mobile, qq, wechat, alipay, douyin, jump; empty uses the server default.
}

type V1PaymentResult struct {
	TradeNo   string `json:"trade_no"`
	PayURL    string `json:"payurl"`
	QRCode    string `json:"qrcode"`
	URLScheme string `json:"urlscheme"`
}

func (request V1CreatePaymentRequest) params() (Params, error) {
	params, err := request.V1Payment.params()
	if err != nil {
		return nil, err
	}
	if request.Type == "" || net.ParseIP(request.ClientIP) == nil {
		return nil, errors.New("epay v1: type and a valid clientip are required for API payment")
	}
	switch request.Device {
	case "", "pc", "mobile", "qq", "wechat", "alipay", "douyin", "jump":
	default:
		return nil, errors.New("epay v1: unsupported device")
	}
	params["clientip"], params["device"] = request.ClientIP, request.Device
	return params, nil
}

// Validate checks a create before the application persists its attempt fence.
func (request V1CreatePaymentRequest) Validate() error {
	_, err := request.params()
	return err
}

func (c *V1Client) CreatePayment(ctx context.Context, request V1CreatePaymentRequest) (*V1PaymentResult, error) {
	params, err := request.params()
	if err != nil {
		return nil, err
	}
	signed, err := c.SignedParams(params)
	if err != nil {
		return nil, err
	}
	response, err := c.send(ctx, http.MethodPost, c.baseURL+"/mapi.php", v1Values(signed), 1)
	if err != nil {
		return nil, err
	}
	var result V1PaymentResult
	if err := decodeRequired(response.raw, &result, "trade_no"); err != nil {
		return nil, err
	}
	count := 0
	for _, value := range []string{result.PayURL, result.QRCode, result.URLScheme} {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	if count != 1 {
		return nil, errors.New("epay v1: expected exactly one of payurl, qrcode and urlscheme")
	}
	return &result, nil
}

// V1Order deliberately uses a separate status field from V2's status enum.
// The V1 reference defines only 0 (unpaid) and 1 (paid).
type V1Order struct {
	MerchantID  string `json:"pid"`
	TradeNo     string `json:"trade_no"`
	OutTradeNo  string `json:"out_trade_no"`
	APITradeNo  string `json:"api_trade_no"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Money       string `json:"money"`
	Status      int    `json:"status"`
	Param       string `json:"param"`
	Buyer       string `json:"buyer"`
	CreatedAt   string `json:"addtime"`
	CompletedAt string `json:"endtime"`
}

// UnmarshalJSON tolerates PHP's integer strings for pid/status, while preserving
// money and order identifiers as strings and rejecting duplicate top-level keys.
func (o *V1Order) UnmarshalJSON(data []byte) error {
	if _, err := responseObject(data); err != nil {
		return err
	}
	type plain V1Order
	var result plain
	wire := struct {
		*plain
		PID    json.RawMessage `json:"pid"`
		Status json.RawMessage `json:"status"`
	}{plain: &result}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	pid, err := v1Scalar(wire.PID)
	if err != nil {
		return errors.New("epay v1: missing or invalid response pid")
	}
	status, err := v1Integer(wire.Status)
	if err != nil {
		return errors.New("epay v1: missing or invalid order status")
	}
	result.MerchantID, result.Status = pid, status
	*o = V1Order(result)
	return nil
}

func (c *V1Client) QueryOrder(ctx context.Context, ref OrderRef) (*V1Order, error) {
	params, err := ref.params()
	if err != nil {
		return nil, err
	}
	response, err := c.queryAPI(ctx, "order", params)
	if err != nil {
		return nil, err
	}
	var order V1Order
	if err := decodeRequired(response.raw, &order, "pid", "trade_no", "out_trade_no", "type", "money", "status"); err != nil {
		return nil, err
	}
	if order.MerchantID != c.merchantID || (ref.TradeNo != "" && order.TradeNo != ref.TradeNo) || (ref.OutTradeNo != "" && order.OutTradeNo != ref.OutTradeNo) {
		return nil, errors.New("epay v1: response merchant/order does not match the request")
	}
	if err := positiveAmount(order.Money); err != nil {
		return nil, err
	}
	return &order, nil
}

type V1RefundRequest struct {
	OrderRef
	Money string // V1 has no documented out_refund_no or refund query endpoint.
}

type V1RefundResult struct {
	Message string `json:"msg"`
}

func (c *V1Client) Refund(ctx context.Context, request V1RefundRequest) (*V1RefundResult, error) {
	params, err := request.OrderRef.params()
	if err != nil {
		return nil, err
	}
	if err := positiveAmount(request.Money); err != nil {
		return nil, err
	}
	form := v1Values(params)
	form.Set("pid", c.merchantID)
	form.Set("key", c.key)
	form.Set("money", request.Money)
	response, err := c.send(ctx, http.MethodPost, c.baseURL+"/api.php?"+url.Values{"act": {"refund"}}.Encode(), form, 0)
	if err != nil {
		return nil, err
	}
	var result V1RefundResult
	if err := response.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
