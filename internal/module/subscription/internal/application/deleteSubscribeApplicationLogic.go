package application

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type DeleteSubscribeApplicationLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewDeleteSubscribeApplicationLogic Delete subscribe application
func newDeleteSubscribeApplicationLogic(ctx context.Context, deps Deps) *DeleteSubscribeApplicationLogic {
	return &DeleteSubscribeApplicationLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeleteSubscribeApplicationLogic) DeleteSubscribeApplication(req *dto.DeleteSubscribeApplicationRequest) error {
	err := l.deps.Clients.Delete(l.ctx, req.Id)
	if err != nil {
		l.Errorf("Failed to delete subscribe application with ID %d: %v", req.Id, err)
		return saveError(err, xerr.DatabaseDeletedError, err.Error())
	}
	return nil
}
