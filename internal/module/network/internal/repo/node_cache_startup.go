package repo

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/redis/go-redis/v9"
)

// ReconcileServerCaches clears every indexed or legacy server response cache
// before listeners and queue workers start. After a crash this reconstructs
// invalidations from surviving Redis keys, including deleted server IDs; it
// does not rely on the former process's in-memory retry queue.
func ReconcileServerCaches(ctx context.Context, client *redis.Client) error {
	if client == nil {
		return fmt.Errorf("server cache reconciliation unavailable")
	}
	prefixes := []string{strings.TrimSuffix(node.ServerCacheIndexKey, "%d"), node.ServerUserListCacheKey, node.ServerConfigCacheKey}
	ids := map[int64]struct{}{}
	responseKeys := map[int64][]string{}
	responseKeyCount := 0
	for _, prefix := range prefixes {
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, prefix+"*", 512).Result()
			if err != nil {
				return err
			}
			for _, key := range keys {
				suffix := strings.TrimPrefix(key, prefix)
				idRaw, _, _ := strings.Cut(suffix, ":")
				id, err := strconv.ParseInt(idRaw, 10, 64)
				if err != nil || id <= 0 {
					continue
				}
				ids[id] = struct{}{}
				if prefix == node.ServerUserListCacheKey || prefix == node.ServerConfigCacheKey {
					responseKeys[id] = append(responseKeys[id], key)
					responseKeyCount++
					if responseKeyCount > 500000 {
						return fmt.Errorf("server response cache reconciliation exceeds safety limit")
					}
				}
				if len(ids) > 100000 {
					return fmt.Errorf("server cache reconciliation exceeds safety limit")
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	for id := range ids {
		if err := clearServerCache(ctx, client, id); err != nil {
			return fmt.Errorf("server cache reconciliation: %w", err)
		}
		// Legacy response keys may coexist with a newer exact-key index.
		if keys := responseKeys[id]; len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("server cache reconciliation: %w", err)
			}
		}
	}
	return nil
}
