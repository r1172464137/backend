package adminuser

import (
	"context"
	"slices"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type administrators interface {
	QueryAdminUsers(ctx context.Context) ([]*user.User, error)
}

// ensureAnotherAdministrator refuses demoting, disabling or deleting the last
// enabled administrator: nobody could re-enable anyone through the API after
// that (upstream 286099f5).
func ensureAnotherAdministrator(ctx context.Context, admins administrators, losing ...int64) error {
	all, err := admins.QueryAdminUsers(ctx)
	if err != nil {
		return errors.Wrap(xerr.NewErrCode(xerr.DatabaseQueryError), err.Error())
	}
	affected := false
	for _, admin := range all {
		if !isEnabledAdministrator(admin) {
			continue
		}
		if !slices.Contains(losing, admin.Id) {
			return nil
		}
		affected = true
	}
	if !affected {
		return nil
	}
	return xerr.NewErrCodeMsg(xerr.InvalidParams, "the last enabled administrator cannot be demoted, disabled or deleted")
}

func isEnabledAdministrator(u *user.User) bool {
	return u != nil && u.IsAdmin != nil && *u.IsAdmin && u.Enable != nil && *u.Enable
}
