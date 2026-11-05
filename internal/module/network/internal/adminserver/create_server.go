package adminserver

import (
	"context"
	"encoding/json"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/geolocation"
	"github.com/perfect-panel/server/internal/module/network/internal/protocolmap"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// CreateServer stores a new server with its protocols, generating the keys
// the protocols need. A server given neither a country nor a city is located
// by its address.
func (s *Service) CreateServer(ctx context.Context, req *dto.CreateServerRequest) (*dto.CreateServerResponse, error) {
	if req == nil {
		return nil, xerr.NewErrCodeMsg(xerr.InvalidParams, "nil request")
	}
	log := logger.WithContext(ctx)
	data := node.Server{
		Name:      req.Name,
		Country:   req.Country,
		City:      req.City,
		Address:   req.Address,
		Sort:      req.Sort,
		Protocols: "",
	}
	protocols := make([]node.Protocol, 0)
	for _, item := range req.Protocols {
		if item.Type == "" {
			code := xerr.InvalidParams
			if item.Enable {
				code = xerr.NetworkNodeProtocolInvalid
			}
			return nil, fmt.Errorf("protocols type is empty: %w", xerr.NewErrCodeMsg(code, "protocols type is empty"))
		}
		protocol, err := protocolmap.FromDTO(item)
		if err != nil {
			return nil, err
		}
		ensureGeneratedProtocolKey(&protocol, nil)
		ensureShadowsocks2022ServerKey(&protocol, nil)
		if err := ensureRealityProtocolKey(&protocol, nil); err != nil {
			log.Errorf("[CreateServer] Generate Reality Key Error: %v", err.Error())
			return nil, xerr.Wrapf(err, xerr.ERROR, "generate reality key error: %v", err)
		}
		ensureRealityProtocolDefaults(&protocol)
		protocol, err = node.NormalizeProtocolForStorage(protocol)
		if err != nil {
			return nil, fmt.Errorf("protocols normalize error: %w: %w", err, xerr.NewErrCodeMsg(xerr.InvalidParams, err.Error()))
		}
		protocols = append(protocols, protocol)
	}

	if err := validateEnabledNodeProtocols(protocols); err != nil {
		return nil, err
	}
	if err := data.MarshalProtocols(protocols); err != nil {
		log.Errorf("[CreateServer] Marshal Protocols Error: %v", err.Error())
		return nil, fmt.Errorf("protocols marshal error: %w: %w", err, xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols marshal error"))
	}
	if data.City == "" && data.Country == "" {
		// A failed lookup only leaves the location empty.
		result, err := geolocation.GetRegionByIp(ctx, req.Address)
		if err != nil {
			log.Errorf("[CreateServer] GetRegionByIp Error: %v", err.Error())
		} else {
			data.City = result.City
			data.Country = result.Country
		}
	}
	if err := s.deps.Store.Node().InsertServer(ctx, &data); err != nil {
		log.Errorf("[CreateServer] Insert Server error: %v", err.Error())
		return nil, xerr.Wrapf(err, xerr.DatabaseInsertError, "insert server error: %v", err)
	}
	return &dto.CreateServerResponse{ID: data.Id}, nil
}

func validateEnabledNodeProtocols(protocols []node.Protocol) error {
	body, err := json.Marshal(protocols)
	if err != nil {
		return xerr.Wrapf(err, xerr.NetworkNodeProtocolInvalid, "encode protocols")
	}
	if err := node.ValidateNodeProtocols(&node.Server{Protocols: string(body)}); err != nil {
		return xerr.Wrapf(err, xerr.NetworkNodeProtocolInvalid, "node protocol validation")
	}
	return nil
}
