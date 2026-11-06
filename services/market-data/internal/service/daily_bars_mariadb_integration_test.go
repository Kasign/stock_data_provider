package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/cache"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/normalize"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/provider"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/repository"
)

func TestDailyBarsMariaDBFallbackPreservesNormalizationWarnings(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	// 600519.SH 上市前的日期窗口，避免与真实历史行情冲突；重复运行会幂等覆盖同一来源记录。
	fetchedAt := time.Now().UTC()
	volume := int64(1000)
	amount := 10000.0
	valid := model.DailyBar{
		Symbol: "600519.SH", TradeDate: "1990-01-02", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &volume, Amount: &amount, VolumeUnit: "shares", AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, Source: "integration-fallback",
		SourceVersion: "integration-fallback-v1", FetchedAt: fetchedAt, NormalizedAt: fetchedAt,
		QualityStatus: model.QualityValid, NormalizationVer: normalize.Version,
	}
	invalid := valid
	invalid.TradeDate = "1990-01-03"
	invalid.High = 8
	if err := store.UpsertBars(ctx, []model.DailyBar{valid, invalid}); err != nil {
		t.Fatalf("seed MariaDB fallback rows: %v", err)
	}

	unavailable := fakeProvider{name: "unavailable-provider", barsErr: errors.New("upstream unavailable")}
	svc := NewWithCapabilities(
		[]provider.NamedProvider{unavailable},
		nil,
		[]provider.DailyBarProvider{unavailable},
		cache.NewDisabled(), store,
		time.Second, time.Minute, 10, 100,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second},
		nil,
	)

	bars, fromCache, warnings, err := svc.DailyBars(
		ctx, "600519.SH", "1990-01-02", "1990-01-03", 10, model.AdjustmentNone,
	)
	if err != nil {
		t.Fatalf("daily bars should fall back to MariaDB: %v", err)
	}
	if fromCache {
		t.Fatal("MariaDB fallback must not be reported as a cache hit")
	}
	if len(bars) != 1 || bars[0].TradeDate != valid.TradeDate || bars[0].QualityStatus != model.QualityStale {
		t.Fatalf("fallback must return only the valid stored bar as stale: bars=%+v", bars)
	}
	hasWarning := func(expected string) bool {
		for _, warning := range warnings {
			if warning == expected {
				return true
			}
		}
		return false
	}
	for _, expected := range []string{
		"provider_unavailable_served_from_mariadb",
		"mariadb_bars_use_one_coherent_source_and_adjustment_reference",
		"invalid_ohlc_bar_dropped",
	} {
		if !hasWarning(expected) {
			t.Errorf("fallback response is missing warning %q: %v", expected, warnings)
		}
	}
}
