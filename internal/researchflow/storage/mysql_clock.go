package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	platformtime "example.com/stock-analysis-flow/internal/platform/time"
	mariadbtime "example.com/stock-analysis-flow/internal/platform/time/mariadb"
	"example.com/stock-analysis-flow/internal/researchflow/application"
)

// MySQLDatabaseClock preserves the Researchflow application port while
// delegating the shared MariaDB UTC clock implementation to Platform.
type MySQLDatabaseClock struct {
	service *platformtime.Service
}

func NewMySQLDatabaseClock(db *sql.DB) (*MySQLDatabaseClock, error) {
	if db == nil {
		return nil, errors.New("Researchflow database clock requires a SQL database")
	}
	source, err := mariadbtime.NewSource(db)
	if err != nil {
		return nil, err
	}
	service, err := platformtime.NewService(source)
	if err != nil {
		return nil, err
	}
	return &MySQLDatabaseClock{service: service}, nil
}

func (c *MySQLDatabaseClock) NowUTC(ctx context.Context) (time.Time, error) {
	if c == nil || c.service == nil || ctx == nil {
		return time.Time{}, errors.New("Researchflow database clock is not configured")
	}
	return c.service.NowUTC(ctx)
}

var _ application.DatabaseClock = (*MySQLDatabaseClock)(nil)
