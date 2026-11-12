package storefront

import (
	"context"
	"strings"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type QueryUserSubscribeNodeListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user subscribe node info
func newQueryUserSubscribeNodeListLogic(ctx context.Context, deps Deps) *QueryUserSubscribeNodeListLogic {
	return &QueryUserSubscribeNodeListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryUserSubscribeNodeListLogic) QueryUserSubscribeNodeList() (resp *dto.QueryUserSubscribeNodeListResponse, err error) {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}

	userSubscribes, err := l.deps.UserSubs.QueryUserSubscribe(l.ctx, u.Id, 1, 2)
	if err != nil {
		logger.Errorw("failed to query user subscribe", logger.Field("error", err.Error()), logger.Field("user_id", u.Id))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "DB_ERROR")
	}

	resp = &dto.QueryUserSubscribeNodeListResponse{}
	nodesByPlan := make(map[int64][]*node.Node)
	for _, us := range userSubscribes {
		if us == nil {
			continue
		}
		userSubscribe := subscribeFromDetails(us)
		nodes, err := l.getServers(userSubscribe, us.Subscribe, nodesByPlan)
		if err != nil {
			return nil, err
		}
		userSubscribeInfo := dto.UserSubscribeInfo{
			Id:          userSubscribe.Id,
			Nodes:       nodes,
			Traffic:     userSubscribe.Traffic,
			Upload:      userSubscribe.Upload,
			Download:    userSubscribe.Download,
			Token:       userSubscribe.Token,
			UserId:      userSubscribe.UserId,
			OrderId:     userSubscribe.OrderId,
			SubscribeId: userSubscribe.SubscribeId,
			StartTime:   userSubscribe.StartTime.Unix(),
			ExpireTime:  userSubscribe.ExpireTime.Unix(),
			Status:      userSubscribe.Status,
			CreatedAt:   userSubscribe.CreatedAt.Unix(),
			UpdatedAt:   userSubscribe.UpdatedAt.Unix(),
		}

		if userSubscribe.FinishedAt != nil {
			userSubscribeInfo.FinishedAt = userSubscribe.FinishedAt.Unix()
		}

		if l.deps.isTrialPlan(userSubscribe.SubscribeId) {
			userSubscribeInfo.IsTryOut = true
		}

		resp.List = append(resp.List, userSubscribeInfo)
	}

	return
}

func subscribeFromDetails(item *usersub.SubscribeDetails) *usersub.Subscribe {
	if item == nil {
		return nil
	}
	return &usersub.Subscribe{
		Id: item.Id, UserId: item.UserId, OrderId: item.OrderId, SubscribeId: item.SubscribeId,
		StartTime: item.StartTime, ExpireTime: item.ExpireTime, FinishedAt: item.FinishedAt,
		Traffic: item.Traffic, Download: item.Download, Upload: item.Upload,
		Token: item.Token, UUID: item.UUID, Status: item.Status, Note: item.Note,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func (l *QueryUserSubscribeNodeListLogic) getServers(userSub *usersub.Subscribe, subDetails *subscribe.Subscribe, nodesByPlan map[int64][]*node.Node) (userSubscribeNodes []*dto.UserSubscribeNodeInfo, err error) {
	userSubscribeNodes = make([]*dto.UserSubscribeNodeInfo, 0)
	if l.isSubscriptionExpired(userSub) || l.isTrafficExhausted(userSub) {
		return l.createExpiredServers(), nil
	}

	if subDetails == nil {
		subDetails, err = l.deps.Plans.FindOne(l.ctx, userSub.SubscribeId)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return userSubscribeNodes, nil // plan deleted: no nodes left
		}
		if err != nil {
			l.Errorw("[Generate Subscribe]find subscribe details error: %v", logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find subscribe details error: %v", err.Error())
		}
	}
	nodeIds := slicesx.StringToInt64Slice(subDetails.Nodes)
	tags := strings.Split(subDetails.NodeTags, ",")
	cleanTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			cleanTags = append(cleanTags, tag)
		}
	}
	tags = cleanTags

	l.Debugf("[Generate Subscribe]nodes: %v, NodeTags: %v", nodeIds, tags)

	enable := true

	nodes, cached := nodesByPlan[subDetails.Id]
	if !cached {
		nodes, err = l.deps.Nodes.ListNodesByScope(l.ctx, nodeIds, tags, &enable, true)
		if err != nil {
			l.Errorw("[Generate Subscribe]find server details error: %v", logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server details error: %v", err.Error())
		}
		nodesByPlan[subDetails.Id] = nodes
	}

	if len(nodes) > 0 {
		for _, n := range nodes {
			server := n.Server
			if server == nil {
				continue
			}
			userSubscribeNode := &dto.UserSubscribeNodeInfo{
				Id:        n.Id,
				Name:      n.Name,
				Uuid:      userSub.UUID,
				Protocol:  n.Protocol,
				Port:      n.Port,
				Address:   n.Address,
				Tags:      strings.Split(n.Tags, ","),
				Country:   server.Country,
				City:      server.City,
				CreatedAt: n.CreatedAt.Unix(),
			}
			userSubscribeNodes = append(userSubscribeNodes, userSubscribeNode)
		}
	}

	l.Debugf("[Query Subscribe]found servers: %v", len(nodes))
	logger.Debugf("[Generate Subscribe]found servers: %v", len(nodes))
	return userSubscribeNodes, nil
}

func (l *QueryUserSubscribeNodeListLogic) isSubscriptionExpired(userSub *usersub.Subscribe) bool {
	return userSub.ExpireTime.Unix() < timeutil.Now().Unix() && userSub.ExpireTime.Unix() != 0
}

// isTrafficExhausted reports whether the subscription has used up its traffic
// quota (Traffic == 0 means unlimited).
func (l *QueryUserSubscribeNodeListLogic) isTrafficExhausted(userSub *usersub.Subscribe) bool {
	return userSub.Traffic > 0 && userSub.Download+userSub.Upload >= userSub.Traffic
}

func (l *QueryUserSubscribeNodeListLogic) createExpiredServers() []*dto.UserSubscribeNodeInfo {
	return nil
}

func (l *QueryUserSubscribeNodeListLogic) getFirstHostLine() string {
	host := l.deps.Host
	lines := strings.Split(host, "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return host
}
