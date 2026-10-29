package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/geolocation"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateServerLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateServerLogic Update Server
func newUpdateServerLogic(ctx context.Context, deps Deps) *UpdateServerLogic {
	return &UpdateServerLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateServerLogic) UpdateServer(req *dto.UpdateServerRequest) error {
	nodeStore := l.deps.Store.Node()
	data, err := nodeStore.FindOneServer(l.ctx, req.Id)
	if err != nil {
		l.Errorf("[UpdateServer] FindOneServer Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server error: %v", err.Error())
	}
	data.Name = req.Name
	data.Country = req.Country
	data.City = req.City
	// only update address when it's  different
	if req.Address != data.Address {
		// query server ip location
		result, err := geolocation.GetRegionByIp(req.Address)
		if err != nil {
			l.Errorf("[UpdateServer] GetRegionByIp Error: %v", err.Error())
		} else {
			data.City = result.City
			data.Country = result.Country
		}
		// update address
		data.Address = req.Address
	}
	existingProtocols, err := data.UnmarshalProtocols()
	if err != nil {
		l.Errorf("[UpdateServer] Unmarshal Protocols Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols unmarshal error"), "protocols unmarshal error: %v", err)
	}
	existingKeys := protocolKeyLookup(existingProtocols)
	existingServerKeys := serverKeyLookup(existingProtocols)
	existingRealityKeys := realityProtocolKeyLookup(existingProtocols)
	existingProtocolLookup := protocolLookup(existingProtocols)
	protocols := make([]node.Protocol, 0)
	for index, item := range req.Protocols {
		if item.Type == "" {
			code := xerr.InvalidParams
			if item.Enable {
				code = xerr.NetworkNodeProtocolInvalid
			}
			return errors.Wrapf(xerr.NewErrCodeMsg(code, "protocols type is empty"), "protocols type is empty")
		}
		var protocol node.Protocol
		mapping.DeepCopy(&protocol, item)
		if existing, ok := existingProtocolLookup[normalizedProtocolType(item.Type)]; ok {
			protocol, err = mergeMissingProtocolFields(protocol, existing, protocolFieldSetAt(req.ProtocolFieldSets, index))
			if err != nil {
				return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols merge error"), "protocols merge error: %v", err)
			}
		}
		ensureGeneratedProtocolKey(&protocol, existingKeys)
		ensureShadowsocks2022ServerKey(&protocol, existingServerKeys)
		if err := ensureRealityProtocolKey(&protocol, existingRealityKeys); err != nil {
			l.Errorf("[UpdateServer] Generate Reality Key Error: %v", err.Error())
			return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "generate reality key error: %v", err)
		}
		ensureRealityProtocolDefaults(&protocol)
		protocols = append(protocols, protocol)
	}
	if err := validateEnabledNodeProtocols(protocols); err != nil {
		return err
	}
	for i := range protocols {
		protocol, err := node.NormalizeProtocolForStorage(protocols[i])
		if err != nil {
			code := xerr.InvalidParams
			if protocols[i].Enable {
				code = xerr.NetworkNodeProtocolInvalid
			}
			return errors.Wrapf(xerr.NewErrCodeMsg(code, err.Error()), "protocols normalize error: %v", err)
		}
		protocols[i] = protocol
	}
	err = data.MarshalProtocols(protocols)
	if err != nil {
		l.Errorf("[UpdateServer] Marshal Protocols Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols marshal error"), "protocols marshal error: %v", err)
	}

	err = nodeStore.UpdateServer(l.ctx, data)
	if err != nil {
		l.Errorf("[UpdateServer] UpdateServer Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server error: %v", err.Error())
	}

	// The server-user cache depends on node/plan/user rows, not server
	// listener, name, address or override fields. Node configuration is read
	// from the authoritative DB, so no post-commit cache invalidation is
	// needed for this write.
	return nil
}
