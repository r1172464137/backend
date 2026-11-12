package delivery

import (
	"context"
	"unicode"

	"github.com/perfect-panel/server/internal/auth/ratelimit"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// Ported from upstream 98365fb3: refuse what can never be a token before any
// lookup, and limit fetches per client address (60/min) so token guessing
// costs one attempt per second; an unreachable limiter admits the fetch.
const (
	minTokenLength     = 8
	FetchRateWindowSec = 60
	FetchRateQuota     = 60
	FetchRateKeyPrefix = "subscribe:deliver:ip:"
)

// FetchLimiter is satisfied by *ratelimit.PeriodLimit.
type FetchLimiter interface {
	TakeCtx(ctx context.Context, key string) (int, error)
}

func acceptableToken(raw string) bool {
	if len(raw) < minTokenLength {
		return false
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (l *SubscribeLogic) admitFetch() error {
	if l.deps.Limiter == nil || l.request.ClientIP == "" {
		return nil
	}
	state, err := l.deps.Limiter.TakeCtx(l.ctx, l.request.ClientIP)
	if err != nil {
		l.Errorw("[SubscribeLogic] Fetch rate limiter unavailable; admitting the fetch", logger.Field("error", err.Error()))
		return nil
	}
	if state == ratelimit.OverQuota {
		l.Infow("[SubscribeLogic] Fetch rate exceeded", logger.Field("client_ip", l.request.ClientIP))
		return errors.Wrap(xerr.NewErrCode(xerr.TooManyRequests), "too many subscription fetches")
	}
	return nil
}
