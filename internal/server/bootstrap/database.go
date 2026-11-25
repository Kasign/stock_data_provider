package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	analysisstorage "example.com/stock-analysis-flow/internal/analysis/storage"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// bootstrapDatabase is the single database initialization path shared by the
// normal Root Server and owner-init. It opens SQL only, applies and verifies
// every registered module migration, and validates final physical schemas for critical modules.
// Redis, HTTP clients and repositories must be created only after it succeeds.
func bootstrapDatabase(ctx context.Context, cfg mysqlredis.Config, timeout time.Duration) (*sql.DB, *servermigration.Registry, error) {
	if ctx == nil || timeout <= 0 {
		return nil, nil, errors.New("database bootstrap context and timeout are required")
	}
	registry, err := newMigrationRegistry()
	if err != nil {
		return nil, nil, fmt.Errorf("register module migrations: %w", err)
	}
	migrationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := serverresources.OpenSQL(migrationCtx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open MariaDB for database bootstrap: %w", err)
	}
	report, err := registry.RunWithReport(migrationCtx, db)
	if err != nil {
		slog.Error("module migrations failed", "report", report, "error", err.Error())
		return nil, nil, errors.Join(fmt.Errorf("apply module migrations: %w", err), db.Close())
	}
	slog.Info("module migrations completed", "report", report)

	if err := registry.ValidateApplied(migrationCtx, db); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("validate migration history and checksums: %w", err), db.Close())
	}
	if err := marketdatabootstrap.ValidateSchema(migrationCtx, db); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("validate final MarketData schema: %w", err), db.Close())
	}
	if err := analysisstorage.ValidateSchema(migrationCtx, db); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("validate final Analysis schema: %w", err), db.Close())
	}
	if err := researchstorage.ValidateSchema(migrationCtx, db); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("validate final Research schema: %w", err), db.Close())
	}
	if err := quantstorage.ValidateSchema(migrationCtx, db); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("validate final Quant schema: %w", err), db.Close())
	}
	if err := researchflowstorage.ValidateSchema(migrationCtx, db); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("validate final Researchflow schema: %w", err), db.Close())
	}
	return db, registry, nil
}
