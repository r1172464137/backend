package payment

import (
	"math"
	"testing"
)

// Every amount a customer can be charged must survive the trip to the
// gateway string and back unchanged; the float formatter this replaced
// turned 58 into 57 and 1990 into 1989.
func TestFormatAmountRoundTripsEveryAmount(t *testing.T) {
	for amount := int64(0); amount <= 100000; amount++ {
		formatted := FormatAmount(amount)
		parsed, err := ParseAmount(formatted)
		if err != nil || parsed != amount {
			t.Fatalf("FormatAmount(%d) = %q parses as (%d, %v)", amount, formatted, parsed, err)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	tests := map[int64]string{
		0: "0.00", 5: "0.05", 58: "0.58", 100: "1.00", 1990: "19.90", 123456: "1234.56",
		-1: "-0.01", -1990: "-19.90",
		math.MaxInt64: "92233720368547758.07",
		math.MinInt64: "-92233720368547758.08",
	}
	for amount, want := range tests {
		if got := FormatAmount(amount); got != want {
			t.Errorf("FormatAmount(%d) = %q, want %q", amount, got, want)
		}
	}
}

func TestConvertAmountRoundsHalfAwayFromZero(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		rate   float64
		want   int64
	}{
		{"identity", 1990, 1, 1990},
		{"exact product", 1000, 7.25, 7250},
		// 1 × 7.125 = 7.125 → 7; 2 × 7.125 = 14.25 → 14; 3 × 7.125 = 21.375 → 21
		{"below half rounds down", 3, 7.125, 21},
		// 7 × 0.5 = 3.5 → 4
		{"half rounds up", 7, 0.5, 4},
		// 5 × 0.7 = 3.5 exactly at the published rate, not 3.4999… of the binary 0.7
		{"half of a decimal rate", 5, 0.7, 4},
		// 58 × 1 = 58: the case the float formatter truncated to 57
		{"historical truncation", 58, 1, 58},
		// 10000 × 7.1234 = 71234 exactly although 7.1234 is not a binary fraction
		{"decimal rate stays exact", 10000, 7.1234, 71234},
		// 1 × 0.004 = 0.004 → 0
		{"tiny amount", 1, 0.004, 0},
		{"zero", 0, 7.1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertAmount(tt.amount, tt.rate)
			if err != nil || got != tt.want {
				t.Fatalf("ConvertAmount(%d, %v) = (%d, %v), want %d", tt.amount, tt.rate, got, err, tt.want)
			}
		})
	}
}

func TestConvertAmountRejectsUnusableRates(t *testing.T) {
	for _, rate := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := ConvertAmount(100, rate); err == nil {
			t.Errorf("ConvertAmount accepted rate %v", rate)
		}
	}
	if _, err := ConvertAmount(math.MaxInt64, 2); err == nil {
		t.Error("ConvertAmount accepted a result beyond int64")
	}
}

// Converting the same amount twice must give the same charge: the checkout
// records the conversion as the payment expectation and the gateway is asked
// for exactly that amount.
func TestConvertAmountIsDeterministicAcrossAmounts(t *testing.T) {
	const rate = 7.2419
	for amount := int64(1); amount <= 100000; amount++ {
		first, err := ConvertAmount(amount, rate)
		if err != nil {
			t.Fatal(err)
		}
		second, _ := ConvertAmount(amount, rate)
		exact := float64(amount) * rate
		if first != second || math.Abs(float64(first)-exact) > 0.5+1e-6 {
			t.Fatalf("ConvertAmount(%d) = %d/%d, exact %.4f", amount, first, second, exact)
		}
	}
}

func TestParseAmountUsesExactMinorUnits(t *testing.T) {
	tests := []struct {
		value   string
		want    int64
		wantErr bool
	}{
		{value: "0", want: 0},
		{value: "10", want: 1000},
		{value: "10.1", want: 1010},
		{value: "10.01", want: 1001},
		{value: "1.001", wantErr: true},
		{value: "-1.00", wantErr: true},
		{value: "1e2", wantErr: true},
		{value: " 1.00", wantErr: true},
		{value: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := ParseAmount(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseAmount(%q) error=%v", test.value, err)
			}
			if err == nil && got != test.want {
				t.Fatalf("ParseAmount(%q)=%d, want %d", test.value, got, test.want)
			}
		})
	}
}
