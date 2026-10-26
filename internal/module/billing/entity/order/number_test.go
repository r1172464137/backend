package order

import (
	"bytes"
	"testing"
	"time"
)

func TestGenerateTradeNoPublicFormat(t *testing.T) {
	assertNumericTradeNo(t, GenerateTradeNo())
}

func TestGenerateTradeNoFormat(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		entropy []byte
		want    string
	}{
		{name: "leading zeroes", entropy: []byte{0, 0, 0, 0}, want: "2026010203040500000000"},
		{name: "maximum suffix", entropy: []byte{0x05, 0xf5, 0xe0, 0xff}, want: "2026010203040599999999"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			no, err := generateTradeNo(now, bytes.NewReader(test.entropy))
			if err != nil {
				t.Fatalf("generate trade number: %v", err)
			}
			if no != test.want {
				t.Fatalf("trade number = %q, want %q", no, test.want)
			}
			assertNumericTradeNo(t, no)
		})
	}
}

func TestGenerateTradeNoUsesEntropy(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	first, err := generateTradeNo(now, bytes.NewReader([]byte{0, 0, 0, 0}))
	if err != nil {
		t.Fatalf("generate first trade number: %v", err)
	}
	second, err := generateTradeNo(now, bytes.NewReader([]byte{0, 0, 0, 1}))
	if err != nil {
		t.Fatalf("generate second trade number: %v", err)
	}
	if first == second {
		t.Fatalf("different entropy produced the same trade number %q", first)
	}
}

func TestGenerateTradeNoEntropyFailure(t *testing.T) {
	_, err := generateTradeNo(time.Unix(0, 0), bytes.NewReader(nil))
	if err == nil {
		t.Fatal("missing entropy must return an error")
	}
}

func assertNumericTradeNo(t *testing.T, no string) {
	t.Helper()
	if len(no) != 22 {
		t.Fatalf("trade number %q has length %d, want fixed 22", no, len(no))
	}
	for i, c := range no {
		if c < '0' || c > '9' {
			t.Fatalf("trade number %q has non-digit %q at %d", no, c, i)
		}
	}
}
