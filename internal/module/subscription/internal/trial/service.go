// Package trial grants the registration trial subscription. It consumes the
// identity.user_registered event: the grant runs in a subscription-domain
// transaction, idempotent via the inbox marker, serialized per user by the
// subscription serial lock. Only the module facade may reach it.
package trial

import (
	"context"
	"fmt"
	"uuid"

	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

const inboxTrialGrant = "subscription.trial_grant"

// Policy is the per-call view of the runtime-mutable trial settings.
type Policy struct {
	Enabled  bool
	PlanID   int64
	Duration int64
	TimeUnit string
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Plans repository.SubscribeRepo
	Cache repository.UserCacheRepo
	Store Store
	// TrialPolicy snapshots the runtime-mutable trial settings per call.
	TrialPolicy func() Policy
}

// Service is the trial-grant entry point used by the subscription facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// GrantTrial applies the registration trial exactly once for the user. A
// disabled trial still consumes the event (the marker records the decision),
// so a later policy change never retroactively grants trials to old
// registrations — matching the old in-transaction behavior where the policy
// was evaluated at registration time.
func (s *Service) GrantTrial(ctx context.Context, userID int64) error {
	policy := s.deps.TrialPolicy()
	var granted *usersub.Subscribe
	err := s.deps.Store.InSubscriptionTx(ctx, func(store repository.SubscriptionStore) error {
		mark, err := store.Inbox().Find(ctx, inboxTrialGrant, fmt.Sprintf("%d", userID))
		if err != nil {
			return err
		}
		if mark != nil {
			return nil
		}
		if policy.Enabled {
			// Serialize with fulfillment and other grants for this user.
			if err := store.UserSubscription().LockUserSerial(ctx, userID); err != nil {
				return err
			}
			plan, err := store.Subscribe().FindOne(ctx, policy.PlanID)
			if err != nil {
				return err
			}
			now := timeutil.Now()
			granted = &usersub.Subscribe{
				UserId:      userID,
				OrderId:     0,
				SubscribeId: plan.Id,
				StartTime:   now,
				ExpireTime:  timeutil.AddTime(policy.TimeUnit, policy.Duration, now),
				Traffic:     plan.Traffic,
				Token:       usersub.TokenFromOrder(fmt.Sprintf("Trial-%v-%s", userID, uuid.NewV7().String())),
				UUID:        uuid.NewV4().String(),
				Status:      usersub.SubscribeStatusActive,
			}
			if err := store.UserSubscription().InsertSubscribe(ctx, granted); err != nil {
				return err
			}
		}
		return store.Inbox().Insert(ctx, inboxTrialGrant, fmt.Sprintf("%d", userID), "")
	})
	if err != nil {
		return err
	}
	if granted != nil {
		if err := s.deps.Cache.ClearSubscribeCache(ctx, granted); err != nil {
			logger.WithContext(ctx).Errorw("[TrialGrant] ClearSubscribeCache failed", logger.Field("error", err.Error()))
		}
		if err := s.deps.Plans.ClearCache(ctx, granted.SubscribeId); err != nil {
			logger.WithContext(ctx).Errorw("[TrialGrant] Clear plan cache failed", logger.Field("error", err.Error()))
		}
	}
	return nil
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.SubscriptionTransactor
}
