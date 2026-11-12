package plan

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type DeleteSubscribeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Delete subscribe
func newDeleteSubscribeLogic(ctx context.Context, deps Deps) *DeleteSubscribeLogic {
	return &DeleteSubscribeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// currentStatuses (pending, active, exhausted in term) may still be renewed,
// reset or delivered from the plan, so they block its deletion (98365fb3).
var currentStatuses = []int64{0, 1, 2}

func (l *DeleteSubscribeLogic) DeleteSubscribe(req *dto.DeleteSubscribeRequest) error {
	// Check if the subscribe exists
	phase := "check"
	err := l.deps.Store.InSubscriptionTx(l.ctx, func(store repository.SubscriptionStore) error {
		total, err := store.UserSubscription().CountUserSubscribesBySubscribeIdAndStatus(l.ctx, req.Id, currentStatuses...)
		if err != nil {
			return err
		}
		if total != 0 {
			return errorIsExistActiveUser
		}
		phase = "delete"
		return store.Subscribe().Delete(l.ctx, req.Id)
	})
	if err != nil {
		if errors.Is(err, errorIsExistActiveUser) {
			return errors.Wrapf(xerr.NewErrCode(xerr.SubscribeIsUsedError), "subscribe is used")
		}
		if phase == "delete" {
			l.Logger.Error("[DeleteSubscribeLogic] delete subscribe failed: ", logger.Field("error", err.Error()))
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete subscribe failed: %v", err.Error())
		}
		l.Logger.Error("[DeleteSubscribeLogic] check subscribe failed: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "check subscribe failed: %v", err.Error())
	}
	return nil
}
