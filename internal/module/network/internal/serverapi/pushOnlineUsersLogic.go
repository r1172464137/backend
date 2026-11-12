package serverapi

import (
	"context"
	"errors"
	"fmt"
	"net"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/logger"
)

type PushOnlineUsersLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewPushOnlineUsersLogic Push online users
func newPushOnlineUsersLogic(ctx context.Context, deps Deps) *PushOnlineUsersLogic {
	return &PushOnlineUsersLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *PushOnlineUsersLogic) PushOnlineUsers(req *dto.OnlineUsersRequest) error {
	// 验证请求数据
	if req.ServerId <= 0 || len(req.Users) == 0 {
		return errors.New("invalid request parameters")
	}

	// 验证用户数据
	// Bounds from upstream 886be78b: entries per report, IP syntax, IPs per SID.
	if len(req.Users) > 10000 {
		return fmt.Errorf("too many online users in one report: %d", len(req.Users))
	}
	for i, user := range req.Users {
		ip := net.ParseIP(user.IP)
		if user.SID <= 0 || ip == nil {
			return fmt.Errorf("invalid user data: uid=%d, ip=%.45q", user.SID, user.IP)
		}
		req.Users[i].IP = ip.String()
	}

	// Find server info
	_, err := l.deps.Store.Node().FindOneServer(l.ctx, req.ServerId)
	if err != nil {
		l.Errorw("[PushOnlineUsers] FindOne error", logger.Field("error", err))
		return fmt.Errorf("server not found: %w", err)
	}

	onlineUsers := make(node.OnlineUserSubscribe)
	for _, user := range req.Users {
		if online, ok := onlineUsers[user.SID]; ok {
			if len(online) >= 64 {
				continue
			}
			// If user already exists, update IP if different
			online = append(online, user.IP)
			onlineUsers[user.SID] = online
		} else {
			// New user, add to map
			onlineUsers[user.SID] = []string{user.IP}
		}
	}
	err = l.deps.Store.Node().UpdateOnlineUserSubscribe(l.ctx, req.ServerId, req.Protocol, onlineUsers)
	if err != nil {
		l.Errorw("[PushOnlineUsers] cache operation error", logger.Field("error", err))
		return err
	}

	err = l.deps.Store.Node().UpdateOnlineUserSubscribeGlobal(l.ctx, onlineUsers)

	if err != nil {
		l.Errorw("[PushOnlineUsers] cache operation error", logger.Field("error", err))
		return err
	}

	return nil
}
