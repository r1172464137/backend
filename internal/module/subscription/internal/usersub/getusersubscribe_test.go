package usersub

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	subEntity "github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
)

type idsLookupRepo struct {
	repository.UserSubscriptionRepo
	gotIds []int64
}

func (r *idsLookupRepo) FindSubscribeDetailsByIds(_ context.Context, ids []int64) ([]*subEntity.SubscribeDetails, error) {
	r.gotIds = ids
	return []*subEntity.SubscribeDetails{{Id: 7, Token: "tok"}}, nil
}

// TestGetUserSubscribeByIds pins the admin cell batch path: ids bypass the
// per-user lookup (the embedded nil repo would panic if it were used).
func TestGetUserSubscribeByIds(t *testing.T) {
	repo := &idsLookupRepo{}
	resp, err := newGetUserSubscribeLogic(context.Background(), Deps{UserSubs: repo}).
		GetUserSubscribe(&dto.GetUserSubscribeListRequest{Page: 1, Size: 2, Ids: []int64{7, 8}})
	if err != nil {
		t.Fatalf("GetUserSubscribe: %v", err)
	}
	if len(repo.gotIds) != 2 || len(resp.List) != 1 || resp.List[0].Id != 7 || resp.List[0].Token != "tok" {
		t.Fatalf("ids=%v list=%+v", repo.gotIds, resp.List)
	}
}
