package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
)

// CutoverStore supplies the server inventory and the two transactions used by the offline 02180 conversion.
type CutoverStore interface {
	Node() repository.NodeRepo
	System() repository.SystemRepo
	repository.NetworkTransactor
	repository.PlatformTransactor
}

type OrderedCutoverPlan struct {
	Global         string
	Local          map[int64]string
	Servers        []int64
	ProtocolHashes map[int64]string
}

// PreflightOrderedCutover proves that the existing routes can be converted before any schema change.
func PreflightOrderedCutover(ctx context.Context, store CutoverStore, fresh bool) (*OrderedCutoverPlan, error) {
	global := config.LegacyRoutingInput{}
	var blockers []string
	globalReadable := true
	if !fresh {
		entries, err := store.System().GetLegacyNodeConfigUncached(ctx)
		if err != nil {
			return nil, err
		}
		if err := validateLegacyIntervals(entries); err != nil {
			blockers = append(blockers, fmt.Sprintf("global interval preflight blocked: %v", err))
		}
		for _, entry := range entries {
			if entry.Key == "DNS" && entry.Value != "" {
				if err := validateStoredDNS(entry.Value); err != nil {
					blockers = append(blockers, fmt.Sprintf("global DNS preflight blocked: %v", err))
				}
			}
		}
		global, err = decodeLegacyRouting(entries)
		if err != nil {
			blockers = append(blockers, fmt.Sprintf("global legacy routing preflight blocked: %v", err))
			globalReadable = false
		}
	}
	plan := &OrderedCutoverPlan{Local: map[int64]string{}, ProtocolHashes: map[int64]string{}}
	if globalReadable {
		globalPreview, err := config.ConvertLegacyRouting(global, "global")
		if err != nil {
			return nil, err
		}
		if globalPreview.Draft == nil {
			blockers = append(blockers, fmt.Sprintf("global legacy routing preflight blocked: %v", globalPreview.Blockers))
		} else {
			body, err := json.Marshal(globalPreview.Draft)
			if err != nil {
				return nil, err
			}
			plan.Global = string(body)
		}
	}
	if fresh {
		if len(blockers) > 0 {
			return nil, fmt.Errorf("%s", strings.Join(blockers, "; "))
		}
		return plan, nil
	}
	return plan.preflightServers(ctx, store.Node(), global, globalReadable, blockers)
}

func (plan *OrderedCutoverPlan) preflightServers(ctx context.Context, nodes repository.NodeRepo, global config.LegacyRoutingInput, globalReadable bool, blockers []string) (*OrderedCutoverPlan, error) {
	servers, err := nodes.QueryServerSorts(ctx)
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		plan.Servers = append(plan.Servers, server.Id)
		inventory, err := nodes.QueryServerList(ctx, []int64{server.Id})
		if err != nil {
			return nil, err
		}
		if len(inventory) != 1 || inventory[0].Id != server.Id {
			return nil, fmt.Errorf("server %d inventory changed", server.Id)
		}
		if err := node.ValidateNodeProtocols(inventory[0]); err != nil {
			blockers = append(blockers, fmt.Sprintf("server %d protocol preflight blocked: %v", server.Id, err))
		}
		sum := sha256.Sum256([]byte(inventory[0].Protocols))
		plan.ProtocolHashes[server.Id] = hex.EncodeToString(sum[:])
		override, err := nodes.FindServerConfigOverride(ctx, server.Id)
		if err != nil {
			return nil, err
		}
		if override != nil && override.DNS != nil {
			if err := validateStoredDNS(*override.DNS); err != nil {
				blockers = append(blockers, fmt.Sprintf("server %d DNS preflight blocked: %v", server.Id, err))
			}
		}
		if !globalReadable {
			continue
		}
		effective, hasOverride, err := effectiveLegacyRouting(global, override)
		if err != nil {
			blockers = append(blockers, fmt.Sprintf("server %d legacy routing preflight blocked: %v", server.Id, err))
			continue
		}
		preview, err := config.ConvertLegacyRouting(effective, "server:"+strconv.FormatInt(server.Id, 10))
		if err != nil {
			return nil, err
		}
		if preview.Draft == nil {
			blockers = append(blockers, fmt.Sprintf("server %d legacy routing preflight blocked: %v", server.Id, preview.Blockers))
			continue
		}
		if hasOverride {
			body, err := json.Marshal(preview.Draft)
			if err != nil {
				return nil, err
			}
			plan.Local[server.Id] = string(body)
		}
	}
	sort.Slice(plan.Servers, func(i, j int) bool { return plan.Servers[i] < plan.Servers[j] })
	if len(blockers) > 0 {
		sort.Strings(blockers)
		return nil, fmt.Errorf("%s", strings.Join(blockers, "; "))
	}
	return plan, nil
}

