package repository

import (
	"context"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func applyMigrationsForTest(ctx context.Context, store *Store) error {
	if store == nil || store.db == nil {
		return nil
	}
	return platformstorage.ApplyMigrations(ctx, store.db, MigrationNamespace, MigrationDefinitions())
}
