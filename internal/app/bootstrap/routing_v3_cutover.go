package bootstrap

// One-time routing format 3 migration (see .local/rebuild/MIGRATION.md).
//
// Input: schema 2180 with RoutingCutover=complete and routing format 1.
// Output: the same rows in routing format 3, RoutingCutover=complete-v3.
// Only data changes; no DDL. Global and per-server rows belong to different
// modules, so the migration uses a marker (migrating-v3), row locks, content
// checks against a read-only plan, and a final marker - never a pretended
// cross-module transaction. Anything unexpected stops and asks for a restore.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
)

const (
	cutoverLegacyPending = "pending"  // 02180 rows created, legacy cutover never completed
	cutoverV1Complete    = "complete" // routing format 1 runtime (migration input)
	cutoverV3Migrating   = "migrating-v3"
	cutoverV3Complete    = "complete-v3" // the only state this build serves
)

type routingV3Plan struct {
	GlobalV1       string
	GlobalV3       string
	GlobalRevision int64
	Servers        []int64
	Local          map[int64]localRoutingV3Plan
}

type localRoutingV3Plan struct {
	V1       string
	V3       string
	Revision int64
}

// routingV1 is the format 1 content, read only as migration input.
type routingV1 struct {
	Version           int                   `json:"version"`
	Outbounds         []config.NodeOutbound `json:"outbounds"`
	Rules             []routingV1Rule       `json:"rules"`
	DefaultOutboundID string                `json:"default_outbound_id"`
}

type routingV1Rule struct {
	ID         string   `json:"id"`
	Enabled    *bool    `json:"enabled"`
	Remark     string   `json:"remark"`
	Kind       string   `json:"kind"`
	Values     []string `json:"values"`
	OutboundID string   `json:"outbound_id"`
}

func systemValues(entries []*system.System) map[string]string {
	values := map[string]string{}
	for _, e := range entries {
		values[e.Key] = e.Value
	}
	return values
}

// DefaultRoutingV3 is the content of a fresh installation.
func DefaultRoutingV3() *config.NodeRouting { return config.DefaultNodeRouting("global") }

func targetPreferenceFromLegacy(ipStrategy string) (string, error) {
	switch strings.TrimSpace(ipStrategy) {
	case "":
		return "none", nil
	case "prefer_ipv4":
		return "prefer_ipv4", nil
	case "prefer_ipv6":
		return "prefer_ipv6", nil
	default:
		return "", fmt.Errorf("unsupported legacy ip_strategy %q", ipStrategy)
	}
}

func legacyDNSEmpty(raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return true, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return false, fmt.Errorf("unreadable legacy DNS: %w", err)
	}
	return len(items) == 0, nil
}

