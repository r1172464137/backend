package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/config"
)

type routingAccountsRepo struct {
	UserRepo
	existing map[int64]bool
	err      error
}

func (r routingAccountsRepo) FindExistingUserIDs(_ context.Context, ids []int64) ([]int64, error) {
	if r.err != nil {
		return nil, r.err
	}
	out := make([]int64, 0)
	for _, id := range ids {
		if r.existing[id] {
			out = append(out, id)
		}
	}
	return out, nil
}
func TestValidateNewRoutingAccounts(t *testing.T) {
	p := func(ids ...int64) *config.NodeRouting {
		return &config.NodeRouting{Rules: []config.NodeRoute{{UserIDs: ids}}}
	}
	for _, tc := range []struct {
		name      string
		old, next *config.NodeRouting
		repo      UserRepo
		ok        bool
	}{
		{"existing disabled allowed", nil, p(1), routingAccountsRepo{existing: map[int64]bool{1: true}}, true},
		{"new missing rejected", nil, p(2), routingAccountsRepo{existing: map[int64]bool{}}, false},
		{"deleted reference retained", p(2), p(2), nil, true},
		{"lookup failure rejected", nil, p(1), routingAccountsRepo{err: errors.New("db")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateNewRoutingAccounts(t.Context(), tc.repo, tc.old, tc.next); (err == nil) != tc.ok {
				t.Fatalf("error %v", err)
			}
		})
	}
}
