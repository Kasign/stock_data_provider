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
	if ctx == nil || strings.TrimSpace(options.Addr) == "" || options.DB < 0 ||
		options.PoolSize < 0 || options.PoolSize > maxSQLMaxOpenConns ||
		options.DialTimeout < 0 || options.ReadTimeout < 0 || options.WriteTimeout < 0 {
		return nil, fault.New(fault.ClassValidation, "platform.storage.invalid_redis_config", "Redis connection configuration is invalid", nil)
	}
	pingTimeout, err := normalizePingTimeout(pingTimeout)
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
	if errors.Is(cause, context.DeadlineExceeded) {
		class = fault.ClassTimeout
	}
	return &fault.Fault{
		Class: class, Code: code, SafeMessage: message,
		Dependency: dependency,
		Retryable: !errors.Is(cause, context.Canceled) &&
			(class == fault.ClassUnavailable || class == fault.ClassTimeout),
		Cause: cause,
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
		return dependencyFault("sql", "platform.storage.migration_lock_failed", "could not acquire migration lock", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return dependencyFault("sql", "platform.storage.migration_lock_busy", "another migration is running", errMigrationLockUnavailable)
	}

	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), defaultPingTimeout)
		defer releaseCancel()
		var released sql.NullInt64
		releaseErr := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
		if resultErr == nil && (releaseErr != nil || !released.Valid || released.Int64 != 1) {
			if releaseErr != nil {
				resultErr = fmt.Errorf("release migration lock: %w", releaseErr)
			} else {
				resultErr = errors.New("release migration lock did not confirm ownership")
			}
		}
	}()

	const createHistory = `CREATE TABLE IF NOT EXISTS platform_schema_migrations (
		namespace VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		version BIGINT UNSIGNED NOT NULL,
		description VARCHAR(255) NOT NULL,
		checksum CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		PRIMARY KEY (namespace, version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`
	if _, err := conn.ExecContext(ctx, createHistory); err != nil {
		return fmt.Errorf("create migration history: %w", err)
	}

	applied := make(map[uint64]string)
	rows, err := conn.QueryContext(ctx, "SELECT version, checksum FROM platform_schema_migrations WHERE namespace = ?", namespace)
	if err != nil {
		return fmt.Errorf("read migration history: %w", err)
	}
	for rows.Next() {
		var version uint64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode migration history: %w", err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate migration history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close migration history rows: %w", err)
	}

	for version := range applied {
		if _, exists := checksums[version]; !exists {
			return fmt.Errorf("%w: database contains unregistered version %d", errMigrationHistoryMismatch, version)
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
		for index, statement := range migration.Statements {
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply migration %s version %d statement %d: %w", namespace, migration.Version, index+1, err)
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO platform_schema_migrations
			(namespace, version, description, checksum) VALUES (?, ?, ?, ?)`,
			namespace, migration.Version, migration.Description, checksum); err != nil {
			return fmt.Errorf("record migration %s version %d: %w", namespace, migration.Version, err)
		}
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
