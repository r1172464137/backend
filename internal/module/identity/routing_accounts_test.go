package identity

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/perfect-panel/server/internal/repository"
)

type routingLookupFake struct {
	repository.UserRepo
	got []int64
	err error
}

func (r *routingLookupFake) FindExistingUserIDs(_ context.Context, ids []int64) ([]int64, error) {
	r.got = ids
	return []int64{2}, r.err
}

func TestRoutingAccountsFacadeDelegates(t *testing.T) {
	failure := errors.New("lookup failed")
	repo := &routingLookupFake{err: failure}
	svc := &service{accounts: accounts{users: repo}}
	ids, err := svc.FindExistingUserIDs(context.Background(), []int64{2, 3})
	if !errors.Is(err, failure) || !reflect.DeepEqual(ids, []int64{2}) || !reflect.DeepEqual(repo.got, []int64{2, 3}) {
		t.Fatalf("ids=%v err=%v input=%v", ids, err, repo.got)
	}
}
