package time

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	stdtime "time"
)

const MariaDBClockQuery = "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')"

const mariaDBTimestampLayout = "2006-01-02T15:04:05.000000000Z"

// MariaDBSource uses the same MariaDB authority as persisted application data.
// Use it for cross-instance decisions that must share the database clock.
type MariaDBSource struct {
	db *sql.DB
}

func NewMariaDBSource(db *sql.DB) (*MariaDBSource, error) {
	if db == nil {
		return nil, errors.New("MariaDB time source requires a SQL database")
	}
	return &MariaDBSource{db: db}, nil
}

func (s *MariaDBSource) NowUTC(ctx context.Context) (stdtime.Time, error) {
	if s == nil || s.db == nil || ctx == nil {
		return stdtime.Time{}, errors.New("MariaDB time source is not configured")
	}
	var value string
	if err := s.db.QueryRowContext(ctx, MariaDBClockQuery).Scan(&value); err != nil {
		return stdtime.Time{}, fmt.Errorf("read MariaDB UTC clock: %w", err)
	}
	parsed, err := stdtime.Parse(mariaDBTimestampLayout, value)
	if err != nil {
		return stdtime.Time{}, fmt.Errorf("parse MariaDB UTC clock: %w", err)
	}
	return parsed.UTC(), nil
}
