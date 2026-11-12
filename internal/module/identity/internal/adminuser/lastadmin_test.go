package adminuser

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
)

type adminList []*user.User

func (a adminList) QueryAdminUsers(context.Context) ([]*user.User, error) { return a, nil }

func admin(id int64, enabled bool) *user.User {
	yes := true
	return &user.User{Id: id, IsAdmin: &yes, Enable: &enabled}
}

func TestEnsureAnotherAdministrator(t *testing.T) {
	ctx := context.Background()
	one := adminList{admin(1, true), admin(2, false)}
	if ensureAnotherAdministrator(ctx, one, 1) == nil {
		t.Error("last enabled administrator removable")
	}
	if ensureAnotherAdministrator(ctx, one, 2) != nil {
		t.Error("disabled administrator not removable")
	}
	two := adminList{admin(1, true), admin(3, true)}
	if ensureAnotherAdministrator(ctx, two, 1) != nil {
		t.Error("one of two enabled administrators not removable")
	}
	if ensureAnotherAdministrator(ctx, two, 1, 3) == nil {
		t.Error("batch removing every enabled administrator accepted")
	}
	if ensureAnotherAdministrator(ctx, one, 42) != nil {
		t.Error("a non-administrator edit refused")
	}
}
