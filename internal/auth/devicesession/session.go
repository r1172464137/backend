// Package devicesession supplies per-device session generations. Rotating a
// generation revokes all sessions for that device, including offline clients,
// without scanning Redis or affecting the user's other sessions.
package devicesession

import (
	"context"
	"errors"
	"strconv"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
)

const (
	IDClaim    = "DeviceId"
	EpochClaim = "DeviceEpoch"
)

// Store is the Redis surface the generations use; *redis.Client satisfies it.
type Store interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
}

func key(id int64) string { return "auth:device_epoch:" + strconv.FormatInt(id, 10) }

// missing reports a store that is absent, including a nil *redis.Client
// passed as a Store.
func missing(client Store) bool {
	if client == nil {
		return true
	}
	rdb, ok := client.(*redis.Client)
	return ok && rdb == nil
}

func Epoch(ctx context.Context, client Store, id int64) (string, error) {
	if missing(client) || id <= 0 {
		return "", errors.New("device session store unavailable")
	}
	value, err := client.Get(ctx, key(id)).Result()
	if err == nil && value == "" {
		return "", errors.New("invalid device session generation")
	}
	return value, err
}

// AcquireEpoch is used only when issuing a new, authenticated session. A
// missing generation during token validation must never default to an old
// value: eviction of a key must not resurrect a previously revoked token.
func AcquireEpoch(ctx context.Context, client Store, id int64) (string, error) {
	if missing(client) || id <= 0 {
		return "", errors.New("device session store unavailable")
	}
	if err := client.SetNX(ctx, key(id), uuid.NewV7().String(), 0).Err(); err != nil {
		return "", err
	}
	return Epoch(ctx, client, id)
}

func Revoke(ctx context.Context, client Store, id int64) error {
	if missing(client) || id <= 0 {
		return errors.New("device session store unavailable")
	}
	// Do not expire the generation: an old token must not become valid again
	// after re-enabling the device or after a generation key's TTL elapses.
	return client.Set(ctx, key(id), uuid.NewV7().String(), 0).Err()
}

// Binding rejects malformed and legacy unbound device sessions. IDs are
// strings so JSON's float64 conversion cannot round a database ID.
func Binding(claims map[string]any) (id int64, epoch string, err error) {
	idValue, hasID := claims[IDClaim]
	epochValue, hasEpoch := claims[EpochClaim]
	if !hasID && !hasEpoch && claims["LoginType"] != "device" {
		return 0, "", nil
	}
	idText, ok := idValue.(string)
	if !ok {
		return 0, "", errors.New("device session must be renewed")
	}
	id, err = strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != idText {
		return 0, "", errors.New("invalid device session")
	}
	epoch, ok = epochValue.(string)
	if !ok || epoch == "" {
		return 0, "", errors.New("invalid device session generation")
	}
	return id, epoch, nil
}
