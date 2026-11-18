// Package mariadb provides the MariaDB-backed implementation of Platform time sources.
package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ClockQuery reads the MariaDB UTC clock at microsecond precision.
const ClockQuery = "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')"

const timestampLayout = "2006-01-02T15:04:05.000000000Z"

// Queryer is implemented by *sql.DB and *sql.Tx.
type Queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ReadUTC reads and parses the MariaDB UTC clock through a database handle or transaction.
func ReadUTC(ctx context.Context, queryer Queryer) (time.Time, error) {
	if ctx == nil || queryer == nil {
		return time.Time{}, errors.New("MariaDB time query and context are required")
	}
	var value string
	if err := queryer.QueryRowContext(ctx, ClockQuery).Scan(&value); err != nil {
		return time.Time{}, fmt.Errorf("read MariaDB UTC clock: %w", err)
	}
	parsed, err := time.Parse(timestampLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse MariaDB UTC clock: %w", err)
	}
	return parsed.UTC(), nil
}

// Source reads the same MariaDB clock used by persisted application data.
// Use it for cross-instance decisions that must share the database clock.
type Source struct {
	db *sql.DB
}

// NewSource creates a MariaDB-backed time source.
func NewSource(db *sql.DB) (*Source, error) {
	if db == nil {
		return nil, errors.New("MariaDB time source requires a SQL database")
	}
	return &Source{db: db}, nil
}

// NowUTC returns the current instant reported by MariaDB.
func (s *Source) NowUTC(ctx context.Context) (time.Time, error) {
	if s == nil || s.db == nil || ctx == nil {
		return time.Time{}, errors.New("MariaDB time source is not configured")
	}
	return ReadUTC(ctx, s.db)
}
