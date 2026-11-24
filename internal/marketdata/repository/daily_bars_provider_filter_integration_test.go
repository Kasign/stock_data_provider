package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestMariaDBPITDailyBarsProviderAllowlistFiltersSourcesBeforeCoverageSelection(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := platformstorage.ApplyMigrations(ctx, store.db, MigrationNamespace, MigrationDefinitions()); err != nil {
		t.Fatalf("apply Market Data migrations: %v", err)
	}

	symbol := fmt.Sprintf("T%014d", time.Now().UnixNano()%100000000000000)
	fetchedAt := time.Now().UTC().Add(-time.Second)
	bars := make([]model.DailyBar, 0, 40)
	for index := 0; index < 20; index++ {
		tradeDate := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, index).Format("2006-01-02")
		for _, source := range []string{"baostock", "eastmoney"} {
			bars = append(bars, model.DailyBar{
				Symbol: symbol, TradeDate: tradeDate, Period: "1d",
				AdjustmentBasis: model.AdjustmentNone,
				Open:            10, High: 11, Low: 9, Close: 10.5,
				Source: source, SourceVersion: source + "-v1",
				FetchedAt: fetchedAt, NormalizedAt: fetchedAt,
				QualityStatus: model.QualityValid,
			})
		}
	}
	if err := store.UpsertBars(ctx, bars); err != nil {
		t.Fatalf("persist multi-source PIT bars: %v", err)
	}
	asOf := time.Now().UTC().Add(time.Second)
	filtered, err := store.ListBarsAsOfFromSources(ctx, symbol, "", "", model.AdjustmentNone, asOf, 20, []string{"eastmoney"})
	if err != nil {
		t.Fatalf("ListBarsAsOfFromSources() error = %v", err)
	}
	if len(filtered) != 20 {
		t.Fatalf("filtered source rows = %d, want 20", len(filtered))
	}
	for _, bar := range filtered {
		if bar.Source != "eastmoney" {
			t.Fatalf("provider allowlist leaked source %q", bar.Source)
		}
	}
	all, err := store.ListBarsAsOf(ctx, symbol, "", "", model.AdjustmentNone, asOf, 20)
	if err != nil {
		t.Fatalf("ListBarsAsOf() error = %v", err)
	}
	if len(all) != 40 {
		t.Fatalf("unfiltered PIT query rows = %d, want 40", len(all))
	}
}
