package routingstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const deliveryFilterTTL = 24 * time.Hour

func filterKey(serverID int64, snapshotID string) string {
	return fmt.Sprintf("server:routing-filter:v2:%d:%s", serverID, snapshotID)
}

// RememberFilter ties an authenticated delivery's opaque snapshot to its requested protocol filter.
func CanonicalFilter(protocols []string) ([]string, error) {
	if len(protocols) > 128 {
		return nil, fmt.Errorf("too many protocol filters")
	}
	seen := make(map[string]bool, len(protocols))
	values := make([]string, 0, len(protocols))
	size := 0
	for _, value := range protocols {
		if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
			return nil, fmt.Errorf("invalid protocol filter")
		}
		size += len(value)
		if size > 4096 {
			return nil, fmt.Errorf("protocol filters too large")
		}
		if !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values, nil
}

func RememberFilter(ctx context.Context, client *redis.Client, serverID int64, snapshotID string, protocols []string) error {
	if client == nil || serverID <= 0 || len(snapshotID) != 64 {
		return fmt.Errorf("delivery filter unavailable")
	}
	values, err := CanonicalFilter(protocols)
	if err != nil {
		return err
	}
	b, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return client.Set(ctx, filterKey(serverID, snapshotID), b, deliveryFilterTTL).Err()
}

func ReadFilter(ctx context.Context, client *redis.Client, serverID int64, snapshotID string) ([]string, error) {
	if client == nil || serverID <= 0 || len(snapshotID) != 64 {
		return nil, fmt.Errorf("delivery filter unavailable")
	}
	b, err := client.Get(ctx, filterKey(serverID, snapshotID)).Bytes()
	if err != nil {
		return nil, err
	}
	if len(b) > 8192 {
		return nil, fmt.Errorf("invalid delivery filter")
	}
	var values []string
	if err := json.Unmarshal(b, &values); err != nil || values == nil || len(values) > 128 {
		return nil, fmt.Errorf("invalid delivery filter")
	}
	return values, nil
}
