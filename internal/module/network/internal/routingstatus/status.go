// Package routingstatus stores server-level routing reports. No protocol
// controller owns this state, and reports from multiple fresh processes are
// deliberately projected as unknown.
package routingstatus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/redis/go-redis/v9"
)

const TTL = 3 * time.Minute

// Preserve sequence watermarks beyond liveness expiry so delayed packets do
// not revive applied state after a normal TTL gap. Redis loss or expiry after
// this bounded retention remains an explicit loss of evidence.
const watermarkRetention = 24 * time.Hour
const maxInstances = 128
const maxRetainedInstances = 4096

var instancePattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Keep in sync with the same-batch node's candidate failure stages.
var knownStages = map[string]bool{"config": true, "fetch": true, "validate": true, "build": true, "core_start": true, "prepare": true, "stop": true, "bind": true, "monitor": true, "restore": true}

// Redis TIME is the common authority even when two backends have skewed
// clocks. Per-instance records are pruned independently, so one process
// cannot extend another process's freshness.
var storeScript = redis.NewScript(`
local now = tonumber(redis.call('TIME')[1])
local ttl = tonumber(ARGV[3])
local retention = tonumber(ARGV[8])
local entries = redis.call('HGETALL', KEYS[1])
local count = 0
local retained = 0
for i=1,#entries,2 do
  local value = cjson.decode(entries[i+1])
  local received = tonumber(value.received_at)
  if not received or now - received >= retention then
    redis.call('HDEL', KEYS[1], entries[i])
  else
    retained = retained + 1
    if now - received < ttl then count = count + 1 end
  end
end
local field = ARGV[2]
local old = redis.call('HGET', KEYS[1], field)
if not old and (count >= tonumber(ARGV[4]) or retained >= tonumber(ARGV[9])) then
  redis.call('HSET', KEYS[1], '__overflow', cjson.encode({received_at=now, conflict=true}))
  redis.call('EXPIRE', KEYS[1], retention)
  return 0
end
-- Keep sequence and payload as strings: Redis Lua numbers cannot represent
-- all uint64 sequences or int64 revisions without rounding.
local incoming = {received_at=now, content_received_at=now, sequence=ARGV[5], verified=ARGV[6] == '1', fingerprint=ARGV[7], payload=ARGV[1]}
if old then
  local previous = cjson.decode(old)
  if previous.sequence and previous.sequence ~= '' and incoming.sequence ~= '' then
    local a, b = incoming.sequence, previous.sequence
    local older = #a < #b or (#a == #b and a < b)
    if older or (previous.conflict and a == b) then
      previous.received_at = now
      redis.call('HSET', KEYS[1], field, cjson.encode(previous))
      redis.call('EXPIRE', KEYS[1], retention)
      return 0
    end
    if a == b then
      if previous.fingerprint ~= incoming.fingerprint then
        previous = {received_at=now, content_received_at=previous.content_received_at, conflict=true, sequence=b}
      else
        previous.received_at = now
      end
      redis.call('HSET', KEYS[1], field, cjson.encode(previous))
      redis.call('EXPIRE', KEYS[1], retention)
      return 0
    end
    -- A strictly higher sequence from this process resolves its conflict.
  end
end
redis.call('HSET', KEYS[1], field, cjson.encode(incoming))
redis.call('EXPIRE', KEYS[1], retention)
return 1
`)
var readScript = redis.NewScript(`
local now = tonumber(redis.call('TIME')[1])
local ttl = tonumber(ARGV[1])
local retention = tonumber(ARGV[2])
local entries = redis.call('HGETALL', KEYS[1])
local count = 0
local result = nil
for i=1,#entries,2 do
  local value = cjson.decode(entries[i+1])
  local received = tonumber(value.received_at)
  if not received or now - received >= retention then
    redis.call('HDEL', KEYS[1], entries[i])
  elseif now - received < ttl then
    count = count + 1
    local content_time = tonumber(value.content_received_at)
    if not value.conflict and value.verified and content_time and now >= content_time and now - content_time < ttl then
      result = entries[i+1]
    end
  end
end
if count ~= 1 then return nil end
return result
`)

func key(id int64) string { return fmt.Sprintf("server:routing-report:v1:%d", id) }

