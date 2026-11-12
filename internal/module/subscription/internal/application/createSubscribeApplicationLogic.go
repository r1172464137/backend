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

type CreateSubscribeApplicationLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewCreateSubscribeApplicationLogic Create subscribe application
func newCreateSubscribeApplicationLogic(ctx context.Context, deps Deps) *CreateSubscribeApplicationLogic {
	return &CreateSubscribeApplicationLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CreateSubscribeApplicationLogic) CreateSubscribeApplication(req *dto.CreateSubscribeApplicationRequest) (resp *dto.SubscribeApplication, err error) {
	if err := validateOutputFormat(req.OutputFormat); err != nil {
		return nil, err
	}
	var link client.DownloadLink
	mapping.DeepCopy(&link, req.DownloadLink)
	linkData, err := link.Marshal()
	if err != nil {
		l.Errorf("Failed to marshal download link: %v", err)
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), " Failed to marshal download link")
	}
	data := &client.SubscribeApplication{
		Name:              req.Name,
		Icon:              req.Icon,
		Description:       req.Description,
		Scheme:            req.Scheme,
		UserAgent:         req.UserAgent,
		IsDefault:         req.IsDefault,
		SubscribeTemplate: req.SubscribeTemplate,
		OutputFormat:      req.OutputFormat,
		DefaultParams:     req.DefaultParams,
		DownloadLink:      string(linkData),
	}

	err = l.deps.Clients.Insert(l.ctx, data)
	if err != nil {
		l.Errorf("Failed to create subscribe application: %v", err)
		return nil, saveError(err, xerr.DatabaseInsertError, "Failed to create subscribe application")
	}

	resp = &dto.SubscribeApplication{}
	mapping.DeepCopy(resp, data)
	resp.DownloadLink = req.DownloadLink

	return
}
