package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger/logtest"
)

func TestNodeReloadKeepsPreviousSettingsForInvalidServerDNS(t *testing.T) {
	logtest.Discard(t)
	deps, runtime := newHarness(healthyStore(), config.Config{})
	deps.ValidateServerDNS = func(context.Context) error { return errors.New("invalid server DNS") }
	if err := Node(context.Background(), deps); err == nil {
		t.Fatal("invalid stored override accepted")
	}
	if runtime.config.Node.NodeSecret != "" || runtime.managerSettings != 0 {
		t.Fatal("published partial node settings")
	}
}
