package mysqlredis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/adapters/storage"
	"example.com/stock-analysis-flow/internal/analysis/domain"
)

type Store struct {
	DB     *sql.DB
	Redis  *redis.Client
	Config Config
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	driverConfig, err := cfg.mysqlDriverConfig()
	if err != nil {
		return nil, fmt.Errorf("configure MariaDB connection: %w", err)
	}
	connector, err := mysqlDriver.NewConnector(driverConfig)
	if err != nil {
		return nil, fmt.Errorf("create MariaDB connector: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	dbPingCtx, cancelDBPing := context.WithTimeout(ctx, 5*time.Second)
	err = db.PingContext(dbPingCtx)
	cancelDBPing()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("ping MariaDB: %w", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB, DialTimeout: cfg.RedisDialTimeout, ReadTimeout: cfg.RedisReadTimeout, WriteTimeout: cfg.RedisWriteTimeout})
	redisPingCtx, cancelRedisPing := context.WithTimeout(ctx, 5*time.Second)
	err = rdb.Ping(redisPingCtx).Err()
	cancelRedisPing()
	if err != nil {
		db.Close()
		rdb.Close()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	s := &Store{DB: db, Redis: rdb, Config: cfg}
	if cfg.DBAutoMigrate {
		if err := s.Migrate(ctx); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err := s.CheckSchema(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	var errs []error
	if s.DB != nil {
		if e := s.DB.Close(); e != nil {
			errs = append(errs, e)
		}
	}
	if s.Redis != nil {
		if e := s.Redis.Close(); e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}
func (s *Store) Health(ctx context.Context) error {
	if err := s.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("MariaDB health: %w", err)
	}
	if err := s.CheckSchema(ctx); err != nil {
		return err
	}
	if err := s.Redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis health: %w", err)
	}
	return nil
}

// CheckSchema verifies the tables required by the analysis API exist in the
// currently selected database. It fails fast when migrations were skipped.
func (s *Store) CheckSchema(ctx context.Context) error {
	const requiredTables = 4
	var found int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('analysis_runs','analysis_stage_events','analysis_snapshots','analysis_tasks')`).Scan(&found)
	if err != nil {
		return fmt.Errorf("check MariaDB schema: %w", err)
	}
	if found != requiredTables {
		return fmt.Errorf("MariaDB schema is incomplete: found %d of %d required tables (set DB_AUTO_MIGRATE=true once or run the migration)", found, requiredTables)
	}
	return nil
}

// SelfTest verifies MariaDB transactional writes and Redis SET/GET/DEL. The
// MariaDB probe is rolled back; the Redis probe uses a unique, short-lived key.
func (s *Store) SelfTest(ctx context.Context) error {
	if err := s.Health(ctx); err != nil {
		return err
	}
	runID := fmt.Sprintf("storage-check-%d", time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin MariaDB write self-test: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO analysis_runs (run_id,status,task_type,as_of,started_at,request_json) VALUES (?,?,?,?,?,?)`, runID, "storage_check", "storage_check", now, now, `{}`); err != nil {
		return fmt.Errorf("MariaDB transactional write self-test: %w", err)
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM analysis_runs WHERE run_id=?`, runID).Scan(&status); err != nil {
		return fmt.Errorf("MariaDB read-after-write self-test: %w", err)
	}
	if status != "storage_check" {
		return fmt.Errorf("MariaDB read-after-write self-test returned unexpected status %q", status)
	}
	if err := tx.Rollback(); err != nil {
		return fmt.Errorf("rollback MariaDB write self-test: %w", err)
	}
	var remaining int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM analysis_runs WHERE run_id=?`, runID).Scan(&remaining); err != nil {
		return fmt.Errorf("verify MariaDB write self-test rollback: %w", err)
	}
	if remaining != 0 {
		return fmt.Errorf("MariaDB write self-test left %d temporary row(s) behind", remaining)
	}

	key := s.Config.RedisKey("self-test:" + runID)
	value := []byte("redis-round-trip-ok")
	if err := s.Redis.Set(ctx, key, value, 30*time.Second).Err(); err != nil {
		return fmt.Errorf("Redis SET self-test: %w", err)
	}
	defer s.Redis.Del(context.Background(), key)
	got, err := s.Redis.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("Redis GET self-test: %w", err)
	}
	if string(got) != string(value) {
		return fmt.Errorf("Redis GET self-test returned unexpected value %q", string(got))
	}
	if err := s.Redis.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("Redis DEL self-test: %w", err)
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS analysis_runs (run_id VARCHAR(64) PRIMARY KEY, status VARCHAR(32) NOT NULL, task_type VARCHAR(64) NOT NULL, as_of DATETIME(6) NOT NULL, started_at DATETIME(6) NOT NULL, finished_at DATETIME(6) NULL, request_json LONGTEXT NOT NULL, result_json LONGTEXT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6), KEY idx_runs_asof (as_of), KEY idx_runs_status (status)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS analysis_stage_events (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY, run_id VARCHAR(64) NOT NULL, stage VARCHAR(100) NOT NULL, status VARCHAR(32) NOT NULL, event_at DATETIME(6) NOT NULL, duration_ms BIGINT NOT NULL DEFAULT 0, event_json LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), KEY idx_events_run (run_id), KEY idx_events_run_stage (run_id, stage), CONSTRAINT fk_events_run FOREIGN KEY (run_id) REFERENCES analysis_runs(run_id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS analysis_snapshots (snapshot_id VARCHAR(96) PRIMARY KEY, run_id VARCHAR(64) NOT NULL, snapshot_type VARCHAR(32) NOT NULL, as_of DATETIME(6) NOT NULL, payload_json LONGTEXT NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), KEY idx_snapshots_run (run_id), KEY idx_snapshots_type_time (snapshot_type, as_of), CONSTRAINT fk_snapshots_run FOREIGN KEY (run_id) REFERENCES analysis_runs(run_id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS analysis_tasks (task_id VARCHAR(64) PRIMARY KEY, run_id VARCHAR(64) NOT NULL UNIQUE, status VARCHAR(24) NOT NULL, task_type VARCHAR(64) NOT NULL, request_json LONGTEXT NOT NULL, idempotency_key VARCHAR(128) NULL UNIQUE, current_stage VARCHAR(100) NULL, failure_code VARCHAR(100) NULL, error_message VARCHAR(300) NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), started_at DATETIME(6) NULL, finished_at DATETIME(6) NULL, updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6), KEY idx_tasks_status_created (status, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	for _, stmt := range statements {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate schema: %w", err)
		}
	}
	return nil
}

func (s *Store) Begin(ctx context.Context, req domain.RunRequest, started time.Time) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO analysis_runs (run_id,status,task_type,as_of,started_at,request_json) VALUES (?,?,?,?,?,?)`, req.RunID, domain.RunStatusRunning, req.TaskType, req.AsOf.UTC(), started.UTC(), string(b))
	if err != nil {
		return fmt.Errorf("persist run start: %w", err)
	}
	return nil
}
func (s *Store) AppendEvent(ctx context.Context, e domain.StageEvent) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO analysis_stage_events (run_id,stage,status,event_at,duration_ms,event_json) VALUES (?,?,?,?,?,?)`, e.RunID, e.Stage, e.Status, e.At.UTC(), e.DurationMS, string(b))
	if err != nil {
		return fmt.Errorf("persist stage event: %w", err)
	}
	return nil
}
func (s *Store) Finish(ctx context.Context, result domain.RunResult) error {
	requestJSON, err := json.Marshal(result.Request)
	if err != nil {
		return fmt.Errorf("marshal analysis request: %w", err)
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE analysis_runs SET status=?,finished_at=?,as_of=?,request_json=?,result_json=? WHERE run_id=?`, result.Status, result.FinishedAt.UTC(), result.Request.AsOf.UTC(), string(requestJSON), string(b), result.RunID)
	if err != nil {
		return fmt.Errorf("persist run result: %w", err)
	}
	if affected, rowsErr := res.RowsAffected(); rowsErr == nil && affected == 0 {
		return storage.ErrRunNotFound
	}
	if err := s.Redis.Set(ctx, s.Config.RedisKey("run:"+result.RunID), b, s.Config.RedisCacheTTL).Err(); err != nil {
		// MariaDB is the source of truth; a cache outage must not undo a durable result.
		log.Printf("WARN: cache analysis result in Redis failed run_id=%s: %v", result.RunID, err)
	}
	return nil
}
func (s *Store) SaveRaw(ctx context.Context, runID string, v domain.RawSnapshot) (string, error) {
	return s.saveSnapshot(ctx, runID, "raw", v.ID, v.AsOf, v)
}
func (s *Store) SaveMarket(ctx context.Context, runID string, v domain.MarketSnapshot) (string, error) {
	return s.saveSnapshot(ctx, runID, "market", v.ID, v.AsOf, v)
}
func (s *Store) SavePortfolio(ctx context.Context, runID string, v domain.PortfolioSnapshot) (string, error) {
	return s.saveSnapshot(ctx, runID, "portfolio", v.ID, v.AsOf, v)
}
func (s *Store) saveSnapshot(ctx context.Context, runID, kind, id string, asOf time.Time, payload any) (string, error) {
	if id == "" {
		// Stable IDs make retries for the same run/snapshot type idempotent.
		id = fmt.Sprintf("%s-%s", runID, kind)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO analysis_snapshots (snapshot_id,run_id,snapshot_type,as_of,payload_json) VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE payload_json=VALUES(payload_json),as_of=VALUES(as_of)`, id, runID, kind, asOf.UTC(), string(b))
	if err != nil {
		return "", fmt.Errorf("persist %s snapshot: %w", kind, err)
	}
	cacheKey := s.Config.RedisKey("snapshot:" + id)
	if err := s.Redis.Set(ctx, cacheKey, b, s.Config.RedisCacheTTL).Err(); err != nil {
		// Snapshot is already durable in MariaDB; caching is best effort.
		log.Printf("WARN: cache snapshot in Redis failed run_id=%s type=%s: %v", runID, kind, err)
	}
	return id, nil
}

