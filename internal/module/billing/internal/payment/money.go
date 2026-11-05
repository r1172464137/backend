package payment

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Money on the order side is always an int64 count of minor units (cents).
// Gateways exchange decimal strings; FormatAmount and ParseAmount are the only
// conversions between the two and ConvertAmount is the only currency
// conversion, so the amount recorded as a payment expectation is exactly the
// amount a gateway is asked to collect.

// FormatAmount renders minor units as the two-decimal string payment gateways
// exchange: 1990 is "19.90" and 5 is "0.05". It never goes through a float.
func FormatAmount(minor int64) string {
	sign := ""
	magnitude := uint64(minor)
	if minor < 0 {
		sign = "-"
		magnitude = uint64(-(minor + 1)) + 1
	}
	cents := strconv.FormatUint(magnitude%100, 10)
	if len(cents) == 1 {
		cents = "0" + cents
	}
	return sign + strconv.FormatUint(magnitude/100, 10) + "." + cents
}

// ConvertAmount converts minor units of one currency into minor units of
// another at rate, the price of one source unit in target units; both
// currencies count hundredths. The rate is taken at its shortest decimal
// representation, which is the value the rate provider published rather than
// the nearest binary fraction, and the exact product is rounded to the
// nearest target minor unit with halves rounded away from zero. The result
// therefore never differs from the exact conversion by more than half a minor
// unit, and the same inputs always produce the same charge.
func ConvertAmount(minor int64, rate float64) (int64, error) {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0, errors.New("invalid exchange rate")
	}
	exact, ok := new(big.Rat).SetString(strconv.FormatFloat(rate, 'g', -1, 64))
	if !ok {
		return 0, errors.New("invalid exchange rate")
	}
	exact.Mul(exact, new(big.Rat).SetInt64(minor))
	numerator, denominator := exact.Num(), exact.Denom()
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if twice := new(big.Int).Lsh(new(big.Int).Abs(remainder), 1); twice.Cmp(denominator) >= 0 {
		if numerator.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	if !quotient.IsInt64() {
		return 0, errors.New("converted amount out of range")
	}
	return quotient.Int64(), nil
}

// ParseAmount converts a non-negative decimal currency amount to its integer
// minor unit. Financial callback code must not compare parsed float64 values.
func ParseAmount(value string) (int64, error) {
	if value == "" || len(value) > 20 || strings.TrimSpace(value) != value {
		return 0, errors.New("invalid money format")
	}
	wholePart, fractionalPart, hasFraction := strings.Cut(value, ".")
	if wholePart == "" || !decimalDigits(wholePart) {
		return 0, errors.New("invalid money format")
	}
	if hasFraction {
		if fractionalPart == "" || len(fractionalPart) > 2 || !decimalDigits(fractionalPart) {
			return 0, errors.New("invalid money format")
		}
	}
	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil || whole > math.MaxInt64/100 {
		return 0, errors.New("money amount out of range")
	}
	fraction := int64(0)
	if hasFraction {
		if len(fractionalPart) == 1 {
			fractionalPart += "0"
		}
		fraction, err = strconv.ParseInt(fractionalPart, 10, 64)
		if err != nil {
			return 0, errors.New("invalid money format")
		}
	}
	minorUnits := whole * 100
	if fraction > math.MaxInt64-minorUnits {
		return 0, errors.New("money amount out of range")
	}
	return minorUnits + fraction, nil
}

func decimalDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