func Record(ctx context.Context, client *redis.Client, id int64, request *dto.ServerPushStatusRequest) error {
	if client == nil {
		return fmt.Errorf("routing status store unavailable")
	}
	if request == nil || id <= 0 {
		return fmt.Errorf("invalid server report")
	}
	field := request.InstanceID
	verified := request.RoutingVersion == 1 && instancePattern.MatchString(field)
	if !verified {
		return storeUnknown(ctx, client, id)
	}
	if request.RoutingSequence == 0 {
		_ = storeUnknown(ctx, client, id)
		return fmt.Errorf("routing report sequence is required")
	}
	for _, candidate := range []*dto.RoutingCandidateIdentity{request.RoutingApplied, request.RoutingPending} {
		if candidate != nil && !validCandidate(candidate, id) {
			_ = storeUnknown(ctx, client, id)
			return fmt.Errorf("invalid routing candidate identity")
		}
	}
	var reportError *dto.RoutingReportError
	if request.RoutingError != nil {
		e := *request.RoutingError
		if !knownStages[e.Stage] || (e.Identity != nil && !validCandidate(e.Identity, id)) {
			_ = storeUnknown(ctx, client, id)
			return fmt.Errorf("invalid routing failure")
		}
		// The node wire intentionally reports a fixed stage summary; never
		// persist arbitrary error text which might contain credentials.
		e.Message = "candidate failed during " + e.Stage
		reportError = &e
	}
	value := dto.RoutingStatusReport{Version: request.RoutingVersion, InstanceID: request.InstanceID,
		Applied: request.RoutingApplied, Pending: request.RoutingPending, Error: reportError, UpdatedAt: request.UpdatedAt}
	// updated_at is informational and does not participate in content order
	// or conflict detection. Nodes assign one monotonic sequence per report.
	content := struct {
		Version    int                           `json:"version"`
		InstanceID string                        `json:"instance_id"`
		Applied    *dto.RoutingCandidateIdentity `json:"applied"`
		Pending    *dto.RoutingCandidateIdentity `json:"pending"`
		Error      *dto.RoutingReportError       `json:"error"`
	}{value.Version, value.InstanceID, value.Applied, value.Pending, value.Error}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(contentBytes)
	return storeScript.Run(ctx, client, []string{key(id)}, body, field, int(TTL.Seconds()), maxInstances, strconv.FormatUint(request.RoutingSequence, 10), "1", hex.EncodeToString(digest[:]), int(watermarkRetention.Seconds()), maxRetainedInstances).Err()
}

// MarkUnknown invalidates capability evidence for an authenticated malformed
// server report. It deliberately records no payload or parse error text.
func MarkUnknown(ctx context.Context, client *redis.Client, id int64) error {
	if client == nil || id <= 0 {
		return fmt.Errorf("routing status store unavailable")
	}
	return storeScript.Run(ctx, client, []string{key(id)}, `{}`, "__unidentified", int(TTL.Seconds()), maxInstances, "", "0", "", int(watermarkRetention.Seconds()), maxRetainedInstances).Err()
}
func storeUnknown(ctx context.Context, client *redis.Client, id int64) error {
	return MarkUnknown(ctx, client, id)
}

func Read(ctx context.Context, client *redis.Client, id int64) (dto.RoutingStatusView, error) {
	if client == nil {
		return dto.RoutingStatusView{Known: false}, nil
	}
	data, err := readScript.Run(ctx, client, []string{key(id)}, int(TTL.Seconds()), int(watermarkRetention.Seconds())).Text()
	if err == redis.Nil {
		return dto.RoutingStatusView{Known: false}, nil
	}
	if err != nil {
		return dto.RoutingStatusView{Known: false}, err
	}
	var stored struct {
		ReceivedAt        int64  `json:"received_at"`
		ContentReceivedAt int64  `json:"content_received_at"`
		Sequence          string `json:"sequence"`
		Payload           string `json:"payload"`
		Verified          bool   `json:"verified"`
		Conflict          bool   `json:"conflict"`
	}
	var report dto.RoutingStatusReport
	if json.Unmarshal([]byte(data), &stored) != nil || !stored.Verified || stored.Conflict || json.Unmarshal([]byte(stored.Payload), &report) != nil {
		return dto.RoutingStatusView{Known: false}, nil
	}
	sequence, err := strconv.ParseUint(stored.Sequence, 10, 64)
	if err != nil || sequence == 0 || report.Version != 1 || !instancePattern.MatchString(report.InstanceID) || stored.ReceivedAt <= 0 || stored.ContentReceivedAt <= 0 {
		return dto.RoutingStatusView{Known: false}, nil
	}
	// received_at is the last accepted content, not an old packet heartbeat.
	report.ReceivedAt = stored.ContentReceivedAt
	report.LivenessAt = stored.ReceivedAt
	report.RoutingSequence = sequence
	return dto.RoutingStatusView{Known: true, ReporterVerified: true, Report: &report}, nil
}

func validCandidate(c *dto.RoutingCandidateIdentity, id int64) bool {
	if c.SourceRevision < 0 || c.LocalTransitionRevision < 0 {
		return false
	}
	switch c.Mode {
	case "ordered-v1-local":
		return c.Source == "server" && c.SourceServerID == id && c.SourceRevision > 0 && c.SourceRevision == c.LocalTransitionRevision
	case "ordered-v1-inherited":
		return c.Source == "global" && c.SourceServerID == 0 && c.SourceRevision > 0
	default:
		return false
	}
}
