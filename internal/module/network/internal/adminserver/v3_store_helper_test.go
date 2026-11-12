package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
)

// serializedTx exposes the stub repositories inside InTx.
type serializedTx struct {
	repository.Store
	owner *serializedAdminNodeStore
}

func (t serializedTx) Node() repository.NodeRepo { return t.owner.nodeRepo }

func (s *serializedAdminNodeStore) InTx(_ context.Context, fn func(repository.Store) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(serializedTx{owner: s})
}

// knownUsers: account IDs 1..999 exist; 1000 and above do not.
type knownUsers struct{ repository.UserRepo }

func (knownUsers) FindUsersByIds(_ context.Context, ids []int64) ([]*user.User, error) {
	var out []*user.User
	for _, id := range ids {
		if id > 0 && id < 1000 {
			out = append(out, &user.User{Id: id})
		}
	}
	return out, nil
}

func (s *serializedAdminNodeStore) User() repository.UserRepo { return knownUsers{} }

func (r *serializedNodeRepo) ListNodes(context.Context, *node.FilterNodeParams) ([]*node.Node, error) {
	return nil, nil
}
