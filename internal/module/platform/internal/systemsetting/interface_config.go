package systemsetting

import (
	"context"
	"encoding/json"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

const interfaceConfigKey = "AdminHiddenMenuIds"

// Hideable admin leaves; system settings is absent so it always stays reachable.
var hideableMenuIDs = map[string]bool{
	"/dashboard": true, "/dashboard/servers": true, "/dashboard/nodes": true,
	"/dashboard/subscribe": true, "/dashboard/product": true, "/dashboard/order": true,
	"/dashboard/withdrawal": true, "/dashboard/coupon": true, "/dashboard/marketing": true,
	"/dashboard/announcement": true, "/dashboard/user": true, "/dashboard/ticket": true,
	"/dashboard/document": true, "/dashboard/auth-control": true, "/dashboard/payment": true,
	"/dashboard/ads": true, "/dashboard/log/login": true, "/dashboard/log/email": true,
	"/dashboard/log/server-traffic": true, "/dashboard/log/balance": true,
}

func validateInterfaceConfig(req *dto.InterfaceConfig) error {
	if req == nil || req.HiddenMenuIds == nil {
		return fmt.Errorf("hiddenMenuIds must be an array")
	}
	seen := map[string]bool{}
	for _, id := range req.HiddenMenuIds {
		if !hideableMenuIDs[id] || seen[id] {
			return fmt.Errorf("invalid or duplicate menu ID: %s", id)
		}
		seen[id] = true
	}
	return nil
}

func (s *Service) GetInterfaceConfig(ctx context.Context) (*dto.InterfaceConfig, error) {
	rows, err := s.deps.System.GetInterfaceConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.DatabaseQueryError), err.Error())
	}
	result := &dto.InterfaceConfig{HiddenMenuIds: []string{}}
	if len(rows) == 0 {
		return result, nil
	}
	if len(rows) != 1 || json.Unmarshal([]byte(rows[0].Value), &result.HiddenMenuIds) != nil || validateInterfaceConfig(result) != nil {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.DatabaseQueryError), "invalid interface configuration")
	}
	return result, nil
}

func (s *Service) UpdateInterfaceConfig(ctx context.Context, req *dto.InterfaceConfig) error {
	if err := validateInterfaceConfig(req); err != nil {
		return xerr.NewErrCodeMsg(xerr.InvalidParams, err.Error())
	}
	value, _ := json.Marshal(req.HiddenMenuIds)
	err := s.deps.Store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		rows, err := tx.System().GetInterfaceConfig(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 { // the row does not exist until the first save
			return tx.System().Insert(ctx, &system.System{Category: "interface", Key: interfaceConfigKey, Value: string(value), Type: "string", Desc: "Admin menu IDs hidden from navigation"})
		}
		return tx.System().UpdateValueByCategoryKey(ctx, "interface", interfaceConfigKey, string(value), "string")
	})
	if err != nil {
		return errors.Wrap(xerr.NewErrCode(xerr.DatabaseUpdateError), err.Error())
	}
	return nil
}
