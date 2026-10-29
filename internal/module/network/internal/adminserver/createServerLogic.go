package adminserver

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/geolocation"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type CreateServerLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newCreateServerLogic(ctx context.Context, deps Deps) *CreateServerLogic {
	return &CreateServerLogic{Logger: logger.WithContext(ctx), ctx: ctx, deps: deps}
}

// Return only the committed ID: generated private keys must not be exposed in
// the create response, and clients need the ID to copy server-local config.
func validateEnabledNodeProtocols(protocols []node.Protocol) error {
	body, err := json.Marshal(protocols)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.NetworkNodeProtocolInvalid, "invalid protocol encoding"), "encode protocols: %v", err)
	}
	if err := node.ValidateNodeProtocols(&node.Server{Protocols: string(body)}); err != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.NetworkNodeProtocolInvalid, err.Error()), "node protocol validation: %v", err)
	}
	return nil
}

func (l *CreateServerLogic) CreateServer(req *dto.CreateServerRequest) (*dto.CreateServerResponse, error) {
	if req == nil {
		return nil, xerr.NewErrCodeMsg(xerr.InvalidParams, "nil request")
	}
	data := node.Server{Name: req.Name, Country: req.Country, City: req.City, Address: req.Address, Sort: req.Sort, Protocols: ""}
	protocols := make([]node.Protocol, 0, len(req.Protocols))
	for _, item := range req.Protocols {
		if item.Type == "" {
			code := xerr.InvalidParams
			if item.Enable {
				code = xerr.NetworkNodeProtocolInvalid
			}
			return nil, errors.Wrapf(xerr.NewErrCodeMsg(code, "protocols type is empty"), "protocols type is empty")
		}
		var protocol node.Protocol
		mapping.DeepCopy(&protocol, item)
		ensureGeneratedProtocolKey(&protocol, nil)
		ensureShadowsocks2022ServerKey(&protocol, nil)
		if err := ensureRealityProtocolKey(&protocol, nil); err != nil {
			l.Errorf("[CreateServer] Generate Reality Key Error: %v", err)
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "generate reality key error: %v", err)
		}
		ensureRealityProtocolDefaults(&protocol)
		protocols = append(protocols, protocol)
	}
	if err := validateEnabledNodeProtocols(protocols); err != nil {
		return nil, err
	}
	for i := range protocols {
		protocol, err := node.NormalizeProtocolForStorage(protocols[i])
		if err != nil {
			code := xerr.InvalidParams
			if protocols[i].Enable {
				code = xerr.NetworkNodeProtocolInvalid
			}
			return nil, errors.Wrapf(xerr.NewErrCodeMsg(code, err.Error()), "protocols normalize error: %v", err)
		}
		protocols[i] = protocol
	}
	if err := data.MarshalProtocols(protocols); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols marshal error"), "protocols marshal error: %v", err)
	}
	if data.City == "" && data.Country == "" {
		result, err := geolocation.GetRegionByIp(req.Address)
		if err != nil {
			l.Errorf("[CreateServer] GetRegionByIp Error: %v", err)
		} else {
			data.City = result.City
			data.Country = result.Country
		}
	}
	if err := l.deps.Store.Node().InsertServer(l.ctx, &data); err != nil {
		l.Errorf("[CreateServer] Insert Server error: %v", err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert server error: %v", err)
	}
	return &dto.CreateServerResponse{ID: data.Id}, nil
}
