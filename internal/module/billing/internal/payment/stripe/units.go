package stripe

import "strings"

// Stripe counts amounts in each currency's smallest unit, which is not always
// a hundredth: zero-decimal currencies (JPY, KRW, …) count whole units and
// three-decimal ones (BHD, KWD, …) count thousandths. The rest of the system
// counts hundredths of every currency, so amounts are converted here, at the
// edge, in both directions.
var (
	zeroDecimalCurrencies = map[string]bool{
		"BIF": true, "CLP": true, "DJF": true, "GNF": true, "JPY": true, "KMF": true,
		"KRW": true, "MGA": true, "PYG": true, "RWF": true, "UGX": true, "VND": true,
		"VUV": true, "XAF": true, "XOF": true, "XPF": true,
	}
	threeDecimalCurrencies = map[string]bool{
		"BHD": true, "JOD": true, "KWD": true, "OMR": true, "TND": true,
	}
)

// RoundHundredths rounds an amount in hundredths of currency to what Stripe
// can collect: whole units for a zero-decimal currency (half up), unchanged
// otherwise. A charge must be rounded before its expectation is recorded, so
// the amount Stripe reports back matches it exactly.
func RoundHundredths(hundredths int64, currency string) int64 {
	if zeroDecimalCurrencies[strings.ToUpper(currency)] {
		return (hundredths + 50) / 100 * 100
	}
	return hundredths
}

// ToStripeAmount converts hundredths of currency into Stripe's unit. The
// amount must already be rounded with RoundHundredths.
func ToStripeAmount(hundredths int64, currency string) int64 {
	switch c := strings.ToUpper(currency); {
	case zeroDecimalCurrencies[c]:
		return hundredths / 100
	case threeDecimalCurrencies[c]:
		return hundredths * 10
	}
	return hundredths
}

// FromStripeAmount converts an amount in Stripe's unit into hundredths of
// currency.
func FromStripeAmount(amount int64, currency string) int64 {
	switch c := strings.ToUpper(currency); {
	case zeroDecimalCurrencies[c]:
		return amount * 100
	case threeDecimalCurrencies[c]:
		return amount / 10
	}
	return amount
}