// convertRoutingV1 maps a format 1 policy to format 3: rule IDs, order,
// enabled flags, remarks, kinds, values and outbound references are kept;
// every rule gets an empty account scope (all accounts); no account
// defaults; the explicit system DNS default; the given IP preference.
func convertRoutingV1(raw, scope, preference string) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var v1 *routingV1
	if err := config.ValidateUniqueJSON([]byte(raw)); err != nil {
		return "", fmt.Errorf("%s routing: %w", scope, err)
	}
	if err := decoder.Decode(&v1); err != nil || v1 == nil {
		return "", fmt.Errorf("%s routing is not readable format 1: %v", scope, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", fmt.Errorf("%s routing has trailing data", scope)
	}
	if v1.Version != 1 {
		return "", fmt.Errorf("%s routing has format %d; only format 1 can be migrated (restore the matching backup)", scope, v1.Version)
	}
	if v1.Outbounds == nil || v1.Rules == nil {
		return "", fmt.Errorf("%s routing lacks outbounds or rules", scope)
	}
	v3 := &config.NodeRouting{Version: config.RoutingVersion, Outbounds: v1.Outbounds, DefaultOutboundID: v1.DefaultOutboundID,
		Rules: make([]config.NodeRoute, len(v1.Rules)), UserDefaults: []config.NodeUserDefault{},
		DNS: config.DefaultDNSPolicy(scope), TargetIPPreference: preference}
	for i, r := range v1.Rules {
		v3.Rules[i] = config.NodeRoute{ID: r.ID, Enabled: r.Enabled, Remark: r.Remark, Kind: r.Kind, Values: r.Values,
			OutboundID: r.OutboundID, UserIDs: []int64{}}
	}
	normalized, err := config.NormalizeNodeRouting(v3)
	if err != nil {
		return "", fmt.Errorf("%s routing does not validate as format 3: %w", scope, err)
	}
	// Rule order, IDs and references must survive normalisation unchanged.
	for i, r := range normalized.Rules {
		if r.ID != v1.Rules[i].ID || r.OutboundID != v1.Rules[i].OutboundID || r.Kind != v1.Rules[i].Kind || *r.Enabled != *v1.Rules[i].Enabled {
			return "", fmt.Errorf("%s rule %d changed during conversion", scope, i+1)
		}
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// planRoutingV3 is read-only. It reads the format 1 input and computes the
// format 3 content of every row it will write.
func planRoutingV3(ctx context.Context, store repository.Store) (*routingV3Plan, error) {
	entries, err := store.System().GetNodeConfigUncached(ctx)
	if err != nil {
		return nil, err
	}
	v := systemValues(entries)
	revision, err := strconv.ParseInt(v["RoutingRevision"], 10, 64)
	if err != nil || revision <= 0 {
		return nil, fmt.Errorf("invalid global routing revision %q", v["RoutingRevision"])
	}
	if empty, err := legacyDNSEmpty(v["DNS"]); err != nil || !empty {
		return nil, fmt.Errorf("global DNS is set; only an empty legacy DNS can be migrated automatically (%v)", err)
	}
	globalPreference, err := targetPreferenceFromLegacy(v["IPStrategy"])
	if err != nil {
		return nil, fmt.Errorf("global %w", err)
	}
	plan := &routingV3Plan{GlobalV1: v["Routing"], GlobalRevision: revision, Local: map[int64]localRoutingV3Plan{}}
	if plan.GlobalV3, err = convertRoutingV1(plan.GlobalV1, "global", globalPreference); err != nil {
		return nil, err
	}
	servers, err := store.Node().QueryServerSorts(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range servers {
		plan.Servers = append(plan.Servers, s.Id)
	}
	sort.Slice(plan.Servers, func(i, j int) bool { return plan.Servers[i] < plan.Servers[j] })
	for _, id := range plan.Servers {
		override, err := store.Node().FindServerConfigOverride(ctx, id)
		if err != nil {
			return nil, err
		}
		routing, err := store.Node().FindServerRoutingOverride(ctx, id)
		if err != nil {
			return nil, err
		}
		local := routing != nil && routing.Routing != nil
		if override != nil && override.DNS != nil {
			if empty, err := legacyDNSEmpty(*override.DNS); err != nil || !empty {
				return nil, fmt.Errorf("server %d has a DNS override; only empty DNS can be migrated automatically (%v)", id, err)
			}
		}
		preference := globalPreference
		if override != nil && override.IPStrategy != nil {
			p, err := targetPreferenceFromLegacy(*override.IPStrategy)
			if err != nil {
				return nil, fmt.Errorf("server %d %w", id, err)
			}
			if !local && p != globalPreference {
				return nil, fmt.Errorf("server %d overrides only ip_strategy (%q); format 3 has no partial overrides", id, *override.IPStrategy)
			}
			preference = p
		}
		if !local {
			if routing != nil && routing.RoutingRevision < 0 {
				return nil, fmt.Errorf("server %d has an invalid routing revision", id)
			}
			continue
		}
		if routing.RoutingRevision <= 0 {
			return nil, fmt.Errorf("server %d local routing has revision %d", id, routing.RoutingRevision)
		}
		v3, err := convertRoutingV1(*routing.Routing, fmt.Sprintf("server-%d", id), preference)
		if err != nil {
			return nil, err
		}
		plan.Local[id] = localRoutingV3Plan{V1: *routing.Routing, V3: v3, Revision: routing.RoutingRevision}
	}
	return plan, nil
}

// migrateRoutingV3 runs or resumes the migration. marker is the value read at
// startup (complete or migrating-v3).
func migrateRoutingV3(ctx context.Context, store repository.Store, marker string) error {
	var plan *routingV3Plan
	var err error
	switch marker {
	case cutoverV1Complete:
		if plan, err = planRoutingV3(ctx, store); err != nil {
			return err
		}
		if err := setCutoverMarker(ctx, store, cutoverV1Complete, cutoverV3Migrating); err != nil {
			return err
		}
	case cutoverV3Migrating:
		if plan, err = resumePlan(ctx, store); err != nil {
			return err
		}
	default:
		return fmt.Errorf("routing migration cannot start from marker %q", marker)
	}
	if err := writeGlobalV3(ctx, store, plan); err != nil {
		return err
	}
	for _, id := range plan.Servers {
		local, ok := plan.Local[id]
		if !ok {
			continue
		}
		if err := writeServerV3(ctx, store, id, local); err != nil {
			return err
		}
	}
	if err := verifyRoutingV3(ctx, store, plan); err != nil {
		return err
	}
	return setCutoverMarker(ctx, store, cutoverV3Migrating, cutoverV3Complete)
}

// resumePlan rebuilds the plan after an interrupted run. Each row must be
// either still the format 1 input or already the planned format 3 content.
func resumePlan(ctx context.Context, store repository.Store) (*routingV3Plan, error) {
	entries, err := store.System().GetNodeConfigUncached(ctx)
	if err != nil {
		return nil, err
	}
	v := systemValues(entries)
	globalPreference, err := targetPreferenceFromLegacy(v["IPStrategy"])
	if err != nil {
		return nil, err
	}
	if _, parseErr := config.ParseNodeRouting([]byte(v["Routing"])); parseErr == nil {
		// Global already written (it parses as format 3): recover the input
		// revision from the written one and plan the remaining server rows
		// from their current content.
		revision, err := strconv.ParseInt(v["RoutingRevision"], 10, 64)
		if err != nil || revision <= 1 {
			return nil, fmt.Errorf("interrupted migration has an invalid global revision; restore backup")
		}
		plan := &routingV3Plan{GlobalV1: "", GlobalV3: v["Routing"], GlobalRevision: revision - 1, Local: map[int64]localRoutingV3Plan{}}
		servers, err := store.Node().QueryServerSorts(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range servers {
			plan.Servers = append(plan.Servers, s.Id)
		}
		sort.Slice(plan.Servers, func(i, j int) bool { return plan.Servers[i] < plan.Servers[j] })
		for _, id := range plan.Servers {
			routing, err := store.Node().FindServerRoutingOverride(ctx, id)
			if err != nil {
				return nil, err
			}
			if routing == nil || routing.Routing == nil {
				continue
			}
			if _, err := config.ParseNodeRouting([]byte(*routing.Routing)); err == nil {
				plan.Local[id] = localRoutingV3Plan{V3: *routing.Routing, Revision: routing.RoutingRevision - 1}
				continue
			}
			override, err := store.Node().FindServerConfigOverride(ctx, id)
			if err != nil {
				return nil, err
			}
			preference := globalPreference
			if override != nil && override.IPStrategy != nil {
				if preference, err = targetPreferenceFromLegacy(*override.IPStrategy); err != nil {
					return nil, err
				}
			}
			v3, err := convertRoutingV1(*routing.Routing, fmt.Sprintf("server-%d", id), preference)
			if err != nil {
				return nil, err
			}
			plan.Local[id] = localRoutingV3Plan{V1: *routing.Routing, V3: v3, Revision: routing.RoutingRevision}
		}
		return plan, nil
	}
	// Nothing written yet: plan from the format 1 input as at the start.
	return planRoutingV3(ctx, store)
}

func setCutoverMarker(ctx context.Context, store repository.Store, from, to string) error {
	return store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		entries, err := tx.System().GetNodeConfigForUpdate(ctx)
		if err != nil {
			return err
		}
		if got := systemValues(entries)["RoutingCutover"]; got != from {
			return fmt.Errorf("routing cutover marker is %q, expected %q; restore backup", got, from)
		}
		return tx.System().UpdateValueByCategoryKey(ctx, "server", "RoutingCutover", to, "string")
	})
}

func writeGlobalV3(ctx context.Context, store repository.Store, plan *routingV3Plan) error {
	return store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		entries, err := tx.System().GetNodeConfigForUpdate(ctx)
		if err != nil {
			return err
		}
		v := systemValues(entries)
		if v["RoutingCutover"] != cutoverV3Migrating {
			return fmt.Errorf("routing cutover marker changed during migration; restore backup")
		}
		target := strconv.FormatInt(plan.GlobalRevision+1, 10)
		if v["Routing"] == plan.GlobalV3 && v["RoutingRevision"] == target {
			return nil // already written by an interrupted run
		}
		if v["Routing"] != plan.GlobalV1 || v["RoutingRevision"] != strconv.FormatInt(plan.GlobalRevision, 10) {
			return fmt.Errorf("global routing changed after the migration plan; restore backup")
		}
		if err := tx.System().UpdateValueByCategoryKey(ctx, "server", "Routing", plan.GlobalV3, "string"); err != nil {
			return err
		}
		return tx.System().UpdateValueByCategoryKey(ctx, "server", "RoutingRevision", target, "int64")
	})
}

