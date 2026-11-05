package config

import (
	"context"
	"errors"
	"testing"
)

type routingAccountsRepo struct {
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
func TestRoutingAccountPresenceAndMissingType(t *testing.T) {
	if HasRoutingAccounts(nil) || HasRoutingAccounts(&NodeRouting{}) {
		t.Fatal("empty routing has accounts")
	}
	p := &NodeRouting{UserDefaults: []NodeUserDefault{{UserID: 7}}, Rules: []NodeRoute{{UserIDs: []int64{9}}}}
	if !HasRoutingAccounts(p) {
		t.Fatal("missing account presence")
	}
	err := ValidateNewRoutingAccounts(context.Background(), routingAccountsRepo{existing: map[int64]bool{9: true}}, nil, p)
	var missing *RoutingAccountNotFoundError
	if !errors.As(err, &missing) || missing.ID != 7 {
		t.Fatalf("missing account error = %v", err)
	}
	if err := ValidateNewRoutingAccounts(context.Background(), nil, p, p); err != nil {
		t.Fatalf("existing references require no lookup: %v", err)
	}
}

func TestValidateNewRoutingAccounts(t *testing.T) {
	p := func(ids ...int64) *NodeRouting {
		return &NodeRouting{Rules: []NodeRoute{{UserIDs: ids}}}
	}
	for _, tc := range []struct {
		name      string
		old, next *NodeRouting
		repo      RoutingAccountLookup
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