// ValidateServerDNS checks each stored override before publishing new node settings.
func ValidateServerDNS(ctx context.Context, store CutoverStore) error {
	servers, err := store.Node().QueryServerSorts(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		override, err := store.Node().FindServerRoutingOverride(ctx, server.Id)
		if err != nil {
			return err
		}
		if override != nil && override.DNS != nil {
			if err := validateStoredDNS(*override.DNS); err != nil {
				return fmt.Errorf("invalid server %d DNS: %w", server.Id, err)
			}
		}
	}
	return nil
}

func validateStoredDNS(raw string) error {
	_, err := config.ParseNodeDNSJSON([]byte(raw))
	return err
}

func validateLegacyIntervals(entries []*system.System) error {
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Key != "NodePullInterval" && entry.Key != "NodePushInterval" {
			continue
		}
		if seen[entry.Key] {
			return fmt.Errorf("duplicate %s", entry.Key)
		}
		seen[entry.Key] = true
		seconds, err := strconv.ParseInt(entry.Value, 10, 64)
		if err != nil || (entry.Type != "int" && entry.Type != "int64") {
			return fmt.Errorf("invalid %s", entry.Key)
		}
		if err := config.ValidateNodeInterval(entry.Key, seconds); err != nil {
			return err
		}
	}
	return nil
}

func decodeLegacyRouting(entries []*system.System) (config.LegacyRoutingInput, error) {
	var out config.LegacyRoutingInput
	seen := map[string]bool{}
	for _, entry := range entries {
		var dst any
		switch entry.Key {
		case "Block":
			dst = &out.Block
		case "BlockIPs":
			dst = &out.BlockIPs
		case "BlockProtocols":
			dst = &out.BlockProtocols
		case "Outbound":
			dst = &out.Outbound
		default:
			continue
		}
		if seen[entry.Key] {
			return out, fmt.Errorf("duplicate legacy %s", entry.Key)
		}
		seen[entry.Key] = true
		if entry.Type != "string" {
			return out, fmt.Errorf("invalid legacy %s storage type", entry.Key)
		}
		if entry.Value == "" {
			continue
		}
		if err := decodeLegacyJSON(entry.Value, dst); err != nil {
			return out, fmt.Errorf("invalid legacy %s: %w", entry.Key, err)
		}
	}
	return out, nil
}
func decodeLegacyJSON(raw string, dst any) error {
	if !utf8.ValidString(raw) {
		return fmt.Errorf("legacy JSON is not valid UTF-8")
	}
	if err := config.ValidateUniqueJSON([]byte(raw)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing legacy JSON")
	}
	return nil
}
func effectiveLegacyRouting(global config.LegacyRoutingInput, o *node.ServerConfigOverride) (config.LegacyRoutingInput, bool, error) {
	out := global
	if o == nil {
		return out, false, nil
	}
	blockOverride := o.Block != nil || o.BlockIPs != nil || o.BlockProtocols != nil
	if blockOverride {
		out.Block, out.BlockIPs, out.BlockProtocols = []string{}, []string{}, []string{}
		for _, item := range []struct {
			field string
			raw   *string
			dst   any
		}{{"block", o.Block, &out.Block}, {"block_ips", o.BlockIPs, &out.BlockIPs}, {"block_protocols", o.BlockProtocols, &out.BlockProtocols}} {
			if item.raw == nil {
				continue
			}
			if err := decodeLegacyJSON(*item.raw, item.dst); err != nil {
				return out, false, fmt.Errorf("invalid %s: %w", item.field, err)
			}
		}
	}
	if o.Outbound != nil {
		if err := decodeLegacyJSON(*o.Outbound, &out.Outbound); err != nil {
			return out, false, fmt.Errorf("invalid outbound: %w", err)
		}
	}
	return out, blockOverride || o.Outbound != nil, nil
}

func checkCutoverStart(ctx context.Context, store CutoverStore, plan *OrderedCutoverPlan) error {
	if plan == nil {
		return fmt.Errorf("missing cutover plan")
	}
	servers, err := store.Node().QueryServerSorts(ctx)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(servers))
	for _, server := range servers {
		ids = append(ids, server.Id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) != len(plan.Servers) {
		return fmt.Errorf("server inventory changed after preflight; restore backup")
	}
	for i := range ids {
		if ids[i] != plan.Servers[i] {
			return fmt.Errorf("server inventory changed after preflight; restore backup")
		}
	}
	for _, server := range servers {
		active, err := store.Node().FindServerRoutingOverride(ctx, server.Id)
		if err != nil {
			return err
		}
		if active == nil {
			continue
		}
		expected, planned := plan.Local[server.Id]
		if active.Routing != nil && (!planned || *active.Routing != expected || active.RoutingRevision != 1) {
			return fmt.Errorf("server %d has unexpected cutover routing; restore backup", server.Id)
		}
		if active.Routing == nil && active.RoutingRevision != 0 {
			return fmt.Errorf("server %d has unexpected cutover revision", server.Id)
		}
	}
	return nil
}

