package protocol

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var amountPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,2})?$`)

// ParseAmount converts a decimal yuan string into integer cents without floating
// point. Zero is valid for balances; payment/refund wrappers require a positive amount.
func ParseAmount(value string) (int64, error) {
	if !amountPattern.MatchString(value) {
		return 0, errors.New("epay: amount must be a nonnegative decimal with at most two fraction digits")
	}
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > math.MaxInt64/100 {
		return 0, errors.New("epay: amount exceeds int64 cents")
	}
	var fraction int64
	if len(parts) == 2 {
		fraction, _ = strconv.ParseInt((parts[1] + "0")[:2], 10, 64)
	}
	if whole*100 > math.MaxInt64-fraction {
		return 0, errors.New("epay: amount exceeds int64 cents")
	}
	return whole*100 + fraction, nil
}

func positiveAmount(value string) error {
	cents, err := ParseAmount(value)
	if err != nil {
		return err
	}
	if cents == 0 {
		return errors.New("epay: amount must be positive")
	}
	return nil
}
