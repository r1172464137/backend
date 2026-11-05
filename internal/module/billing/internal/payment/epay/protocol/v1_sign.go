package protocol

import (
	"crypto/md5" //nolint:gosec // G501: EPay V1 mandates MD5(string + shared key).
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// V1CanonicalString preserves whitespace-only values, unlike the V2 reference
// SDK. Only empty strings, sign and sign_type are omitted. Values are not encoded.
func V1CanonicalString(params Params) (string, error) {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if !parameterName.MatchString(key) {
			return "", fmt.Errorf("epay v1: invalid scalar parameter name %q", key)
		}
		if key != "sign" && key != "sign_type" && value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&"), nil
}

func v1Signature(key string, params Params) (string, error) {
	canonical, err := V1CanonicalString(params)
	if err != nil {
		return "", err
	}
	digest := md5.Sum([]byte(canonical + key)) //nolint:gosec // G401: required by the V1 wire protocol.
	return hex.EncodeToString(digest[:]), nil
}

// SignedParams adds the configured pid and MD5 signature without exposing KEY.
// V1 does not inject a timestamp. Caller-owned maps are not changed.
func (c *V1Client) SignedParams(params Params) (Params, error) {
	result := make(Params, len(params)+3)
	for key, value := range params {
		switch key {
		case "pid", "key", "sign", "sign_type":
			return nil, fmt.Errorf("epay v1: %s is managed by the client", key)
		}
		result[key] = value
	}
	result["pid"] = c.merchantID
	result["sign_type"] = "MD5"
	signature, err := v1Signature(c.key, result)
	if err != nil {
		return nil, err
	}
	result["sign"] = signature
	return result, nil
}

// VerifyNotification validates V1's MD5 signature, merchant and successful trade
// status. There is no protocol timestamp/replay window; durable idempotency belongs
// to the business Store. All additional scalar fields participate in verification.
func (c *V1Client) VerifyNotification(values url.Values) (*Notification, error) {
	params := make(Params, len(values))
	for key, items := range values {
		if len(items) != 1 {
			return nil, fmt.Errorf("epay v1: notification parameter %s must occur once", key)
		}
		params[key] = items[0]
	}
	if params["sign_type"] != "MD5" {
		return nil, errors.New("epay v1: missing or unsupported sign_type")
	}
	signature, err := v1Signature(c.key, params)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(signature), []byte(params["sign"])) != 1 {
		return nil, errors.New("epay v1: signature verification failed")
	}
	if params["pid"] != c.merchantID || params["trade_status"] != "TRADE_SUCCESS" {
		return nil, errors.New("epay v1: notification merchant or trade status mismatch")
	}
	for _, key := range []string{"trade_no", "out_trade_no", "type", "name", "money"} {
		if strings.TrimSpace(params[key]) == "" {
			return nil, fmt.Errorf("epay v1: notification missing %s", key)
		}
	}
	if err := positiveAmount(params["money"]); err != nil {
		return nil, err
	}
	return &Notification{
		MerchantID: params["pid"], TradeNo: params["trade_no"], OutTradeNo: params["out_trade_no"],
		APITradeNo: params["api_trade_no"], Type: params["type"], Money: params["money"],
		Param: params["param"], Buyer: params["buyer"], Fields: params,
	}, nil
}
