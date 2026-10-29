package adminserver

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/routingstatus"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type FilterServerListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewFilterServerListLogic Filter Server List
func newFilterServerListLogic(ctx context.Context, deps Deps) *FilterServerListLogic {
	return &FilterServerListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *FilterServerListLogic) FilterServerList(req *dto.FilterServerListRequest) (resp *dto.FilterServerListResponse, err error) {
	nodeStore := l.deps.Store.Node()
	total, data, err := nodeStore.FilterServerList(l.ctx, &node.FilterParams{
		Page:   req.Page,
		Size:   req.Size,
		Search: req.Search,
	})
	if err != nil {
		l.Errorw("[FilterServerList] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[FilterServerList] Query Database Error")
	}

	list := make([]dto.Server, 0)

	for _, datum := range data {
		var server dto.Server
		mapping.DeepCopy(&server, datum)

		// handler protocols
		var protocols []dto.Protocol
		dst, err := datum.UnmarshalProtocols()
		if err != nil {
			l.Errorf("[FilterServerList] UnmarshalProtocols Error: %s", err.Error())
			continue
		}
		mapping.DeepCopy(&protocols, dst)
		server.Protocols = protocols

		nodeStatus, err := nodeStore.StatusCache(l.ctx, datum.Id)
		if err != nil && !errors.Is(err, redis.Nil) {
			// A Redis outage must not turn stale routing status into success or
			// make the entire server inventory disappear. Metrics are unknown.
			l.Errorw("[handlerServerStatus] GetNodeStatus Error: ", logger.Field("error", err.Error()), logger.Field("node_id", datum.Id))
		}
		server.Status = dto.ServerStatus{
			Mem:    nodeStatus.Mem,
			Cpu:    nodeStatus.Cpu,
			Disk:   nodeStatus.Disk,
			Online: l.handlerServerStatus(datum.Id, protocols),
			Status: l.handlerServerStaus(datum.LastReportedAt),
		}
		status, err := routingstatus.Read(l.ctx, l.deps.Redis, datum.Id)
		if err != nil {
			status = dto.RoutingStatusView{Known: false}
		}
		server.Status.Routing = status
		list = append(list, server)
	}

	return &dto.FilterServerListResponse{
		List:  list,
		Total: total,
	}, nil
}

func (l *FilterServerListLogic) handlerServerStatus(id int64, protocols []dto.Protocol) []dto.ServerOnlineUser {
	result := make([]dto.ServerOnlineUser, 0)
	nodeStore := l.deps.Store.Node()
	userSubscriptions := l.deps.Store.UserSubscription()

	for _, protocol := range protocols {
		// query online user
		data, err := nodeStore.OnlineUserSubscribe(l.ctx, id, protocol.Type)
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				l.Errorw("[handlerServerStatus] OnlineUserSubscribe Error: ", logger.Field("error", err.Error()), logger.Field("node_id", id), logger.Field("protocol", protocol.Type))
			}
			continue
		}
		if len(data) > 0 {
			for sub, online := range data {
				var ips []dto.ServerOnlineIP
				for _, ip := range online {
					ips = append(ips, dto.ServerOnlineIP{
						IP:       ip,
						Protocol: protocol.Type,
					})
				}

				result = append(result, dto.ServerOnlineUser{
					IP:          ips,
					SubscribeId: sub,
				})
			}
		}
	}
	// merge same subscribe
	var mapResult = make(map[int64]dto.ServerOnlineUser)
	for _, item := range result {
		if exist, ok := mapResult[item.SubscribeId]; ok {
			// merge
			exist.Traffic += item.Traffic
			exist.IP = append(exist.IP, item.IP...)
			mapResult[item.SubscribeId] = exist
		} else {
			// get subscribe info
			info, err := userSubscriptions.FindOneUserSubscribe(l.ctx, item.SubscribeId)
			if err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					l.Errorw("[handlerServerStatus] FindOneSubscribe Error: ", logger.Field("error", err.Error()), logger.Field("subscribe_id", item.SubscribeId))
				}
				continue
			}
			data := dto.ServerOnlineUser{
				IP:          item.IP,
				UserId:      info.UserId,
				Subscribe:   "",
				SubscribeId: item.SubscribeId,
				Traffic:     info.Download + info.Upload,
				ExpiredAt:   info.ExpireTime.UnixMilli(),
			}
			if info.Subscribe != nil {
				data.Subscribe = info.Subscribe.Name
			}
			// add new
			mapResult[item.SubscribeId] = data
		}
	}
	// convert map to slice
	result = make([]dto.ServerOnlineUser, 0, len(mapResult))
	for _, item := range mapResult {
		result = append(result, item)
	}
	return result
}

func (l *FilterServerListLogic) handlerServerStaus(last *time.Time) string {
	if last == nil {
		return "offline"
	}
	if time.Since(*last) > time.Minute*5 {
		return "offline"
	}
	if time.Since(*last) > time.Minute*3 {
		return "warning"
	}
	return "online"

}
