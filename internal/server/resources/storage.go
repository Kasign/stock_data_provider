// Package resources owns the lifetime of shared Platform resources used by Server.
package resources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// Storage contains shared database and Redis clients owned by the Root Server.
type Storage struct {
	DB    *sql.DB
	Redis *redis.Client
}

// OpenLegacyAnalysisStorage creates connections through Platform and adapts
// them to the still-running legacy Analysis persistence interfaces.
func OpenLegacyAnalysisStorage(ctx context.Context, cfg mysqlredis.Config) (*Storage, *mysqlredis.Store, error) {
	if ctx == nil {
		return nil, nil, errors.New("storage context is required")
	}
	connector, err := cfg.SQLConnector()
	if err != nil {
		return nil, nil, fmt.Errorf("create MariaDB connector: %w", err)
	}
	sqlConfig := platformstorage.DefaultSQLConfig()
	sqlConfig.MaxOpenConns = cfg.DBMaxOpenConns
	sqlConfig.MaxIdleConns = cfg.DBMaxIdleConns
	sqlConfig.ConnMaxLifetime = cfg.DBConnMaxLifetime
	db, err := platformstorage.OpenSQL(ctx, connector, sqlConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("open MariaDB through Platform: %w", err)
	}

	redisClient, err := platformstorage.OpenRedis(ctx, cfg.RedisOptions(), 5*time.Second)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("open Redis through Platform: %w", err)
	}
	store, err := mysqlredis.NewWithResources(ctx, cfg, db, redisClient)
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, fmt.Errorf("initialize legacy Analysis storage: %w", err)
	}
	return &Storage{DB: db, Redis: redisClient}, store, nil
}

// Close releases shared resources in reverse dependency order.
func (s *Storage) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.Redis != nil {
		if err := s.Redis.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.DB != nil {
		if err := s.DB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
