package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A signal that arrives while the infrastructure connections are being
// retried cancels the start: getServers returns the context error instead of
// waiting for the retry budget or starting the services.
func TestGetServersReturnsACanceledStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppanel.yaml")
	content := "AppLocation: UTC\n" +
		"JwtAuth:\n  AccessSecret: test-secret-long-enough\n" +
		"Database:\n  Driver: mysql\n  Addr: 127.0.0.1:1\n  Dbname: ppanel\n" +
		"Redis:\n  Host: 127.0.0.1:1\n" +
		"GeoIP:\n  Download: false\n" +
		"Logger:\n  Mode: console\n  Level: severe\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	useConfigPath(t, path)
	oldLocal := time.Local
	t.Cleanup(func() { time.Local = oldLocal })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	services, err := getServers(ctx)

	if services != nil {
		t.Fatalf("getServers returned %+v although the start was canceled", services)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("getServers error = %v, want context.Canceled", err)
	}
}
