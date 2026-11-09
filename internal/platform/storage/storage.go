// Package storage owns reusable database and Redis connection lifecycle helpers
// and the namespace-scoped SQL migration runner.
package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

const (
	defaultSQLMaxOpenConns    = 20
	defaultSQLMaxIdleConns    = 5
	defaultSQLConnMaxLifetime = 30 * time.Minute
	defaultSQLConnMaxIdleTime = 5 * time.Minute
	defaultPingTimeout        = 5 * time.Second
	maxSQLMaxOpenConns        = 128
	defaultRedisPoolSize      = 20
	maxRedisPoolSize          = 128
	maxPingTimeout            = 30 * time.Second
	migrationLockWaitSeconds  = 10
)

var (
	errMigrationLockUnavailable = errors.New("migration lock is unavailable")
	errMigrationHistoryMismatch = errors.New("migration history does not match the registered migrations")
)

type SQLConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

// DefaultSQLConfig returns conservative pool settings for a single service instance.
func DefaultSQLConfig() SQLConfig {
	return SQLConfig{
		MaxOpenConns:    defaultSQLMaxOpenConns,
		MaxIdleConns:    defaultSQLMaxIdleConns,
		ConnMaxLifetime: defaultSQLConnMaxLifetime,
		ConnMaxIdleTime: defaultSQLConnMaxIdleTime,
		PingTimeout:     defaultPingTimeout,
	}
}

