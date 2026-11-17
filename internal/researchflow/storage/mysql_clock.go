package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const researchflowDatabaseClockQuery = "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')"

// MySQLDatabaseClock reads time from the same database authority used by
// Researchflow persistence. It does not use the Server host clock.
type MySQLDatabaseClock struct {
	db *sql.DB
}

func NewMySQLDatabaseClock(db *sql.DB) (*MySQLDatabaseClock, error) {
	if db == nil {
		return nil, errors.New("Researchflow database clock requires a SQL database")
	}
	return &MySQLDatabaseClock{db: db}, nil
}

func (c *MySQLDatabaseClock) NowUTC(ctx context.Context) (time.Time, error) {
	if c == nil || c.db == nil || ctx == nil {
		return time.Time{}, errors.New("Researchflow database clock is not configured")
	}
	var value string
	if err := c.db.QueryRowContext(ctx, researchflowDatabaseClockQuery).Scan(&value); err != nil {
		return time.Time{}, fmt.Errorf("read MariaDB UTC clock: %w", err)
	}
	parsed, err := parseResearchflowTime(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse MariaDB UTC clock: %w", err)
	}
	return parsed.UTC(), nil
}

var _ application.DatabaseClock = (*MySQLDatabaseClock)(nil)
