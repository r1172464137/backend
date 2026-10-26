package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/repository"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ repository.NodeRepo = (*nodeRepo)(nil)

type nodeRepo struct {
	*gorm.DB
	Cache        *redis.Client
	cacheRetrier *ServerCacheInvalidationRetrier
}

var (
	setServerCacheIfGenerationScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then current = '0' end
if current ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
redis.call('SADD', KEYS[3], KEYS[2])
redis.call('PEXPIRE', KEYS[3], ARGV[3])
return 1
`)
	clearServerCacheScript = redis.NewScript(`
redis.call('INCR', KEYS[1])
local keys = redis.call('SMEMBERS', KEYS[2])
for _, key in ipairs(keys) do redis.call('DEL', key) end
redis.call('DEL', KEYS[2])
for _, key in ipairs(ARGV) do redis.call('DEL', key) end
return #keys
`)
)

// NewNodeRepo builds the module-owned implementation.
func NewNodeRepo(db *gorm.DB, cache *redis.Client, retriers ...*ServerCacheInvalidationRetrier) repository.NodeRepo {
	var retrier *ServerCacheInvalidationRetrier
	if len(retriers) > 0 {
		retrier = retriers[0]
	}
	return &nodeRepo{
		DB:           db,
		Cache:        cache,
		cacheRetrier: retrier,
	}
}

// nodeInSet 支持多值 OR 查询
func nodeInSet(field string, values []string) func(db *gorm.DB) *gorm.DB {
	return orm.CommaSeparatedContains(field, values)
}

// NodeUserListCacheKeys resolves the server user-list cache keys for the
// given node ids and node tags; the subscription bundle invalidates them
// when a plan's node set changes (repository.NodeCacheKeyBridge).
func (m *nodeRepo) NodeUserListCacheKeys(ctx context.Context, nodeIDs []int64, tags []string) ([]string, error) {
	keys := make([]string, 0)
	appendKeys := func(nodes []*node.Node) {
		for _, n := range nodes {
			keys = append(keys, fmt.Sprintf("%s%d", node.ServerUserListCacheKey, n.ServerId))
			keys = append(keys, fmt.Sprintf("%s%d:%s", node.ServerUserListCacheKey, n.ServerId, n.Protocol))
		}
	}
	if len(nodeIDs) > 0 {
		var nodes []*node.Node
		if err := m.DB.WithContext(ctx).Model(&node.Node{}).Where("id IN (?)", nodeIDs).Find(&nodes).Error; err != nil {
			return nil, err
		}
		appendKeys(nodes)
	}
	if len(tags) > 0 {
		var nodes []*node.Node
		if err := m.DB.WithContext(ctx).Model(&node.Node{}).Scopes(nodeInSet("tags", tags)).Find(&nodes).Error; err != nil {
			return nil, err
		}
		appendKeys(nodes)
	}
	return keys, nil
}

func (m *nodeRepo) InsertServer(ctx context.Context, data *node.Server, tx ...*gorm.DB) error {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Create(data).Error
}

func (m *nodeRepo) FindOneServer(ctx context.Context, id int64) (*node.Server, error) {
	var server node.Server
	err := m.WithContext(ctx).Model(&node.Server{}).Where("id = ?", id).First(&server).Error
	return &server, err
}

func (m *nodeRepo) FindOneServerForUpdate(ctx context.Context, id int64) (*node.Server, error) {
	var server node.Server
	err := m.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Model(&node.Server{}).Where("id = ?", id).First(&server).Error
	return &server, err
}

func (m *nodeRepo) UpdateServer(ctx context.Context, data *node.Server, tx ...*gorm.DB) error {
	_, err := m.FindOneServer(ctx, data.Id)
	if err != nil {
		return err
	}

	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Where("id = ?", data.Id).Save(data).Error
}

// UpdateServerProtocolsIfCurrent persists node-reported protocol metadata
// without running the full-row server update hooks. The compare-and-swap guard
// prevents a heartbeat based on stale data from overwriting a concurrent admin
// configuration change.
func (m *nodeRepo) UpdateServerProtocolsIfCurrent(ctx context.Context, id int64, current, updated string, tx ...*gorm.DB) (bool, error) {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	result := db.WithContext(ctx).Model(&node.Server{}).
		Where("id = ? AND protocols = ?", id, current).
		UpdateColumns(map[string]interface{}{
			"protocols":  updated,
			"updated_at": time.Now(),
		})
	return result.RowsAffected == 1, result.Error
}

func (m *nodeRepo) BatchUpdateServerLastReportedAt(ctx context.Context, reports map[int64]time.Time, tx ...*gorm.DB) error {
	if len(reports) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(reports))
	for id := range reports {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	expr, args := serverLastReportedAtExpr(db, ids, reports)
	return db.WithContext(ctx).Model(&node.Server{}).Where("id IN ?", ids).
		Update("last_reported_at", gorm.Expr(expr, args...)).Error
}

func serverLastReportedAtExpr(db *gorm.DB, ids []int64, reports map[int64]time.Time) (string, []interface{}) {
	idColumn := serverColumn(db, "id")
	targetColumn := serverColumn(db, "last_reported_at")
	parts := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids)*2)
	for _, id := range ids {
		parts = append(parts, "WHEN ? THEN ?")
		args = append(args, id, reports[id])
	}
	return fmt.Sprintf("CASE %s %s ELSE %s END", idColumn, strings.Join(parts, " "), targetColumn), args
}

func serverColumn(db *gorm.DB, column string) string {
	if db != nil && db.Statement != nil {
		return db.Statement.Quote(clause.Column{Table: (&node.Server{}).TableName(), Name: column})
	}
	return (&node.Server{}).TableName() + "." + column
}

func (m *nodeRepo) DeleteServer(ctx context.Context, id int64, tx ...*gorm.DB) error {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Where("id = ?", id).Delete(&node.Server{}).Error
}

func (m *nodeRepo) FindServerConfigOverride(ctx context.Context, serverId int64) (*node.ServerConfigOverride, error) {
	var data []node.ServerConfigOverride

	err := m.WithContext(ctx).Model(&node.ServerConfigOverride{}).Where("server_id = ?", serverId).Limit(1).Find(&data).Error
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	return &data[0], nil
}

func (m *nodeRepo) SaveServerConfigOverride(ctx context.Context, data *node.ServerConfigOverride, tx ...*gorm.DB) error {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}

	var old node.ServerConfigOverride
	err := db.WithContext(ctx).Model(&node.ServerConfigOverride{}).Where("server_id = ?", data.ServerId).First(&old).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		data.Id = old.Id
		data.CreatedAt = old.CreatedAt
	}

	return db.WithContext(ctx).Save(data).Error
}

func (m *nodeRepo) DeleteServerConfigOverride(ctx context.Context, serverId int64, tx ...*gorm.DB) error {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Where("server_id = ?", serverId).Delete(&node.ServerConfigOverride{}).Error
}

func (m *nodeRepo) InsertNode(ctx context.Context, data *node.Node, tx ...*gorm.DB) error {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Create(data).Error
}

func (m *nodeRepo) FindOneNode(ctx context.Context, id int64) (*node.Node, error) {
	var n node.Node
	err := m.WithContext(ctx).Model(&node.Node{}).Where("id = ?", id).First(&n).Error
	return &n, err
}

func (m *nodeRepo) UpdateNode(ctx context.Context, data *node.Node, tx ...*gorm.DB) error {
	_, err := m.FindOneNode(ctx, data.Id)
	if err != nil {
		return err
	}

	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Where("id = ?", data.Id).Save(data).Error
}

func (m *nodeRepo) DeleteNode(ctx context.Context, id int64, tx ...*gorm.DB) error {
	db := m.DB
	if len(tx) > 0 {
		db = tx[0]
	}
	return db.WithContext(ctx).Where("id = ?", id).Delete(&node.Node{}).Error
}

// UpdateStatusCache Update server status to cache
func (m *nodeRepo) UpdateStatusCache(ctx context.Context, serverId int64, status *node.Status) error {
	key := fmt.Sprintf(node.StatusCacheKey, serverId)
	return m.Cache.Set(ctx, key, status.Marshal(), node.Expiry).Err()
}

// DeleteStatusCache Delete server status from cache
func (m *nodeRepo) DeleteStatusCache(ctx context.Context, serverId int64) error {
	key := fmt.Sprintf(node.StatusCacheKey, serverId)
	return m.Cache.Del(ctx, key).Err()
}

// StatusCache Get server status from cache
func (m *nodeRepo) StatusCache(ctx context.Context, serverId int64) (node.Status, error) {
	var status node.Status
	key := fmt.Sprintf(node.StatusCacheKey, serverId)

	result, err := m.Cache.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return status, nil
		}
		return status, err
	}
	if result == "" {
		return status, nil
	}
	err = status.Unmarshal(result)
	return status, err
}

// OnlineUserSubscribe Get online user subscribe
func (m *nodeRepo) OnlineUserSubscribe(ctx context.Context, serverId int64, protocol string) (node.OnlineUserSubscribe, error) {
	key := fmt.Sprintf(node.OnlineUserCacheKeyWithSubscribe, serverId, protocol)
	result, err := m.Cache.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return node.OnlineUserSubscribe{}, nil
		}
		return nil, err
	}
	if result == "" {
		return node.OnlineUserSubscribe{}, nil
	}
	var subscribe node.OnlineUserSubscribe
	err = json.Unmarshal([]byte(result), &subscribe)
	return subscribe, err
}

// UpdateOnlineUserSubscribe Update online user subscribe
func (m *nodeRepo) UpdateOnlineUserSubscribe(ctx context.Context, serverId int64, protocol string, subscribe node.OnlineUserSubscribe) error {
	key := fmt.Sprintf(node.OnlineUserCacheKeyWithSubscribe, serverId, protocol)
	data, err := json.Marshal(subscribe)
	if err != nil {
		return err
	}
	return m.Cache.Set(ctx, key, data, node.Expiry).Err()
}

// DeleteOnlineUserSubscribe Delete online user subscribe
func (m *nodeRepo) DeleteOnlineUserSubscribe(ctx context.Context, serverId int64, protocol string) error {
	key := fmt.Sprintf(node.OnlineUserCacheKeyWithSubscribe, serverId, protocol)
	return m.Cache.Del(ctx, key).Err()
}

// OnlineUserSubscribeGlobal Get global online user subscribe count
func (m *nodeRepo) OnlineUserSubscribeGlobal(ctx context.Context) (int64, error) {
	now := time.Now().Unix()
	// Clear expired data
	if err := m.Cache.ZRemRangeByScore(ctx, node.OnlineUserSubscribeCacheKeyWithGlobal, "-inf", fmt.Sprintf("%d", now)).Err(); err != nil {
		return 0, err
	}
	return m.Cache.ZCard(ctx, node.OnlineUserSubscribeCacheKeyWithGlobal).Result()
}

// UpdateOnlineUserSubscribeGlobal Update global online user subscribe count
func (m *nodeRepo) UpdateOnlineUserSubscribeGlobal(ctx context.Context, subscribe node.OnlineUserSubscribe) error {
	now := time.Now()
	expireTime := now.Add(5 * time.Minute).Unix() // set expire time 5 minutes later

	pipe := m.Cache.Pipeline()

	// Clear expired data
	pipe.ZRemRangeByScore(ctx, node.OnlineUserSubscribeCacheKeyWithGlobal, "-inf", fmt.Sprintf("%d", now.Unix()))
	// Add or update each subscribe with new expire time
	for sub := range subscribe {
		// Use ZAdd to add or update the member with new score (expire time)
		pipe.ZAdd(ctx, node.OnlineUserSubscribeCacheKeyWithGlobal, redis.Z{
			Score:  float64(expireTime),
			Member: sub,
		})
	}

	_, err := pipe.Exec(ctx)
	return err
}

// DeleteOnlineUserSubscribeGlobal Delete global online user subscribe count
func (m *nodeRepo) DeleteOnlineUserSubscribeGlobal(ctx context.Context) error {
	return m.Cache.Del(ctx, node.OnlineUserSubscribeCacheKeyWithGlobal).Err()
}

// FilterServerList Filter Server List
func (m *nodeRepo) FilterServerList(ctx context.Context, params *node.FilterParams) (int64, []*node.Server, error) {
	var servers []*node.Server
	var total int64
	query := m.WithContext(ctx).Model(&node.Server{})
	if params == nil {
		params = &node.FilterParams{
			Page: 1,
			Size: 10,
		}
	}
	params.Page, params.Size = repository.NormalizePageFloor(params.Page, params.Size)
	if params.Search != "" {
		query = query.Scopes(orm.PrefixLike([]string{"name", "address"}, params.Search))
	}
	if len(params.Ids) > 0 {
		query = query.Where("id IN ?", params.Ids)
	}
	err := query.Count(&total).Order("sort ASC").Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(&servers).Error
	return total, servers, err
}

func (m *nodeRepo) QueryServerList(ctx context.Context, ids []int64) (servers []*node.Server, err error) {
	query := m.WithContext(ctx).Model(&node.Server{})
	err = query.Where("id IN (?)", ids).Find(&servers).Error
	return
}

func (m *nodeRepo) QueryServerSorts(ctx context.Context) ([]node.SortItem, error) {
	var items []node.SortItem
	err := m.WithContext(ctx).Model(&node.Server{}).Select("id", "sort").Order("sort ASC").Find(&items).Error
	return items, err
}

func (m *nodeRepo) UpdateServerSort(ctx context.Context, id int64, sort int64) error {
	server, err := m.FindOneServer(ctx, id)
	if err != nil {
		return err
	}
	server.Sort = int(sort)
	return m.UpdateServer(ctx, server)
}

func (m *nodeRepo) nodeListQuery(ctx context.Context, params *node.FilterNodeParams) *gorm.DB {
	query := m.WithContext(ctx).Model(&node.Node{})
	if params == nil {
		params = &node.FilterNodeParams{}
	}
	if params.Search != "" {
		pattern := orm.LikePrefixPattern(params.Search)
		condition := "(name LIKE ?" + orm.LikeEscapeClause() + " OR address LIKE ?" + orm.LikeEscapeClause() + " OR tags LIKE ?" + orm.LikeEscapeClause()
		args := []interface{}{pattern, pattern, pattern}
		if port, err := strconv.ParseUint(params.Search, 10, 16); err == nil {
			condition += " OR port = ?"
			args = append(args, uint16(port))
		}
		condition += ")"
		query = query.Where(condition, args...)
	}
	if len(params.NodeId) > 0 {
		query = query.Where("id IN ?", params.NodeId)
	}
	if len(params.ServerId) > 0 {
		query = query.Where("server_id IN ?", params.ServerId)
	}
	if len(params.Tag) > 0 {
		query = query.Scopes(nodeInSet("tags", params.Tag))
	}
	if params.Protocol != "" {
		query = query.Where("protocol = ?", params.Protocol)
	}

	if params.Enabled != nil {
		query = query.Where("enabled = ?", *params.Enabled)
	}

	if params.Preload {
		query = query.Preload("Server")
	}
	return query
}

// FilterNodeList Filter Node List
func (m *nodeRepo) FilterNodeList(ctx context.Context, params *node.FilterNodeParams) (int64, []*node.Node, error) {
	if params == nil {
		params = &node.FilterNodeParams{}
	}
	params.Page, params.Size = repository.NormalizePageFloor(params.Page, params.Size)
	var nodes []*node.Node
	var total int64
	query := m.nodeListQuery(ctx, params)

	err := query.Count(&total).Order("sort ASC").Limit(params.Size).Offset((params.Page - 1) * params.Size).Find(&nodes).Error
	return total, nodes, err
}

// ListNodes is the unpaginated internal hot-path query. Unlike the admin list,
// callers already provide a bounded server/plan scope and do not need COUNT.
func (m *nodeRepo) ListNodes(ctx context.Context, params *node.FilterNodeParams) ([]*node.Node, error) {
	var nodes []*node.Node
	err := m.nodeListQuery(ctx, params).Order("sort ASC").Find(&nodes).Error
	return nodes, err
}

// ListNodesByScope applies the plan's explicit-node and tag selectors with OR
// semantics in one query. This is the shared read path for all subscription
// renderers, avoiding two node scans and inconsistent selector behavior.
func (m *nodeRepo) ListNodesByScope(ctx context.Context, nodeIDs []int64, tags []string, enabled *bool, preload bool) ([]*node.Node, error) {
	query := m.nodeListQuery(ctx, &node.FilterNodeParams{Enabled: enabled, Preload: preload})
	conditions := make([]string, 0, 2)
	args := make([]interface{}, 0, len(tags)+1)
	if len(nodeIDs) > 0 {
		conditions = append(conditions, "id IN ?")
		args = append(args, nodeIDs)
	}
	if condition, values := orm.CommaSeparatedContainsCondition(query, "tags", tags); condition != "" {
		conditions = append(conditions, condition)
		args = append(args, values...)
	}
	if len(conditions) == 0 {
		return m.ListNodes(ctx, &node.FilterNodeParams{Enabled: enabled, Preload: preload})
	}
	var nodes []*node.Node
	err := query.Where("("+strings.Join(conditions, " OR ")+")", args...).Order("sort ASC").Find(&nodes).Error
	return nodes, err
}

func (m *nodeRepo) QueryNodeSorts(ctx context.Context) ([]node.SortItem, error) {
	var items []node.SortItem
	err := m.WithContext(ctx).Model(&node.Node{}).Select("id", "sort").Order("sort ASC").Find(&items).Error
	return items, err
}

func (m *nodeRepo) UpdateNodeSort(ctx context.Context, id int64, sort int64) error {
	n, err := m.FindOneNode(ctx, id)
	if err != nil {
		return err
	}
	n.Sort = int(sort)
	return m.UpdateNode(ctx, n)
}

func (m *nodeRepo) QueryNodeTags(ctx context.Context) ([]string, error) {
	var tags []string
	err := m.WithContext(ctx).Model(&node.Node{}).Pluck("tags", &tags).Error
	return tags, err
}

func (m *nodeRepo) CountEnabledNodes(ctx context.Context) (int64, error) {
	var total int64
	err := m.WithContext(ctx).Model(&node.Node{}).Where("enabled = ?", true).Count(&total).Error
	return total, err
}

func (m *nodeRepo) CountServersByReportStatus(ctx context.Context, cutoff time.Time) (int64, int64, error) {
	var online int64
	if err := m.WithContext(ctx).Model(&node.Server{}).Where("last_reported_at > ?", cutoff).Count(&online).Error; err != nil {
		return 0, 0, err
	}

	var offline int64
	if err := m.WithContext(ctx).Model(&node.Server{}).Where("last_reported_at <= ? OR last_reported_at IS NULL", cutoff).Count(&offline).Error; err != nil {
		return 0, 0, err
	}

	return online, offline, nil
}

func (m *nodeRepo) QueryServerAddresses(ctx context.Context) ([]string, error) {
	var addresses []string
	err := m.WithContext(ctx).Model(&node.Server{}).Pluck("address", &addresses).Error
	return addresses, err
}

func (m *nodeRepo) QueryEnabledNodeProtocols(ctx context.Context) ([]string, error) {
	var protocols []string
	err := m.WithContext(ctx).Model(&node.Node{}).Where("enabled = ?", true).Pluck("protocol", &protocols).Error
	return protocols, err
}

func (m *nodeRepo) ServerCacheGeneration(ctx context.Context, serverId int64) (int64, error) {
	if serverId <= 0 {
		return 0, nil
	}
	generation, err := m.Cache.Get(ctx, fmt.Sprintf(node.ServerCacheGenerationKey, serverId)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return generation, err
}

// SetServerCache stores a response only when the generation captured before
// the database read still matches. This prevents pre-update readers from
// repopulating stale data after ClearServerCache runs.
func (m *nodeRepo) SetServerCache(ctx context.Context, serverId int64, key string, value interface{}, generation int64) error {
	if serverId <= 0 || key == "" {
		return nil
	}
	indexKey := fmt.Sprintf(node.ServerCacheIndexKey, serverId)
	_, err := setServerCacheIfGenerationScript.Run(ctx, m.Cache,
		[]string{fmt.Sprintf(node.ServerCacheGenerationKey, serverId), key, indexKey},
		strconv.FormatInt(generation, 10), value, node.ServerCacheTTL.Milliseconds()).Result()
	return err
}

// ClearNodeCache Clear Node Cache
func (m *nodeRepo) ClearNodeCache(ctx context.Context, params *node.FilterNodeParams) error {
	nodes, err := m.ListNodes(ctx, params)
	if err != nil {
		return err
	}
	serverIDs := make(map[int64]struct{}, len(nodes))
	for _, n := range nodes {
		serverIDs[n.ServerId] = struct{}{}
	}
	for serverID := range serverIDs {
		if err := m.ClearServerCache(ctx, serverID); err != nil {
			return err
		}
	}
	return nil
}

// ClearServerCache Clear Server Cache
func (m *nodeRepo) ClearServerCache(ctx context.Context, serverId int64) error {
	err := clearServerCache(ctx, m.Cache, serverId)
	if err == nil || m.cacheRetrier == nil {
		return err
	}
	// The database write has already committed at every caller. Queue Redis
	// recovery rather than returning a failed write response that clients may
	// retry despite the durable change succeeding.
	m.cacheRetrier.Enqueue(serverId)
	return nil
}

func clearServerCache(ctx context.Context, client *redis.Client, serverId int64) error {
	indexKey := fmt.Sprintf(node.ServerCacheIndexKey, serverId)
	cacheKeys, err := client.SMembers(ctx, indexKey).Result()
	if err != nil {
		return err
	}
	// Entries created before the exact-key index was introduced are short-lived
	// (five minutes). Scan only when no index exists, preserving immediate
	// invalidation during a rolling upgrade without penalizing steady state.
	if len(cacheKeys) == 0 {
		patterns := []string{
			fmt.Sprintf("%s%d:*", node.ServerUserListCacheKey, serverId),
			fmt.Sprintf("%s%d:*", node.ServerConfigCacheKey, serverId),
		}
		for _, pattern := range patterns {
			var cursor uint64
			for {
				keys, newCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
				if err != nil {
					return err
				}
				if len(keys) > 0 {
					cacheKeys = append(cacheKeys, keys...)
				}
				cursor = newCursor
				if cursor == 0 {
					break
				}
			}
		}
	}
	cacheKeys = append(cacheKeys, fmt.Sprintf("%s%d", node.ServerUserListCacheKey, serverId))
	args := make([]interface{}, len(cacheKeys))
	for i, key := range cacheKeys {
		args[i] = key
	}
	_, err = clearServerCacheScript.Run(ctx, client,
		[]string{fmt.Sprintf(node.ServerCacheGenerationKey, serverId), indexKey}, args...).Result()
	return err
}
