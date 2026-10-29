package serverapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
)

type mutableProtocolRepo struct {
	repository.NodeRepo
	protocols string
}

func (r *mutableProtocolRepo) FindOneServer(context.Context, int64) (*node.Server, error) {
	return &node.Server{Id: 1, Protocols: r.protocols}, nil
}
func (*mutableProtocolRepo) FindServerRoutingOverride(context.Context, int64) (*node.ServerConfigOverride, error) {
	return nil, nil
}

func TestQueryFailsClosedForEnabledInvalidProtocols(t *testing.T) {
	for _, raw := range []string{
		`[{"type":"vless","port":0,"enable":true,"transport":"tcp"}]`,
		`[{"type":"naive","port":443,"enable":true}]`,
		`[{"type":"shadowsocks","port":443,"enable":true,"cipher":"aes-128-gcm","plugin":"obfs"}]`,
		`[{"type":"vless","port":443,"enable":true,"transport":"xhttp","xhttp_mode":"bogus"}]`,
	} {
		repo := &mutableProtocolRepo{protocols: raw}
		deps := Deps{Store: snapshotStore{node: repo}, ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) {
			return config.NodeConfig{RoutingRevision: 1, Routing: &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}}, nil
		}}
		if got, err := newQueryServerProtocolConfigLogic(context.Background(), deps).QueryServerProtocolConfig(&dto.QueryServerConfigRequest{ServerID: 1}); err == nil {
			t.Fatalf("enabled invalid sanitized: %#v", got)
		}
	}
}

func TestQueryOneListenerToZeroEmitsArray(t *testing.T) {
	repo := &mutableProtocolRepo{protocols: `[{"type":"vless","port":443,"enable":true,"transport":"tcp"}]`}
	deps := Deps{Store: snapshotStore{node: repo}, ReadGlobalNodeConfig: func(context.Context) (config.NodeConfig, error) {
		return config.NodeConfig{RoutingRevision: 1, Routing: &config.NodeRouting{Version: 1, Outbounds: []config.NodeOutbound{}, Rules: []config.NodeRoute{}, DefaultOutboundID: "direct"}}, nil
	}}
	query := func() map[string]json.RawMessage {
		t.Helper()
		got, err := newQueryServerProtocolConfigLogic(context.Background(), deps).QueryServerProtocolConfig(&dto.QueryServerConfigRequest{ServerID: 1})
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(body, &data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	if got := string(query()["protocols"]); got == "[]" || got == "null" {
		t.Fatalf("one listener protocols=%s", got)
	}
	repo.protocols = `[{"type":"vless","port":443,"enable":false}]`
	if got := string(query()["protocols"]); got != "[]" {
		t.Fatalf("zero listeners protocols=%s, want []", got)
	}
}
