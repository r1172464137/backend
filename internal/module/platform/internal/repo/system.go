package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/perfect-panel/server/internal/repository"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	cacheSystemIdPrefix  = "cache:System:id:"
	cacheSystemKeyPrefix = "cache:System:key:"
)

var _ repository.SystemRepo = (*systemRepo)(nil)

type systemRepo struct {
	cache.CachedConn
	table string
}

// NewSystemRepo builds the module-owned implementation over the shared
// cached connection.
func NewSystemRepo(conn cache.CachedConn) repository.SystemRepo {
	return &systemRepo{
		CachedConn: conn,
		table:      "System",
	}
}

func (m *systemRepo) getCacheKeys(data *system.System) []string {
	if data == nil {
		return []string{}
	}
	keys := []string{fmt.Sprintf("%s%v", cacheSystemIdPrefix, data.Id)}
	if data.Key != "" {
		keys = append(keys, fmt.Sprintf("%s%v", cacheSystemKeyPrefix, data.Key))
	}
	return append(keys, systemCategoryCacheKeys(data.Category)...)
}

func systemCategoryCacheKeys(category string) []string {
	switch category {
	case "sms":
		return []string{config.SmsConfigKey}
	case "site":
		return []string{config.SiteConfigKey, config.GlobalConfigKey}
	case "email":
		return []string{config.EmailSmtpConfigKey}
	case "subscribe":
		return []string{config.SubscribeConfigKey, config.GlobalConfigKey}
	case "register":
		return []string{config.RegisterConfigKey, config.GlobalConfigKey}
	case "verify":
		return []string{config.VerifyConfigKey, config.GlobalConfigKey}
	case "server":
		return []string{config.NodeConfigKey}
	case "invite":
		return []string{config.InviteConfigKey, config.GlobalConfigKey}
	case "telegram":
		return []string{config.TelegramConfigKey}
	case "tos":
		return []string{config.TosConfigKey}
	case "currency":
		return []string{config.CurrencyConfigKey, config.GlobalConfigKey}
	case "verify_code":
		return []string{config.VerifyCodeConfigKey}
	default:
		return nil
	}
}

func (m *systemRepo) FindOneByKey(ctx context.Context, key string) (*system.System, error) {
	sys := new(system.System)
	cacheKey := fmt.Sprintf("%s%v", cacheSystemKeyPrefix, key)
	err := m.QueryCtx(ctx, sys, cacheKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&system.System{}).Scopes(systemWhereKey(key)).First(v).Error
	})
	return sys, err
}

func (m *systemRepo) Insert(ctx context.Context, data *system.System) error {
	err := m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Create(&data).Error
	}, m.getCacheKeys(data)...)
	return err
}

func (m *systemRepo) FindOne(ctx context.Context, id int64) (*system.System, error) {
	SystemIdKey := fmt.Sprintf("%s%v", cacheSystemIdPrefix, id)
	var resp system.System
	err := m.QueryCtx(ctx, &resp, SystemIdKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&system.System{}).Where("id = ?", id).First(&resp).Error
	})
	switch {
	case err == nil:
		return &resp, nil
	default:
		return nil, err
	}
}

func (m *systemRepo) Update(ctx context.Context, data *system.System) error {
	old, err := m.FindOne(ctx, data.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	keys := append(m.getCacheKeys(old), m.getCacheKeys(data)...)
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		db := conn
		return db.Save(data).Error
	}, keys...)
	return err
}

func (m *systemRepo) Delete(ctx context.Context, id int64) error {
	data, err := m.FindOne(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		db := conn
		return db.Delete(&system.System{}, id).Error
	}, m.getCacheKeys(data)...)
	return err
}

// GetSmsConfig returns the sms config.
func (m *systemRepo) GetSmsConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.SmsConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "sms").Find(v).Error
	})
	return configs, err
}

// GetSiteConfig returns the site config.
func (m *systemRepo) GetSiteConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.SiteConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "site").Find(v).Error
	})
	return configs, err
}

// GetEmailConfig returns the email config.
func (m *systemRepo) GetEmailConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.EmailSmtpConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "email").Find(v).Error
	})
	return configs, err
}

// GetSubscribeConfig returns the subscribe config.
func (m *systemRepo) GetSubscribeConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.SubscribeConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "subscribe").Find(v).Error
	})
	return configs, err
}

// GetRegisterConfig returns the register config.
func (m *systemRepo) GetRegisterConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.RegisterConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "register").Find(v).Error
	})
	return configs, err
}

// GetVerifyConfig returns the verify config.
func (m *systemRepo) GetVerifyConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.VerifyConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "verify").Find(v).Error
	})
	return configs, err
}