// OpenSQL opens a database from a driver connector, applies bounded pool settings,
// and verifies connectivity before returning ownership to the caller.
func OpenSQL(ctx context.Context, connector driver.Connector, config SQLConfig) (*sql.DB, error) {
	if ctx == nil || connector == nil {
		return nil, fault.New(fault.ClassValidation, "platform.storage.invalid_sql_config", "SQL connection configuration is invalid", nil)
	}
	config, err := normalizeSQLConfig(config)
	if err != nil {
		return nil, fault.New(fault.ClassValidation, "platform.storage.invalid_sql_config", "SQL connection configuration is invalid", err)
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, config.PingTimeout)
	defer cancel()
	if err := PingSQL(pingCtx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// PingSQL verifies an SQL database connection using the caller's deadline.
func PingSQL(ctx context.Context, db *sql.DB) error {
	if ctx == nil || db == nil {
		return fault.New(fault.ClassValidation, "platform.storage.invalid_sql", "SQL database is not configured", nil)
	}
	if err := db.PingContext(ctx); err != nil {
		return dependencyFault("sql", "platform.storage.sql_unavailable", "SQL database is unavailable", err)
	}
	return nil
}

// OpenRedis opens a Redis client and verifies connectivity before returning it.
// The caller owns the returned client's lifecycle and must call Close.
func OpenRedis(ctx context.Context, options redis.Options, pingTimeout time.Duration) (*redis.Client, error) {
	if ctx == nil {
		return nil, fault.New(fault.ClassValidation, "platform.storage.invalid_redis_config", "Redis connection configuration is invalid", nil)
	}
	options, err := normalizeRedisOptions(options)
	if err != nil {
		return nil, fault.New(fault.ClassValidation, "platform.storage.invalid_redis_config", "Redis connection configuration is invalid", err)
	}
	pingTimeout, err = normalizePingTimeout(pingTimeout)
	if err != nil {
		return nil, fault.New(fault.ClassValidation, "platform.storage.invalid_redis_config", "Redis connection configuration is invalid", err)
	}

	client := redis.NewClient(&options)
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := PingRedis(pingCtx, client); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// PingRedis verifies Redis connectivity using the caller's deadline.
func PingRedis(ctx context.Context, client *redis.Client) error {
	if ctx == nil || client == nil {
		return fault.New(fault.ClassValidation, "platform.storage.invalid_redis", "Redis client is not configured", nil)
	}
	if err := client.Ping(ctx).Err(); err != nil {
		return dependencyFault("redis", "platform.storage.redis_unavailable", "Redis is unavailable", err)
	}
	return nil
}

func normalizeSQLConfig(config SQLConfig) (SQLConfig, error) {
	defaults := DefaultSQLConfig()
	if config.MaxOpenConns < 0 || config.MaxIdleConns < 0 ||
		config.ConnMaxLifetime < 0 || config.ConnMaxIdleTime < 0 || config.PingTimeout < 0 {
		return config, errors.New("SQL pool settings cannot be negative")
	}
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = defaults.MaxOpenConns
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = defaults.MaxIdleConns
	}
	if config.ConnMaxLifetime == 0 {
		config.ConnMaxLifetime = defaults.ConnMaxLifetime
	}
	if config.ConnMaxIdleTime == 0 {
		config.ConnMaxIdleTime = defaults.ConnMaxIdleTime
	}
	if config.PingTimeout == 0 {
		config.PingTimeout = defaults.PingTimeout
	}
	if config.MaxOpenConns > maxSQLMaxOpenConns || config.MaxIdleConns > config.MaxOpenConns {
		return config, errors.New("SQL connection pool limits are invalid")
	}
	if config.PingTimeout > maxPingTimeout {
		return config, errors.New("SQL ping timeout exceeds 30 seconds")
	}
	return config, nil
}

func normalizeRedisOptions(options redis.Options) (redis.Options, error) {
	if strings.TrimSpace(options.Addr) == "" || options.DB < 0 ||
		options.PoolSize < 0 || options.PoolSize > maxRedisPoolSize ||
		options.MaxActiveConns < 0 || options.MaxActiveConns > maxRedisPoolSize ||
		options.MinIdleConns < 0 || options.MaxIdleConns < 0 ||
		options.DialTimeout < 0 || options.ReadTimeout < 0 || options.WriteTimeout < 0 ||
		options.PoolTimeout < 0 || options.ConnMaxIdleTime < 0 || options.ConnMaxLifetime < 0 {
		return options, errors.New("Redis pool and timeout settings are invalid")
	}
	if options.PoolSize == 0 {
		options.PoolSize = defaultRedisPoolSize
	}
	if options.MaxActiveConns == 0 {
		options.MaxActiveConns = maxRedisPoolSize
	}
	if options.MaxIdleConns == 0 {
		options.MaxIdleConns = options.PoolSize
	}
	if options.PoolSize > options.MaxActiveConns ||
		options.MaxIdleConns > options.MaxActiveConns ||
		options.MinIdleConns > options.MaxIdleConns {
		return options, errors.New("Redis connection pool limits are inconsistent")
	}
	return options, nil
}

func normalizePingTimeout(timeout time.Duration) (time.Duration, error) {
	if timeout < 0 {
		return 0, errors.New("ping timeout cannot be negative")
	}
	if timeout == 0 {
		return defaultPingTimeout, nil
	}
	if timeout > maxPingTimeout {
		return 0, errors.New("ping timeout exceeds 30 seconds")
	}
	return timeout, nil
}

func dependencyFault(dependency, code, message string, cause error) *fault.Fault {
	class := fault.ClassUnavailable
	retryable := false
	if errors.Is(cause, context.DeadlineExceeded) {
		class = fault.ClassTimeout
	} else if !errors.Is(cause, context.Canceled) {
		var networkError net.Error
		if errors.As(cause, &networkError) {
			if networkError.Timeout() {
				class = fault.ClassTimeout
			}
			retryable = networkError.Timeout() || networkError.Temporary()
		}
	}
	return &fault.Fault{
		Class: class, Code: code, SafeMessage: message,
		Dependency: dependency, Retryable: retryable, Cause: cause,
	}
}

// Migration is an immutable, ordered schema change. Statements are executed
// individually because MariaDB DDL may implicitly commit.
type Migration struct {
	Version     uint64
	Description string
	Statements  []string
}

// ApplyMigrations applies the complete immutable migration set for one namespace.
// The caller must explicitly invoke it; opening a database never runs migrations.
func ApplyMigrations(ctx context.Context, db *sql.DB, namespace string, migrations []Migration) (resultErr error) {
	if ctx == nil || db == nil {
		return fault.New(fault.ClassValidation, "platform.storage.invalid_migration_target", "migration target is not configured", nil)
	}
	ordered, checksums, err := prepareMigrations(namespace, migrations)
	if err != nil {
		return fault.New(fault.ClassValidation, "platform.storage.invalid_migrations", "migration definitions are invalid", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return dependencyFault("sql", "platform.storage.migration_connection_failed", "migration database connection failed", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	lockName := migrationLockName(namespace)
	lockCtx, cancel := context.WithTimeout(ctx, defaultPingTimeout+5*time.Second)
	defer cancel()
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(lockCtx, "SELECT GET_LOCK(?, ?)", lockName, migrationLockWaitSeconds).Scan(&acquired); err != nil {
		// The server may have acquired the advisory lock even if the response
		// was lost or could not be decoded. Do not return an ambiguous session
		// to the connection pool.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return dependencyFault("sql", "platform.storage.migration_lock_failed", "could not acquire migration lock", err)
	}
	if !acquired.Valid {
		// MariaDB returns NULL when GET_LOCK cannot determine a result. Treat
		// ownership as ambiguous and discard the physical connection.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return dependencyFault("sql", "platform.storage.migration_lock_failed", "could not confirm migration lock ownership", errors.New("migration lock result is NULL"))
	}
	if acquired.Int64 != 1 {
		return dependencyFault("sql", "platform.storage.migration_lock_busy", "another migration is running", errMigrationLockUnavailable)
	}

	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), defaultPingTimeout)
		defer releaseCancel()
		var released sql.NullInt64
		releaseErr := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
		if releaseErr != nil || !released.Valid || released.Int64 != 1 {
			// MariaDB advisory locks belong to the physical connection. If release
			// cannot be confirmed, discard it instead of returning a locked
			// connection to database/sql's pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			if resultErr == nil {
				if releaseErr != nil {
					resultErr = dependencyFault("sql", "platform.storage.migration_lock_release_failed", "migration lock release could not be confirmed", releaseErr)
				} else {
					resultErr = fault.New(fault.ClassInternal, "platform.storage.migration_lock_release_failed", "migration lock release could not be confirmed", nil)
				}
			}
		}
	}()

	const createHistory = `CREATE TABLE IF NOT EXISTS platform_schema_migrations (
		namespace VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		version BIGINT UNSIGNED NOT NULL,
		description VARCHAR(255) NOT NULL,
		checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'applied',
		failure_statement INT UNSIGNED NOT NULL DEFAULT 0,
		applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		started_at DATETIME(6) NULL,
		finished_at DATETIME(6) NULL,
		failure_code VARCHAR(64) NOT NULL DEFAULT '',
		PRIMARY KEY (namespace, version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`
	if _, err := conn.ExecContext(ctx, createHistory); err != nil {
		return dependencyFault("sql", "platform.storage.migration_history_failed", "migration history storage is unavailable", err)
	}
	// Upgrade history tables created by earlier platform versions without changing
	// the meaning of the legacy applied_at column.
	for _, statement := range []string{
		"ALTER TABLE platform_schema_migrations ADD COLUMN IF NOT EXISTS started_at DATETIME(6) NULL",
		"ALTER TABLE platform_schema_migrations ADD COLUMN IF NOT EXISTS finished_at DATETIME(6) NULL",
		"ALTER TABLE platform_schema_migrations ADD COLUMN IF NOT EXISTS failure_code VARCHAR(64) NOT NULL DEFAULT ''",
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return dependencyFault("sql", "platform.storage.migration_history_failed", "migration history schema could not be upgraded", err)
		}
	}

	applied := make(map[uint64]string)
	rows, err := conn.QueryContext(ctx, "SELECT version, checksum, status, failure_statement FROM platform_schema_migrations WHERE namespace = ?", namespace)
	if err != nil {
		return dependencyFault("sql", "platform.storage.migration_history_failed", "migration history storage is unavailable", err)
	}
	for rows.Next() {
		var version uint64
		var checksum string
		var status string
		var failureStatement uint32
		if err := rows.Scan(&version, &checksum, &status, &failureStatement); err != nil {
			_ = rows.Close()
			return dependencyFault("sql", "platform.storage.migration_history_failed", "migration history could not be read", err)
		}
		if status != "applied" {
			_ = rows.Close()
			return fault.New(fault.ClassConflict, "platform.storage.migration_requires_recovery",
				"migration history contains an incomplete migration; manual recovery is required", nil)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return dependencyFault("sql", "platform.storage.migration_history_failed", "migration history could not be read", err)
	}
	if err := rows.Close(); err != nil {
		return dependencyFault("sql", "platform.storage.migration_history_failed", "migration history could not be read", err)
	}

	lastAppliedIndex := -1
	for version := range applied {
		index := -1
		for migrationIndex, migration := range ordered {
			if migration.Version == version {
				index = migrationIndex
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("%w: database contains unregistered version %d", errMigrationHistoryMismatch, version)
		}
		if index > lastAppliedIndex {
			lastAppliedIndex = index
		}
	}
	for index := 0; index <= lastAppliedIndex; index++ {
		if _, exists := applied[ordered[index].Version]; !exists {
			return fmt.Errorf("%w: migration version %d is missing before an applied version", errMigrationHistoryMismatch, ordered[index].Version)
		}
	}
	for _, migration := range ordered {
		checksum := checksums[migration.Version]
		if existing, exists := applied[migration.Version]; exists {
			if existing != checksum {
				return fmt.Errorf("%w: checksum changed for version %d", errMigrationHistoryMismatch, migration.Version)
			}
			continue
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO platform_schema_migrations
			(namespace, version, description, checksum, status, failure_statement, started_at, finished_at, failure_code)
			VALUES (?, ?, ?, ?, 'running', 0, CURRENT_TIMESTAMP(6), NULL, '')`,
			namespace, migration.Version, migration.Description, checksum); err != nil {
			return dependencyFault("sql", "platform.storage.migration_history_failed", "migration execution could not be recorded", err)
		}
		for index, statement := range migration.Statements {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				recordCtx, recordCancel := context.WithTimeout(context.Background(), defaultPingTimeout)
				recordResult, recordErr := conn.ExecContext(recordCtx, `UPDATE platform_schema_migrations
					SET status = 'failed', failure_statement = ?,
						failure_code = 'platform.storage.migration_apply_failed',
						finished_at = CURRENT_TIMESTAMP(6)
					WHERE namespace = ? AND version = ? AND status = 'running'`,
					index+1, namespace, migration.Version)
				if recordErr == nil {
					recordErr = requireSingleRowAffected(recordResult)
				}
				recordCancel()
				cause := err
				if recordErr != nil {
					cause = errors.Join(err, recordErr)
				}
				return &fault.Fault{
					Class: fault.ClassInternal, Code: "platform.storage.migration_apply_failed",
					SafeMessage: "migration failed; manual recovery is required",
					Dependency:  "sql", Retryable: false, Cause: cause,
				}
			}
		}
		result, err := conn.ExecContext(ctx, `UPDATE platform_schema_migrations
			SET status = 'applied', failure_statement = 0, failure_code = '',
				finished_at = CURRENT_TIMESTAMP(6)
			WHERE namespace = ? AND version = ? AND status = 'running'`,
			namespace, migration.Version)
		if err == nil {
			err = requireSingleRowAffected(result)
		}
		if err != nil {
			return &fault.Fault{
				Class: fault.ClassInternal, Code: "platform.storage.migration_record_failed",
				SafeMessage: "migration statements completed but history update failed; inspect the database before retrying",
				Dependency:  "sql", Retryable: false, Cause: err,
			}
		}
	}
	return nil
}

func requireSingleRowAffected(result sql.Result) error {
	if result == nil {
		return errors.New("migration history update returned no result")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("could not verify migration history update: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("migration history update affected %d rows, want 1", affected)
	}
	return nil
}

func prepareMigrations(namespace string, migrations []Migration) ([]Migration, map[uint64]string, error) {
	if !validNamespace(namespace) || len(migrations) == 0 || len(migrations) > 1000 {
		return nil, nil, errors.New("namespace and migration count are invalid")
	}
	ordered := make([]Migration, len(migrations))
	copy(ordered, migrations)
	for index := range ordered {
		ordered[index].Statements = append([]string(nil), ordered[index].Statements...)
	}
	sortMigrations(ordered)

	checksums := make(map[uint64]string, len(ordered))
	for _, migration := range ordered {
		if migration.Version == 0 || strings.TrimSpace(migration.Description) == "" ||
			utf8.RuneCountInString(migration.Description) > 255 || len(migration.Statements) == 0 {
			return nil, nil, errors.New("migration version, description and statements are required")
		}
		if _, exists := checksums[migration.Version]; exists {
			return nil, nil, errors.New("migration versions must be unique")
		}
		hash := sha256.New()
		for _, statement := range migration.Statements {
			if strings.TrimSpace(statement) == "" || len(statement) > 1<<20 {
				return nil, nil, errors.New("migration statements must be non-empty and at most 1 MiB")
			}
			_, _ = hash.Write([]byte(statement))
			_, _ = hash.Write([]byte{0})
		}
		checksums[migration.Version] = hex.EncodeToString(hash.Sum(nil))
	}
	return ordered, checksums, nil
}

func sortMigrations(migrations []Migration) {
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
}

func validNamespace(namespace string) bool {
	if namespace == "" || len(namespace) > 64 || strings.TrimSpace(namespace) != namespace {
		return false
	}
	for _, char := range namespace {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func migrationLockName(namespace string) string {
	sum := sha256.Sum256([]byte(namespace))
	return "platform:migrate:" + hex.EncodeToString(sum[:16])
}
