package order

import (
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"math/big"
	"time"
)

const (
	tradeNoTimestampLayout = "20060102150405"
	tradeNoSuffixSpace     = 100_000_000
)

// GenerateTradeNo returns a fixed-width numeric trade number: a 14-digit
// timestamp plus an 8-digit value sampled uniformly with crypto/rand. The
// random suffix reduces collisions but does not promise uniqueness; the
// database's unique order_no constraint remains the final arbiter.
func GenerateTradeNo() string {
	tradeNo, err := generateTradeNo(time.Now(), cryptorand.Reader)
	if err != nil {
		panic(fmt.Errorf("generate trade number entropy: %w", err))
	}
	return tradeNo
}

// generateTradeNo accepts its clock and entropy as values so the format and
// entropy contract can be tested without probabilistic uniqueness assertions.
func generateTradeNo(now time.Time, entropy io.Reader) (string, error) {
	suffix, err := cryptorand.Int(entropy, big.NewInt(tradeNoSuffixSpace))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%08d", now.Format(tradeNoTimestampLayout), suffix.Int64()), nil
}
