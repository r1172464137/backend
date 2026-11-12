package adminuser

import (
	"context"
	"os"
	"slices"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type BatchDeleteUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newBatchDeleteUserLogic(ctx context.Context, deps Deps) *BatchDeleteUserLogic {
	return &BatchDeleteUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *BatchDeleteUserLogic) BatchDeleteUser(req *dto.BatchDeleteUserRequest) error {
	isDemo := strings.ToLower(os.Getenv("PPANEL_MODE")) == "demo"

	if slices.Contains(req.Ids, 2) && isDemo {
		return errors.Wrapf(xerr.NewErrCodeMsg(503, "Demo mode does not allow deletion of the admin user"), "BatchDeleteUser failed: cannot delete admin user in demo mode")
	}

	if err := ensureAnotherAdministrator(l.ctx, l.deps.Users, req.Ids...); err != nil {
		return err
	}
	err := l.deps.Users.BatchDeleteUser(l.ctx, req.Ids)
	if err != nil {
		l.Logger.Error("[BatchDeleteUserLogic] BatchDeleteUser failed: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "BatchDeleteUser failed: %v", err.Error())
	}
	l.clearDeletedUserAccessCaches(req.Ids)
	return nil
}
