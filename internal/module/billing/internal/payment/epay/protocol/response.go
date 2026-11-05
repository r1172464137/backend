package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
)

// APIError reports a V2 nonzero code. Verified=false means the error was unsigned:
// it is diagnostic information, not authenticated evidence of a financial outcome.
type APIError struct {
	Code     int
	Message  string
	Verified bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("epay: API code=%d verified=%t: %s", e.Code, e.Verified, e.Message)
}

// Response is only returned after validating a code=0 response's signature and
// timestamp. Nested arrays/objects are NOT covered by this upstream protocol.
type Response struct {
	raw            []byte
	UnsignedFields []string
}

func (r *Response) Decode(destination any) error { return json.Unmarshal(r.raw, destination) }
func (r *Response) JSON() []byte                 { return append([]byte(nil), r.raw...) }

// Parse the top-level object without accepting duplicate keys or trailing JSON.
// Keep numbers as raw JSON so large order identifiers never pass through float64.
func responseObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("epay: response must be a JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("epay: invalid JSON property")
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("epay: duplicate JSON property %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("epay: unexpected data after JSON object")
	}
	return fields, nil
}

var jsonInteger = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func responseParams(fields map[string]json.RawMessage) (Params, []string, error) {
	params := make(Params)
	var unsigned []string
	for key, raw := range fields {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			return nil, nil, errors.New("epay: empty JSON value")
		}
		switch raw[0] {
		case '{', '[': // PHP json_decode(..., true) produces arrays for both.
			unsigned = append(unsigned, key)
		case '"':
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, nil, err
			}
			params[key] = value
		default:
			switch string(raw) {
			case "null", "false":
				params[key] = "" // PHP string conversion, excluded as empty.
			case "true":
				params[key] = "1"
			default:
				if !jsonInteger.Match(raw) {
					return nil, nil, fmt.Errorf("epay: non-integer JSON number in %s; amounts must be strings", key)
				}
				params[key] = string(raw)
			}
		}
	}
	sort.Strings(unsigned)
	return params, unsigned, nil
}

func (c *Client) parseResponse(data []byte) (*Response, error) {
	fields, err := responseObject(data)
	if err != nil {
		return nil, err
	}
	var code int
	rawCode, exists := fields["code"]
	if !exists || string(bytes.TrimSpace(rawCode)) == "null" || json.Unmarshal(rawCode, &code) != nil {
		return nil, errors.New("epay: missing or non-integer response code")
	}
	params, unsigned, err := responseParams(fields)
	if err != nil {
		return nil, err
	}
	// The PHP SDK accepts unsigned failures but verifies all successful replies.
	// An invalid supplied signature is never downgraded to an unsigned API error.
	_, hasSignature := fields["sign"]
	verified := false
	if code == 0 || hasSignature {
		if err := c.verify(params); err != nil {
			return nil, err
		}
		verified = true
	}
	if code != 0 {
		return nil, &APIError{Code: code, Message: params["msg"], Verified: verified}
	}
	return &Response{raw: append([]byte(nil), data...), UnsignedFields: unsigned}, nil
}
