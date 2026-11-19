package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestMariaDBDailyBarExactValuesPersistAcrossCurrentAndPITReads(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
	if dsn == "" {
		t.Skip("Data MariaDB integration DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateCompatibilitySchema(ctx); err != nil {
		t.Fatalf("validate Data schema: %v", err)
	}

	rawAmount := "987654.321"
	fetchedAt := time.Now().UTC().Truncate(time.Microsecond)
	bar := model.DailyBar{
		Symbol: "999999.SH", TradeDate: "2026-11-18", Period: "1d",
		Open: 10.123, High: 10.456, Low: 10.001, Close: 10.321,
		Volume: dailyBarExactInt64Ptr(1234500), Amount: dailyBarExactFloat64Ptr(987654.321),
		AdjustmentBasis: model.AdjustmentNone, Source: "exact-values-integration",
		SourceVersion: "adapter-v1", FetchedAt: fetchedAt, NormalizedAt: fetchedAt,
		RawVolume: dailyBarExactFloat64Ptr(12345), RawVolumeUnit: "hands",
		RawAmount: dailyBarExactFloat64Ptr(987654.321), RawAmountUnit: "CNY",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
		NormalizationVer: "normalization-v1",
		ExactValues: &model.DailyBarExactValues{
			Open: "10.123", High: "10.456", Low: "10.001", Close: "10.321",
			RawVolume: "12345", RawAmount: &rawAmount,
		},
	}
	if err := store.UpsertBars(ctx, []model.DailyBar{bar}); err != nil {
		t.Fatalf("persist daily bar exact values: %v", err)
	}
	current, err := store.ListBars(ctx, bar.Symbol, bar.TradeDate, bar.TradeDate, model.AdjustmentNone, 10)
	if err != nil {
		t.Fatalf("read current daily bar: %v", err)
	}
	if len(current) != 1 || current[0].ExactValues == nil || current[0].ExactValues.Close != "10.321" ||
		current[0].ExactValues.RawVolume != "12345" || current[0].ExactValues.RawAmount == nil ||
		*current[0].ExactValues.RawAmount != rawAmount {
		t.Fatalf("current daily bar lost exact values: %+v", current)
	}
	asOf := time.Now().UTC().Add(time.Second)
	historical, err := store.ListBarsAsOf(ctx, bar.Symbol, bar.TradeDate, bar.TradeDate, model.AdjustmentNone, asOf, 10)
	if err != nil {
		t.Fatalf("read PIT daily bar: %v", err)
	}
	if len(historical) != 1 || historical[0].ExactValues == nil || historical[0].ExactValues.Open != "10.123" ||
		historical[0].ExactValues.Close != "10.321" {
		t.Fatalf("PIT daily bar lost exact values: %+v", historical)
	}
}

func dailyBarExactInt64Ptr(value int64) *int64 { return &value }
func dailyBarExactFloat64Ptr(value float64) *float64 { return &value }
