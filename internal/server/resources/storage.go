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

// OpenSQL creates only the MariaDB connection used by the Root Migration Registry.
// It deliberately does not open Redis or construct any Repository.
func OpenSQL(ctx context.Context, cfg mysqlredis.Config) (*sql.DB, error) {
	if ctx == nil {
		return nil, errors.New("storage context is required")
	}
	connector, err := cfg.SQLConnector()
	if err != nil {
		return nil, fmt.Errorf("create MariaDB connector: %w", err)
	}
	sqlConfig := platformstorage.DefaultSQLConfig()
	sqlConfig.MaxOpenConns = cfg.DBMaxOpenConns
	sqlConfig.MaxIdleConns = cfg.DBMaxIdleConns
	sqlConfig.ConnMaxLifetime = cfg.DBConnMaxLifetime
	db, err := platformstorage.OpenSQL(ctx, connector, sqlConfig)
	if err != nil {
		return nil, fmt.Errorf("open MariaDB through Platform: %w", err)
	}
	return db, nil
}

// OpenApplicationResources creates Redis, the shared HTTP Client and the
// Analysis persistence adapter after the Root Migration Registry has completed.
// Ownership of db transfers to the returned Resources only on success; on
// failure the caller remains responsible for closing db.
func OpenApplicationResources(ctx context.Context, cfg mysqlredis.Config, db *sql.DB) (*Resources, *mysqlredis.Store, error) {
	if ctx == nil || db == nil {
		return nil, nil, errors.New("application storage context and MariaDB connection are required")
	}
	redisClient, err := platformstorage.OpenRedis(ctx, cfg.RedisOptions(), 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("open Redis through Platform: %w", err)
	}
	httpClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		return nil, nil, closePartialResources(
			fmt.Errorf("create shared Platform HTTP Client: %w", err),
			redisClient.Close,
		)
	}
	store, err := mysqlredis.NewWithResources(ctx, cfg, db, redisClient)
	if err != nil {
		httpClient.CloseIdleConnections()
		return nil, nil, closePartialResources(
			fmt.Errorf("initialize Analysis storage adapter: %w", err),
			redisClient.Close,
		)
	}
	return &Resources{DB: db, Redis: redisClient, HTTPClient: httpClient}, store, nil
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
