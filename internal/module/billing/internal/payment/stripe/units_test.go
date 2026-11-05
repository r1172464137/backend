package stripe

import "testing"

// Stripe counts JPY in whole yen and KWD in fils (thousandths); the system
// counts hundredths of every currency. A charge sent in the wrong unit is
// off by a factor of 100 or 10.
func TestStripeUnits(t *testing.T) {
	for _, tc := range []struct {
		currency           string
		hundredths, stripe int64
	}{
		{"usd", 1990, 1990},
		{"JPY", 100000, 1000},
		{"krw", 500000, 5000},
		{"KWD", 1250, 12500},
	} {
		if got := ToStripeAmount(tc.hundredths, tc.currency); got != tc.stripe {
			t.Errorf("ToStripeAmount(%d, %s) = %d, want %d", tc.hundredths, tc.currency, got, tc.stripe)
		}
		if got := FromStripeAmount(tc.stripe, tc.currency); got != tc.hundredths {
			t.Errorf("FromStripeAmount(%d, %s) = %d, want %d", tc.stripe, tc.currency, got, tc.hundredths)
		}
	}
}

// A zero-decimal charge is rounded to whole units before it is recorded, so
// the amount Stripe reports back converts to exactly the expectation.
func TestRoundHundredthsRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		currency string
		in, want int64
	}{
		{"JPY", 100050, 100100},
		{"JPY", 100049, 100000},
		{"JPY", 100000, 100000},
		{"USD", 100050, 100050},
		{"KWD", 1255, 1255},
	} {
		got := RoundHundredths(tc.in, tc.currency)
		if got != tc.want {
			t.Errorf("RoundHundredths(%d, %s) = %d, want %d", tc.in, tc.currency, got, tc.want)
		}
		if back := FromStripeAmount(ToStripeAmount(got, tc.currency), tc.currency); back != got {
			t.Errorf("%s %d does not survive the round trip: %d", tc.currency, got, back)
		}
	}
}
