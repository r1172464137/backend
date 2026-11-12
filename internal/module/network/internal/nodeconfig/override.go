package nodeconfig

import (
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

// Routing modes shown to administrators; they are not format versions.
const (
	ModeGlobal    = "global"
	ModeInherited = "inherited"
	ModeLocal     = "local"
)

// OverrideResponse reports whether a server inherits the global routing
// package or holds a complete local one. The legacy DNS/IP columns of the
// row are rollback material and are not shown or read.
func OverrideResponse(override *node.ServerConfigOverride) (dto.ServerNodeConfigOverride, error) {
	resp := dto.ServerNodeConfigOverride{InheritRouting: true}
	if override == nil {
		return resp, nil
	}
	resp.RoutingRevision = override.RoutingRevision
	resp.InheritRouting = override.Routing == nil
	if override.Routing != nil {
		policy, err := config.ParseNodeRouting([]byte(*override.Routing))
		if err != nil {
			return resp, err
		}
		resp.Routing = policy
	}
	return resp, nil
}
