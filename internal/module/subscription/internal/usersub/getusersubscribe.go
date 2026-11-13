package usersub

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/protocolkey"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	subEntity "github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserSubscribeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user subcribe
func newGetUserSubscribeLogic(ctx context.Context, deps Deps) *GetUserSubscribeLogic {
	return &GetUserSubscribeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserSubscribeLogic) GetUserSubscribe(req *dto.GetUserSubscribeListRequest) (resp *dto.GetUserSubscribeListResponse, err error) {
	var data []*subEntity.SubscribeDetails
	if len(req.Ids) > 0 {
		data, err = l.deps.UserSubs.FindSubscribeDetailsByIds(l.ctx, req.Ids)
	} else {
		data, err = l.deps.UserSubs.QueryUserSubscribe(l.ctx, req.UserId, 0, 1, 2, 3, 4, 5)
	}
	if err != nil {
		l.Errorw("[GetUserSubscribeLogs] Get User Subscribe Error:", logger.Field("err", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Get User Subscribe Error")
	}

	resp = &dto.GetUserSubscribeListResponse{
		List:  make([]dto.UserSubscribe, 0),
		Total: int64(len(data)),
	}

	for _, item := range data {
		var sub dto.UserSubscribe
		mapping.DeepCopy(&sub, item)
		sub.Short, _ = protocolkey.FixedUniqueString(item.Token, 8, "")
		resp.List = append(resp.List, sub)
	}
	return
}
