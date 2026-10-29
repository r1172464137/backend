package adminserver

import (
	"context"
	"errors"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
)

type lostRedisNodes struct{ repository.NodeRepo }

func (lostRedisNodes) FilterServerList(context.Context, *node.FilterParams) (int64, []*node.Server, error) {
	return 1, []*node.Server{{Id: 7, Name: "node", Protocols: `[]`}}, nil
}
func (lostRedisNodes) StatusCache(context.Context, int64) (node.Status, error) {
	return node.Status{}, errors.New("redis unavailable")
}

type lostRedisStore struct {
	repository.NetworkTransactor
	repository.NodeRepo
}

func (s lostRedisStore) Node() repository.NodeRepo                       { return s.NodeRepo }
func (lostRedisStore) UserSubscription() repository.UserSubscriptionRepo { return nil }
func TestServerListShowsUnknownRoutingWhenRedisLost(t *testing.T) {
	service := NewService(Deps{Store: lostRedisStore{NodeRepo: lostRedisNodes{}}})
	result, err := service.FilterServerList(context.Background(), &dto.FilterServerListRequest{Page: 1, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.List) != 1 || result.List[0].Status.Routing.Known || result.List[0].Status.Routing.ReporterVerified {
		t.Fatalf("status %#v", result)
	}
}
