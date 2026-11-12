package repo

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/cache"

	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ repository.ClientRepo = (*clientRepo)(nil)

type clientRepo struct {
	cache.CachedConn
}

const clientListCacheKey = "cache:subscribe:applications:v1"

// NewClientRepo builds the module-owned implementation.
func NewClientRepo(conn cache.CachedConn) repository.ClientRepo {
	return &clientRepo{CachedConn: conn}
}

func (m *clientRepo) Insert(ctx context.Context, data *client.SubscribeApplication) error {
	return m.mutate(ctx, data.IsDefault, 0, func(tx *gorm.DB) error {
		return tx.Model(&client.SubscribeApplication{}).Create(data).Error
	})
}

func (m *clientRepo) FindOne(ctx context.Context, id int64) (*client.SubscribeApplication, error) {
	var resp client.SubscribeApplication
	key := fmt.Sprintf("cache:subscribe:application:%d", id)
	if err := m.QueryCtx(ctx, &resp, key, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&client.SubscribeApplication{}).Where("id = ?", id).First(v).Error
	}); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (m *clientRepo) Update(ctx context.Context, data *client.SubscribeApplication) error {
	return m.mutate(ctx, data.IsDefault, data.Id, func(tx *gorm.DB) error {
		return tx.Model(&client.SubscribeApplication{}).Where("id = ?", data.Id).Save(data).Error
	})
}

func (m *clientRepo) Delete(ctx context.Context, id int64) error {
	return m.mutate(ctx, false, 0, func(tx *gorm.DB) error {
		return tx.Where("id = ?", id).Delete(&client.SubscribeApplication{}).Error
	})
}

// mutate runs change with every application row locked so concurrent saves
// serialize; a new default clears the others (except keep), and the change
// is rolled back unless existing applications end with exactly one default.
func (m *clientRepo) mutate(ctx context.Context, makeDefault bool, keep int64, change func(tx *gorm.DB) error) error {
	var ids []int64
	err := m.TransactCtx(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&client.SubscribeApplication{}).Clauses(clause.Locking{Strength: "UPDATE"}).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if err := change(tx); err != nil {
			return err
		}
		if makeDefault {
			if keep == 0 {
				// Insert: the new row has the largest id once created.
				if err := tx.Model(&client.SubscribeApplication{}).Select("MAX(id)").Scan(&keep).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&client.SubscribeApplication{}).Where("id <> ? AND is_default = ?", keep, true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		var total, defaults int64
		if err := tx.Model(&client.SubscribeApplication{}).Count(&total).Error; err != nil {
			return err
		}
		if err := tx.Model(&client.SubscribeApplication{}).Where("is_default = ?", true).Count(&defaults).Error; err != nil {
			return err
		}
		if total > 0 && defaults != 1 {
			return repository.ErrDefaultApplicationRequired
		}
		return nil
	})
	if err != nil {
		return err
	}
	keys := []string{clientListCacheKey}
	for _, id := range append(ids, keep) {
		keys = append(keys, fmt.Sprintf("cache:subscribe:application:%d", id))
	}
	_ = m.DelCacheCtx(ctx, keys...)
	return nil
}

func (m *clientRepo) List(ctx context.Context) ([]*client.SubscribeApplication, error) {
	var resp []*client.SubscribeApplication
	if err := m.QueryCtx(ctx, &resp, clientListCacheKey, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&client.SubscribeApplication{}).Order("id ASC").Find(v).Error
	}); err != nil {
		return nil, err
	}
	return resp, nil
}
