package adminuser

import (
	"context"
	"os"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type DeleteUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newDeleteUserLogic(ctx context.Context, deps Deps) *DeleteUserLogic {
	return &DeleteUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *DeleteUserLogic) DeleteUser(req *dto.GetDetailRequest) error {
	isDemo := strings.ToLower(os.Getenv("PPANEL_MODE")) == "demo"

	if req.Id == 2 && isDemo {
		return errors.Wrapf(xerr.NewErrCodeMsg(503, "Demo mode does not allow deletion of the admin user"), "delete user failed: cannot delete admin user in demo mode")
	}
	if err := ensureAnotherAdministrator(l.ctx, l.deps.Users, req.Id); err != nil {
		return err
	}
	err := l.deps.Users.Delete(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete user error: %v", err.Error())
	}
	l.clearDeletedUserAccessCaches([]int64{req.Id})
	return nil
}
