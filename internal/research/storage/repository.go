package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound = errors.New("research record not found")
	ErrConflict = errors.New("research version conflict")
	ErrInvalidRepository = errors.New("research repository is not configured")
)

const pitTimeLayout = "2006-01-02T15:04:05.000000000Z"

type Clock func() time.Time

type repositoryBase struct {
	db *sql.DB
	now Clock
}

func newRepositoryBase(db *sql.DB, clock Clock) (repositoryBase, error) {
	if db == nil { return repositoryBase{}, ErrInvalidRepository }
	if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
	return repositoryBase{db: db, now: clock}, nil
}

func payloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// formatPIT always emits nine fractional digits so ascii_bin ordering is chronological.
func formatPIT(value time.Time) string {
	return value.UTC().Format(pitTimeLayout)
}

// appendVersion serializes writers on one aggregate-head row. SQL identifiers
// are fixed by typed adapters and are never derived from caller input.
func (r repositoryBase) appendVersion(ctx context.Context, aggregateType, aggregateID string,
	version int64, table, idColumn, versionColumn string, payload []byte,
	insertSQL string, insertArgs []any, validateNext func(previous []byte) error) error {
	if r.db == nil { return ErrInvalidRepository }
	if ctx == nil { return errors.New("context is nil") }
	if aggregateID == "" || utf8.RuneCountInString(aggregateID) > 255 || version < 1 || len(payload) == 0 {
		return fmt.Errorf("aggregate identity, version and payload are required")
	}
	now := r.now().UTC()
	if now.IsZero() { return errors.New("repository clock returned zero time") }
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin research append transaction: %w", err) }
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "INSERT IGNORE INTO research_aggregate_heads "+
		"(aggregate_type, aggregate_id, latest_version, created_at, updated_at) VALUES (?, ?, 0, ?, ?)",
		aggregateType, aggregateID, now, now)
	if err != nil { return fmt.Errorf("initialize research aggregate head: %w", err) }

	var latest int64
	if err := tx.QueryRowContext(ctx, "SELECT latest_version FROM research_aggregate_heads "+
		"WHERE aggregate_type=? AND aggregate_id=? FOR UPDATE", aggregateType, aggregateID).Scan(&latest); err != nil {
		return fmt.Errorf("lock research aggregate head: %w", err)
	}

	digest := payloadDigest(payload)
	if version <= latest {
		var existingDigest string
		var existingPayload []byte
		query := fmt.Sprintf("SELECT payload_sha256, payload FROM %s WHERE %s=? AND %s=?", table, idColumn, versionColumn)
		err := tx.QueryRowContext(ctx, query, aggregateID, version).Scan(&existingDigest, &existingPayload)
		if err == nil && existingDigest == digest && bytes.Equal(existingPayload, payload) { return tx.Commit() }
		if err != nil && !errors.Is(err, sql.ErrNoRows) { return fmt.Errorf("read existing research version: %w", err) }
		return fmt.Errorf("%w: %s %s version %d already has different content", ErrConflict, aggregateType, aggregateID, version)
	}
	if version != latest+1 {
		return fmt.Errorf("%w: %s %s expected version %d, got %d", ErrConflict, aggregateType, aggregateID, latest+1, version)
	}
	if latest > 0 {
		var previous []byte
		query := fmt.Sprintf("SELECT payload FROM %s WHERE %s=? AND %s=?", table, idColumn, versionColumn)
		if err := tx.QueryRowContext(ctx, query, aggregateID, latest).Scan(&previous); err != nil {
			return fmt.Errorf("read previous research version: %w", err)
		}
		if validateNext == nil { return errors.New("next-version validator is required") }
		if err := validateNext(previous); err != nil { return fmt.Errorf("validate next research version: %w", err) }
	}
	if _, err := tx.ExecContext(ctx, insertSQL, insertArgs...); err != nil {
		return fmt.Errorf("insert immutable research version: %w", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE research_aggregate_heads SET latest_version=?, updated_at=? "+
		"WHERE aggregate_type=? AND aggregate_id=? AND latest_version=?", version, now, aggregateType, aggregateID, latest)
	if err != nil { return fmt.Errorf("advance research aggregate head: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("verify research aggregate head update: %w", err) }
	if affected != 1 { return fmt.Errorf("%w: aggregate head changed during append", ErrConflict) }
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit research append transaction: %w", err) }
	return nil
}

func (r repositoryBase) getVersion(ctx context.Context, table, idColumn, versionColumn, aggregateID string, version int64) ([]byte, error) {
	if r.db == nil { return nil, ErrInvalidRepository }
	if ctx == nil { return nil, errors.New("context is nil") }
	if aggregateID == "" || version < 1 { return nil, fmt.Errorf("aggregate identity and positive version are required") }
	query := fmt.Sprintf("SELECT payload FROM %s WHERE %s=? AND %s=?", table, idColumn, versionColumn)
	var payload []byte
	if err := r.db.QueryRowContext(ctx, query, aggregateID, version).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return nil, ErrNotFound }
		return nil, fmt.Errorf("read research version: %w", err)
	}
	return payload, nil
}

func encodePayload(value any) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil { return nil, "", fmt.Errorf("encode research payload: %w", err) }
	return payload, payloadDigest(payload), nil
}