// GetNodeConfigUncached reads the authoritative database, never Redis.
func (m *systemRepo) GetNodeConfigUncached(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryNoCacheCtx(ctx, &configs, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "server").Not(map[string]any{"key": []string{"Block", "BlockIPs", "BlockProtocols", "Outbound"}}).Order("id ASC").Find(v).Error
	})
	return configs, err
}

// GetLegacyNodeConfigUncached is reserved for the offline 02179 cutover.
func (m *systemRepo) GetLegacyNodeConfigUncached(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryNoCacheCtx(ctx, &configs, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "server").Order("id ASC").Find(v).Error
	})
	return configs, err
}

// GetNodeConfigForUpdate returns one uncached, locked snapshot. Callers must
// invoke it inside InPlatformTx; the stable server-category rows serialize
// read/merge/write updates even when an additive key is being inserted.
func (m *systemRepo) GetNodeConfigForUpdate(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryNoCacheCtx(ctx, &configs, func(conn *gorm.DB, v interface{}) error {
		return conn.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("category = ?", "server").Not(map[string]any{"key": []string{"Block", "BlockIPs", "BlockProtocols", "Outbound"}}).Order("id ASC").Find(v).Error
	})
	return configs, err
}

// GetInviteConfig returns the invite config.
func (m *systemRepo) GetInviteConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.InviteConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "invite").Find(v).Error
	})
	return configs, err
}

// GetTelegramConfig returns the telegram config.
func (m *systemRepo) GetTelegramConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.TelegramConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "telegram").Find(v).Error
	})
	return configs, err
}

// GetTosConfig returns the tos config.
func (m *systemRepo) GetTosConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.TosConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "tos").Find(v).Error
	})
	return configs, err
}

// GetCurrencyConfig returns the currency config.
func (m *systemRepo) GetCurrencyConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.CurrencyConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "currency").Find(v).Error
	})
	return configs, err
}

func (m *systemRepo) UpdateValueByCategoryKey(ctx context.Context, category, key, value string, valueType ...string) error {
	err := m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		fieldType := "string"
		if len(valueType) > 0 && valueType[0] != "" {
			fieldType = valueType[0]
		}
		return conn.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&system.System{
			Category: category,
			Key:      key,
			Value:    value,
			Type:     fieldType,
			Desc:     key,
		}).Error
	})
	if err != nil {
		return err
	}
	// Cache invalidation is best-effort and is deferred automatically when the
	// repository is used through Store.InTx.
	_ = m.DelCacheCtx(ctx, append([]string{fmt.Sprintf("%s%v", cacheSystemKeyPrefix, key)}, systemCategoryCacheKeys(category)...)...)
	return nil
}

func (m *systemRepo) UpdateNodeMultiplierConfig(ctx context.Context, config string) error {
	err := m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		return conn.Model(&system.System{}).
			Scopes(systemWhereCategoryKey("server", "NodeMultiplierConfig")).
			Update("value", config).Error
	})
	if err != nil {
		return err
	}
	_ = m.DelCacheCtx(ctx, systemCategoryCacheKeys("server")...)
	return nil
}

func (m *systemRepo) FindNodeMultiplierConfig(ctx context.Context) (*system.System, error) {
	var data system.System
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Scopes(systemWhereCategoryKey("server", "NodeMultiplierConfig")).Find(v).Error
	})
	return &data, err
}

// GetVerifyCodeConfig returns the verify code config.
func (m *systemRepo) GetVerifyCodeConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryCtx(ctx, &configs, config.VerifyCodeConfigKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "verify_code").Find(v).Error
	})
	return configs, err
}

// GetLogConfig returns the log config.
func (m *systemRepo) GetLogConfig(ctx context.Context) ([]*system.System, error) {
	var configs []*system.System
	err := m.QueryNoCacheCtx(ctx, &configs, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("category = ?", "log").Find(v).Error
	})
	return configs, err
}

// systemWhereKey returns a GORM scope filtering by the "key" column.
// Migrated from internal/model/entity/system/scope.go (renamed with the system prefix
// to avoid colliding with other domain scopes inside the flat repository package).
func systemWhereKey(key string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(clause.Eq{
			Column: clause.Column{Name: "key"},
			Value:  key,
		})
	}
}

// systemWhereCategoryKey returns a GORM scope filtering by both "category" and "key".
// Migrated from internal/model/entity/system/scope.go (renamed with the system prefix
// to avoid colliding with other domain scopes inside the flat repository package).
func systemWhereCategoryKey(category, key string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(clause.Eq{
			Column: clause.Column{Name: "category"},
			Value:  category,
		}).Where(clause.Eq{
			Column: clause.Column{Name: "key"},
			Value:  key,
		})
	}
}
