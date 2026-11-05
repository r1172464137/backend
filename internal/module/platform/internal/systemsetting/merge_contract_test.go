package systemsetting

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

func TestRoutingCredentialsRoundTripAndOrdinarySave(t *testing.T) {
	for _, secret := range []string{"secret-password", dto.SecretMask} {
		t.Run(secret, func(t *testing.T) { checkRoutingCredentialsRoundTrip(t, secret) })
	}
}

func checkRoutingCredentialsRoundTrip(t *testing.T, secret string) {
	t.Helper()
	w := newSettingsWorld(t)
	ctx := adminContext()
	id := "ob_11111111-1111-4111-8111-111111111111"
	policy := &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{{ID: id, Name: "proxy", Protocol: "socks", Address: "127.0.0.1", Port: 1080, Password: secret, UUID: secret, EncryptionPassword: secret}}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}
	if err := w.svc.UpdateNodeConfig(ctx, &dto.NodeConfig{Routing: policy, RoutingRevision: 1}); err != nil {
		t.Fatal(err)
	}
	expected, err := config.NormalizeNodeRouting(policy)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.GetNodeConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Routing, expected) {
		t.Fatalf("read = %+v, want %+v", got.Routing, expected)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var req dto.NodeConfig
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.UpdateNodeConfig(ctx, &req); err != nil {
		t.Fatal(err)
	}
	stored := w.storedValue(t, "server", "Routing")
	parsed, err := config.ParseNodeRouting([]byte(stored))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, expected) {
		t.Fatalf("saved = %+v, want %+v", parsed, expected)
	}
	var partial dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"node_push_interval":42}`), &partial); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.UpdateNodeConfig(ctx, &partial); err != nil {
		t.Fatal(err)
	}
	if after := w.storedValue(t, "server", "Routing"); after != stored {
		t.Fatalf("ordinary update changed routing: %s", after)
	}
	if rev := w.storedValue(t, "server", "RoutingRevision"); rev != "3" {
		t.Fatalf("revision %s", rev)
	}
	// Copying a literal mask to a new outbound is a normal credential write.
	copied := *parsed
	outbound := parsed.Outbounds[0]
	outbound.ID = "ob_22222222-2222-4222-8222-222222222222"
	outbound.Name = "copy"
	copied.Outbounds = append(copied.Outbounds, outbound)
	if err := w.svc.UpdateNodeConfig(ctx, &dto.NodeConfig{Routing: &copied, RoutingRevision: 3}); err != nil {
		t.Fatalf("copy: %v", err)
	}
	read, err := w.svc.GetNodeConfig(ctx)
	if err != nil || read.Routing.Outbounds[1].Password != secret {
		t.Fatalf("copy read = %+v, %v", read, err)
	}
}

func TestNodeUpdateRepairsOnlyDNSNotCutover(t *testing.T) {
	w := newSettingsWorld(t)
	ctx := context.Background()
	if err := w.system.UpdateValueByCategoryKey(ctx, "server", "DNS", "{bad"); err != nil {
		t.Fatal(err)
	}
	var req dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"dns":[]}`), &req); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.UpdateNodeConfig(ctx, &req); err != nil {
		t.Fatalf("repair DNS: %v", err)
	}
	if got := w.storedValue(t, "server", "DNS"); got != "[]" {
		t.Fatalf("DNS = %s", got)
	}
	if err := w.system.UpdateValueByCategoryKey(ctx, "server", "RoutingCutover", "pending"); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.UpdateNodeConfig(ctx, &req); err == nil {
		t.Fatal("accepted incomplete cutover")
	}
}

func TestNodeReloadFailureIsSavedNotApplied(t *testing.T) {
	w := newSettingsWorld(t)
	w.runtime.reloadErr = errors.New("reload failed")
	var req dto.NodeConfig
	if err := json.Unmarshal([]byte(`{"node_push_interval":32}`), &req); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.UpdateNodeConfig(context.Background(), &req); xerr.CodeOf(err) != xerr.SettingsSavedNotApplied {
		t.Fatalf("error %v", err)
	}
	if got := w.storedValue(t, "server", "NodePushInterval"); got != "32" {
		t.Fatalf("stored %s", got)
	}
}

func TestInterfaceSettingsUseSharedStore(t *testing.T) {
	w := newSettingsWorld(t)
	ctx := context.Background()
	other := NewService(Deps{System: w.system, Store: w.tx})
	if err := w.svc.UpdateInterfaceConfig(ctx, &dto.InterfaceConfig{HiddenMenuIds: []string{"/dashboard/nodes"}}); err != nil {
		t.Fatal(err)
	}
	got, err := other.GetInterfaceConfig(ctx)
	if err != nil || len(got.HiddenMenuIds) != 1 || got.HiddenMenuIds[0] != "/dashboard/nodes" {
		t.Fatalf("got %+v %v", got, err)
	}
	if err := other.UpdateInterfaceConfig(ctx, &dto.InterfaceConfig{HiddenMenuIds: []string{"/dashboard/nodes", "/dashboard/nodes"}}); xerr.CodeOf(err) != xerr.InvalidParams {
		t.Fatalf("duplicate accepted: %v", err)
	}
	got, err = w.svc.GetInterfaceConfig(ctx)
	if err != nil || len(got.HiddenMenuIds) != 1 {
		t.Fatalf("got %+v %v", got, err)
	}
}
