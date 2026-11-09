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
	mu        sync.Mutex
	closed    bool
	closeErr  error
}

// Storage is kept as a compatibility alias for the transitional Analysis adapter.
type Storage = Resources

// BindHTTPDestination creates a BoundDoer from the shared Platform Client.
// The caller supplies the exact upstream allowlist; the returned doer cannot
// replace that policy per request. Binding is rejected once resources close.
func (r *Resources) BindHTTPDestination(policy platformhttpclient.DestinationPolicy) (*platformhttpclient.BoundDoer, error) {
	if r == nil {
		return nil, errors.New("server resources are nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("server resources are closed")
	}
	if r.HTTPClient == nil {
		return nil, errors.New("shared Platform HTTP Client is not configured")
	}
	return r.HTTPClient.Bind(policy)
}

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
		return nil, nil, closePartialResources(
			fmt.Errorf("open Redis through Platform: %w", err),
			db.Close,
		)
	}

	httpClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		return nil, nil, closePartialResources(
			fmt.Errorf("create shared Platform HTTP Client: %w", err),
			redisClient.Close,
			db.Close,
		)
	}

	store, err := mysqlredis.NewWithResources(ctx, cfg, db, redisClient)
	if err != nil {
		httpClient.CloseIdleConnections()
		return nil, nil, closePartialResources(
			fmt.Errorf("initialize legacy Analysis storage: %w", err),
			redisClient.Close,
			db.Close,
		)
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
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
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


// closePartialResources releases successfully opened dependencies in reverse
// creation order and preserves both the initialization failure and cleanup
// failures for startup diagnostics.
func closePartialResources(cause error, closers ...func() error) error {
	joined := cause
	for _, closeResource := range closers {
		if closeResource == nil {
			continue
		}
		if err := closeResource(); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}
