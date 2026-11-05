package systemsetting

import (
	"context"
	"encoding/json"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

const interfaceConfigKey = "AdminHiddenMenuIds"

// Keep these IDs in sync with the leaf URLs in the admin navigation.
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
	if req == nil || req.HiddenMenuIds == nil || len(req.HiddenMenuIds) > len(hideableMenuIDs) {
		return fmt.Errorf("hiddenMenuIds must be an array of menu IDs")
	}
	seen := make(map[string]bool)
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
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), err.Error())
	}
	value, err := json.Marshal(req.HiddenMenuIds)
	if err != nil {
		return err
	}
	if err := s.deps.System.UpdateValueByCategoryKey(ctx, "interface", interfaceConfigKey, string(value), "string"); err != nil {
		return errors.Wrap(xerr.NewErrCode(xerr.DatabaseUpdateError), err.Error())
	}
	return nil
}
