package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository/kernel"
)

// ReadNodeConfigSnapshot is the cross-module view and excludes NodeSecret.
func (s *Service) ReadNodeConfigSnapshot(ctx context.Context) (config.NodeConfig, error) {
	return ReadNodeConfigSnapshotFromRepo(ctx, s.deps.System)
}

func ReadNodeConfigSnapshotFromRepo(ctx context.Context, repo kernel.SystemRepo) (config.NodeConfig, error) {
	rows, err := repo.GetNodeConfigUncached(ctx)
	if err != nil {
		return config.NodeConfig{}, err
	}
	out, err := system.ParseNodeConfig(rows)
	if err != nil {
		return config.NodeConfig{}, err
	}
	out.NodeSecret = ""
	return out, nil
}