func writeCutoverGlobal(ctx context.Context, store CutoverStore, plan *OrderedCutoverPlan) error {
	return store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		entries, err := tx.System().GetNodeConfigForUpdate(ctx)
		if err != nil {
			return err
		}
		marker, current, revision := "", "", ""
		for _, e := range entries {
			if e.Key == "RoutingCutover" {
				marker = e.Value
			}
			if e.Key == "Routing" {
				current = e.Value
			}
			if e.Key == "RoutingRevision" {
				revision = e.Value
			}
		}
		if marker != "pending" {
			return fmt.Errorf("unexpected cutover marker %q", marker)
		}
		if (current == "" && revision != "0") || (current != "" && (current != plan.Global || revision != "1")) {
			return fmt.Errorf("global cutover content or revision changed; restore backup")
		}
		if err := tx.System().UpdateValueByCategoryKey(ctx, "server", "Routing", plan.Global, "string"); err != nil {
			return err
		}
		return tx.System().UpdateValueByCategoryKey(ctx, "server", "RoutingRevision", "1", "int64")
	})
}

func writeCutoverOverrides(ctx context.Context, store CutoverStore, plan *OrderedCutoverPlan) error {
	for _, id := range plan.Servers {
		content, ok := plan.Local[id]
		if !ok {
			continue
		}
		if err := store.InNetworkTx(ctx, func(tx repository.NetworkStore) error {
			if _, err := tx.Node().FindOneServerForUpdate(ctx, id); err != nil {
				return err
			}
			old, err := tx.Node().FindServerConfigOverride(ctx, id)
			if err != nil {
				return err
			}
			if old == nil {
				return fmt.Errorf("server %d override disappeared", id)
			}
			active, err := tx.Node().FindServerRoutingOverride(ctx, id)
			if err != nil {
				return err
			}
			if active != nil && active.Routing != nil && (*active.Routing != content || active.RoutingRevision != 1) {
				return fmt.Errorf("server %d cutover content changed; restore backup", id)
			}
			data := &node.ServerConfigOverride{ServerId: id, IPStrategy: old.IPStrategy, DNS: old.DNS, Routing: &content, RoutingRevision: 1}
			return tx.Node().SaveServerRoutingOverride(ctx, data)
		}); err != nil {
			return err
		}
	}
	return nil
}

// ApplyOrderedCutover writes each owner's settings in its transaction, then
// rechecks the original inputs before marking the conversion complete.
func ApplyOrderedCutover(ctx context.Context, store CutoverStore, plan *OrderedCutoverPlan) error {
	if err := checkCutoverStart(ctx, store, plan); err != nil {
		return err
	}
	if err := writeCutoverGlobal(ctx, store, plan); err != nil {
		return err
	}
	if err := writeCutoverOverrides(ctx, store, plan); err != nil {
		return err
	}
	// Recheck the independently-owned source inventory and effective legacy
	// inputs before declaring cutover complete. Offline operation remains an
	// operator prerequisite; this detects unexpected changes rather than
	// pretending to hold a cross-module transaction.
	rechecked, err := PreflightOrderedCutover(ctx, store, false)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, rechecked) {
		return fmt.Errorf("legacy inputs or server inventory changed during cutover; restore backup")
	}
	for _, id := range plan.Servers {
		current, err := store.Node().FindServerRoutingOverride(ctx, id)
		if err != nil {
			return err
		}
		content, planned := plan.Local[id]
		if planned {
			if current == nil || current.Routing == nil || *current.Routing != content || current.RoutingRevision != 1 {
				return fmt.Errorf("server %d cutover verification failed", id)
			}
		} else if current != nil && (current.Routing != nil || current.RoutingRevision != 0) {
			return fmt.Errorf("server %d acquired an unexpected routing override during cutover", id)
		}
	}
	return store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		entries, err := tx.System().GetNodeConfigForUpdate(ctx)
		if err != nil {
			return err
		}
		marker, current := "", ""
		for _, e := range entries {
			if e.Key == "RoutingCutover" {
				marker = e.Value
			}
			if e.Key == "Routing" {
				current = e.Value
			}
		}
		if marker != "pending" || current != plan.Global {
			return fmt.Errorf("global cutover verification failed")
		}
		return tx.System().UpdateValueByCategoryKey(ctx, "server", "RoutingCutover", "complete", "string")
	})
}
