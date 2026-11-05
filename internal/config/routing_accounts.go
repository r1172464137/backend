package config

import (
	"context"
	"fmt"
	"sort"
)

// RoutingAccountLookup resolves existing account IDs, including disabled accounts.
type RoutingAccountLookup interface {
	FindExistingUserIDs(context.Context, []int64) ([]int64, error)
}

func HasRoutingAccounts(p *NodeRouting) bool {
	if p == nil {
		return false
	}
	if len(p.UserDefaults) > 0 {
		return true
	}
	for _, r := range p.Rules {
		if len(r.UserIDs) > 0 {
			return true
		}
	}
	return false
}

type RoutingAccountNotFoundError struct{ ID int64 }

func (e *RoutingAccountNotFoundError) Error() string { return fmt.Sprintf("unknown account %d", e.ID) }

// ValidateNewRoutingAccounts checks only newly introduced references: a deleted
// account already present in a saved package remains inert and editable.
func ValidateNewRoutingAccounts(ctx context.Context, users RoutingAccountLookup, previous, next *NodeRouting) error {
	collect := func(p *NodeRouting) map[int64]bool {
		m := map[int64]bool{}
		if p == nil {
			return m
		}
		for _, d := range p.UserDefaults {
			m[d.UserID] = true
		}
		for _, r := range p.Rules {
			for _, id := range r.UserIDs {
				m[id] = true
			}
		}
		return m
	}
	old := collect(previous)
	current := collect(next)
	ids := make([]int64, 0, len(current))
	for id := range current {
		if !old[id] {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if users == nil {
		return fmt.Errorf("account lookup unavailable")
	}
	rows, err := users.FindExistingUserIDs(ctx, ids)
	if err != nil {
		return err
	}
	found := make(map[int64]bool, len(rows))
	for _, id := range rows {
		found[id] = true
	}
	for _, id := range ids {
		if !found[id] {
			return &RoutingAccountNotFoundError{ID: id}
		}
	}
	return nil
}
