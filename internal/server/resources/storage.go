// Package resources owns the lifetime of shared Platform resources used by Server.
package resources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// Resources contains shared Platform clients owned by the Root Server.
type Resources struct {
	DB         *sql.DB
	Redis      *redis.Client
	HTTPClient *platformhttpclient.Client

	closeOnce sync.Once
	closeErr  error
}

// Storage is kept as a compatibility alias for the transitional Analysis adapter.
type Storage = Resources

// OpenLegacyAnalysisResources creates shared Platform resources and adapts the
// SQL/Redis clients to the still-running legacy Analysis persistence interfaces.
func OpenLegacyAnalysisResources(ctx context.Context, cfg mysqlredis.Config) (*Resources, *mysqlredis.Store, error) {
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

	httpClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, fmt.Errorf("create shared Platform HTTP Client: %w", err)
	}

	store, err := mysqlredis.NewWithResources(ctx, cfg, db, redisClient)
	if err != nil {
		httpClient.CloseIdleConnections()
		_ = redisClient.Close()
		_ = db.Close()
		return nil, nil, fmt.Errorf("initialize legacy Analysis storage: %w", err)
	}
	return &Resources{DB: db, Redis: redisClient, HTTPClient: httpClient}, store, nil
}

// OpenLegacyAnalysisStorage preserves the old constructor name during migration.
func OpenLegacyAnalysisStorage(ctx context.Context, cfg mysqlredis.Config) (*Storage, *mysqlredis.Store, error) {
	return OpenLegacyAnalysisResources(ctx, cfg)
}

// Close releases shared resources in reverse dependency order. It is safe to
// call more than once; module consumers must stop before Server closes resources.
func (r *Resources) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		var errs []error
		if r.HTTPClient != nil {
			r.HTTPClient.CloseIdleConnections()
		}
		if r.Redis != nil {
			if err := r.Redis.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if r.DB != nil {
			if err := r.DB.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		r.closeErr = errors.Join(errs...)
	})
	return r.closeErr
}