func writeServerV3(ctx context.Context, store repository.Store, id int64, local localRoutingV3Plan) error {
	return store.InNetworkTx(ctx, func(tx repository.NetworkStore) error {
		if _, err := tx.Node().FindOneServerForUpdate(ctx, id); err != nil {
			return err
		}
		old, err := tx.Node().FindServerConfigOverride(ctx, id)
		if err != nil {
			return err
		}
		routing, err := tx.Node().FindServerRoutingOverride(ctx, id)
		if err != nil {
			return err
		}
		if old == nil || routing == nil || routing.Routing == nil {
			return fmt.Errorf("server %d local routing disappeared during migration; restore backup", id)
		}
		if *routing.Routing == local.V3 && routing.RoutingRevision == local.Revision+1 {
			return nil
		}
		if *routing.Routing != local.V1 || routing.RoutingRevision != local.Revision {
			return fmt.Errorf("server %d routing changed after the migration plan; restore backup", id)
		}
		content := local.V3
		return tx.Node().SaveServerRoutingOverride(ctx, &node.ServerConfigOverride{ServerId: id, IPStrategy: old.IPStrategy,
			DNS: old.DNS, Routing: &content, RoutingRevision: local.Revision + 1})
	})
}

func verifyRoutingV3(ctx context.Context, store repository.Store, plan *routingV3Plan) error {
	entries, err := store.System().GetNodeConfigUncached(ctx)
	if err != nil {
		return err
	}
	v := systemValues(entries)
	if v["Routing"] != plan.GlobalV3 || v["RoutingRevision"] != strconv.FormatInt(plan.GlobalRevision+1, 10) {
		return fmt.Errorf("global routing verification failed")
	}
	servers, err := store.Node().QueryServerSorts(ctx)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(servers))
	for _, s := range servers {
		ids = append(ids, s.Id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if fmt.Sprint(ids) != fmt.Sprint(plan.Servers) {
		return fmt.Errorf("server inventory changed during migration; restore backup")
	}
	for _, id := range ids {
		routing, err := store.Node().FindServerRoutingOverride(ctx, id)
		if err != nil {
			return err
		}
		local, planned := plan.Local[id]
		if planned {
			if routing == nil || routing.Routing == nil || *routing.Routing != local.V3 || routing.RoutingRevision != local.Revision+1 {
				return fmt.Errorf("server %d routing verification failed", id)
			}
			continue
		}
		if routing != nil && routing.Routing != nil {
			return fmt.Errorf("server %d acquired local routing during migration; restore backup", id)
		}
	}
	return nil
}

// bootstrapFreshRoutingV3 initialises an empty installation directly in
// format 3. It runs only when no server exists and the 02180 rows still hold
// their initial values.
func bootstrapFreshRoutingV3(ctx context.Context, store repository.Store) error {
	servers, err := store.Node().QueryServerSorts(ctx)
	if err != nil {
		return err
	}
	if len(servers) != 0 {
		return fmt.Errorf("database has servers but no completed routing cutover; restore the matching backup")
	}
	body, err := json.Marshal(DefaultRoutingV3())
	if err != nil {
		return err
	}
	if _, err := config.ParseNodeRouting(body); err != nil {
		return err
	}
	return store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		entries, err := tx.System().GetNodeConfigForUpdate(ctx)
		if err != nil {
			return err
		}
		v := systemValues(entries)
		if v["RoutingCutover"] != cutoverLegacyPending || v["Routing"] != "" || v["RoutingRevision"] != "0" {
			return fmt.Errorf("unexpected routing state on a fresh installation; restore backup")
		}
		if err := tx.System().UpdateValueByCategoryKey(ctx, "server", "Routing", string(body), "string"); err != nil {
			return err
		}
		if err := tx.System().UpdateValueByCategoryKey(ctx, "server", "RoutingRevision", "1", "int64"); err != nil {
			return err
		}
		return tx.System().UpdateValueByCategoryKey(ctx, "server", "RoutingCutover", cutoverV3Complete, "string")
	})
}