// CacheGet and CacheSet are optional cache-aside primitives. MariaDB remains the durable source of truth.
func (s *Store) CacheGet(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := s.Redis.Get(ctx, s.Config.RedisKey("cache:"+key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}
func (s *Store) CacheSet(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = s.Config.RedisCacheTTL
	}
	return s.Redis.Set(ctx, s.Config.RedisKey("cache:"+key), value, ttl).Err()
}

// GetResult reads through Redis and falls back to MariaDB. Redis is explicitly
// treated as a cache: malformed cache entries are discarded in favor of SQL.
func (s *Store) GetResult(ctx context.Context, runID string) (domain.RunResult, error) {
	var result domain.RunResult
	if strings.TrimSpace(runID) == "" {
		return result, errors.New("run_id is required")
	}
	cacheKey := s.Config.RedisKey("run:" + runID)
	if raw, err := s.Redis.Get(ctx, cacheKey).Bytes(); err == nil {
		if json.Unmarshal(raw, &result) == nil && result.RunID == runID {
			return result, nil
		}
		_ = s.Redis.Del(ctx, cacheKey).Err()
	}
	var raw sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT result_json FROM analysis_runs WHERE run_id=?`, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, storage.ErrRunNotFound
	}
	if err != nil {
		return result, fmt.Errorf("read analysis result: %w", err)
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return result, storage.ErrRunNotFinished
	}
	if err := json.Unmarshal([]byte(raw.String), &result); err != nil {
		return result, fmt.Errorf("decode persisted analysis result: %w", err)
	}
	if err := s.Redis.Set(ctx, cacheKey, raw.String, s.Config.RedisCacheTTL).Err(); err != nil {
		log.Printf("WARN: refresh Redis result cache failed run_id=%s: %v", runID, err)
	}
	return result, nil
}

// GetEvents returns the append-only audit events in their database insertion order.
func (s *Store) GetEvents(ctx context.Context, runID string) ([]domain.StageEvent, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, errors.New("run_id is required")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT event_json FROM analysis_stage_events WHERE run_id=? ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("query analysis events: %w", err)
	}
	defer rows.Close()
	out := make([]domain.StageEvent, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan analysis event: %w", err)
		}
		var event domain.StageEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("decode analysis event: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analysis events: %w", err)
	}
	if len(out) == 0 {
		var exists int
		err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM analysis_runs WHERE run_id=?`, runID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.ErrRunNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("check analysis run for events: %w", err)
		}
	}
	return out, nil
}
