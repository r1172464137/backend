package application

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateSubscribeApplicationLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateSubscribeApplicationLogic Update subscribe application
func newUpdateSubscribeApplicationLogic(ctx context.Context, deps Deps) *UpdateSubscribeApplicationLogic {
	return &UpdateSubscribeApplicationLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateSubscribeApplicationLogic) UpdateSubscribeApplication(req *dto.UpdateSubscribeApplicationRequest) (resp *dto.SubscribeApplication, err error) {
	if err := validateOutputFormat(req.OutputFormat); err != nil {
		return nil, err
	}
	data, err := l.deps.Clients.FindOne(l.ctx, req.Id)
	if err != nil {
		l.Errorf("Failed to find subscribe application with ID %d: %v", req.Id, err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Failed to find subscribe application with ID %d", req.Id)
	}
	var link client.DownloadLink
	mapping.DeepCopy(&link, req.DownloadLink)
	linkData, err := link.Marshal()
	if err != nil {
		l.Errorf("Failed to marshal download link: %v", err)
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), " Failed to marshal download link")
	}

	data.Name = req.Name
	data.Icon = req.Icon
	data.Description = req.Description
	data.Scheme = req.Scheme
	data.UserAgent = req.UserAgent
	data.IsDefault = req.IsDefault
	data.SubscribeTemplate = req.SubscribeTemplate
	data.OutputFormat = req.OutputFormat
	data.DefaultParams = req.DefaultParams
	data.DownloadLink = string(linkData)
	err = l.deps.Clients.Update(l.ctx, data)
	if err != nil {
		l.Errorf("Failed to update subscribe application with ID %d: %v", req.Id, err)
		return nil, saveError(err, xerr.DatabaseUpdateError, "Failed to update subscribe application")
	}
	resp = &dto.SubscribeApplication{}
	mapping.DeepCopy(&resp, data)
	resp.DownloadLink = req.DownloadLink
	return
}
