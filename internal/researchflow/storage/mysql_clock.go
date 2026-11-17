package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	platformtime "example.com/stock-analysis-flow/internal/platform/time"
	"example.com/stock-analysis-flow/internal/researchflow/application"
)

const researchflowDatabaseClockQuery = platformtime.MariaDBClockQuery

// MySQLDatabaseClock preserves the Researchflow application port while
// delegating the shared MariaDB UTC clock implementation to Platform.
type MySQLDatabaseClock struct {
	source *platformtime.MariaDBSource
}

func NewMySQLDatabaseClock(db *sql.DB) (*MySQLDatabaseClock, error) {
	if db == nil {
		return nil, errors.New("Researchflow database clock requires a SQL database")
	}
	source, err := platformtime.NewMariaDBSource(db)
	if err != nil {
		return nil, err
	}
	return &MySQLDatabaseClock{source: source}, nil
}

func (c *MySQLDatabaseClock) NowUTC(ctx context.Context) (time.Time, error) {
	if c == nil || c.source == nil || ctx == nil {
		return time.Time{}, errors.New("Researchflow database clock is not configured")
	}
	return c.source.NowUTC(ctx)
}

var _ application.DatabaseClock = (*MySQLDatabaseClock)(nil)
