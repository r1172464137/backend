package identity

import (
	"context"
	"errors"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/internal/repository"
)

// TxStore runs a function over all repositories inside one transaction.
type TxStore interface {
	InTx(ctx context.Context, fn func(store repository.Store) error) error
}

// GlobalReader reads the platform-owned global node settings and routing.
type GlobalReader func(context.Context) (config.NodeConfig, error)

// Candidate is one complete, consistent node configuration.
type Candidate struct {
	Response *dto.QueryServerConfigResponse
	Result   *Result
}

// ErrGlobalChanged is returned when the global routing kept changing while
// the candidate was read.
var ErrGlobalChanged = errors.New("global routing changed while reading the candidate")

// BuildCandidate reads the server, its delivered protocols, its routing
// override and the authorised identities in one database transaction, with
// the platform-owned global routing read before and after; a revision change
// in between retries. activate=true is used only by real node pulls.
func BuildCandidate(ctx context.Context, store TxStore, readGlobal GlobalReader, serverID int64, now time.Time, activate bool) (*Candidate, error) {
	if store == nil || readGlobal == nil {
		return nil, errors.New("candidate readers unavailable")
	}
	for attempt := 0; attempt < 4; attempt++ {
		before, err := readGlobal(ctx)
		if err != nil {
			return nil, err
		}
		var out *Candidate
		err = store.InTx(ctx, func(tx repository.Store) error {
			server, err := tx.Node().FindOneServer(ctx, serverID)
			if err != nil {
				return err
			}
			protocols, err := DeliveredProtocols(server)
			if err != nil {
				return err
			}
			override, err := tx.Node().FindServerRoutingOverride(ctx, serverID)
			if err != nil {
				return err
			}
			routing, err := nodeconfig.DeliveryPolicy(before, override, serverID)
			if err != nil {
				return err
			}
			res, err := Build(ctx, tx, serverID, protocols, now, activate)
			if err != nil {
				return err
			}
			id, err := SnapshotID(routing, protocols, res)
			if err != nil {
				return err
			}
			res.Snapshot.SnapshotID = id
			out = &Candidate{Result: res, Response: &dto.QueryServerConfigResponse{
				TrafficReportThreshold: before.TrafficReportThreshold,
				PushInterval:           before.NodePushInterval,
				PullInterval:           before.NodePullInterval,
				Protocols:              protocols,
				Total:                  int64(len(protocols)),
				Routing:                routing,
				IdentitySnapshot:       res.Snapshot,
			}}
			return nil
		})
		if err != nil {
			return nil, err
		}
		after, err := readGlobal(ctx)
		if err != nil {
			return nil, err
		}
		if after.RoutingRevision == before.RoutingRevision {
			return out, nil
		}
	}
	return nil, ErrGlobalChanged
}

// DeliveredProtocols returns the enabled protocols of a server in the form
// sent to nodes. An enabled but invalid listener is an error; only explicitly
// disabled entries are dropped.
func DeliveredProtocols(server *node.Server) ([]dto.Protocol, error) {
	if err := node.ValidateNodeProtocols(server); err != nil {
		return nil, err
	}
	stored, err := server.UnmarshalProtocols()
	if err != nil {
		return nil, err
	}
	stored = node.SanitizeProtocolsForNodeDistribution(stored)
	var all []dto.Protocol
	mapping.DeepCopy(&all, stored)
	enabled := make([]dto.Protocol, 0, len(all))
	for _, p := range all {
		if p.Enable {
			enabled = append(enabled, p)
		}
	}
	return enabled, nil
}
