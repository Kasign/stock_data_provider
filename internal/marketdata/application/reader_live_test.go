package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/baostock"
	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/service"
	"github.com/go-sql-driver/mysql"
)

func TestLiveReaderGetDailyBarsV2PreservesProviderDecimalEvidence(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PROVIDER_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PROVIDER_TESTS=1 to run the real Data Application Port probe")
	}
	dsn := strings.TrimSpace(os.Getenv("MARKET_DATA_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not set")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MARKET_DATA_TEST_MYSQL_DSN: %v", err)
	}
	if parsed.DBName != "market_data_probe" && !strings.HasSuffix(strings.ToLower(parsed.DBName), "_test") {
		t.Fatalf("unsafe live-probe database %q; expected market_data_probe or a dedicated _test database", parsed.DBName)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}
	store, err := repository.NewWithBorrowedDB(db)
	if err != nil {
		t.Fatalf("create Data repository store: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply Data migrations: %v", err)
	}

	baostockProvider := provider.NewBaostock(baostock.NewClient(baostock.Config{}))
	backend := service.NewWithCapabilities(
		[]provider.NamedProvider{baostockProvider},
		nil,
		[]provider.DailyBarProvider{baostockProvider},
		cache.NewDisabled(),
		store,
		time.Minute,
		time.Minute,
		120,
		2000,
		service.ResilienceConfig{},
		slog.Default(),
	)
	backend.SetTradingCalendarProviders([]provider.TradingCalendarProvider{baostockProvider})
	reader, err := NewReader(backend)
	if err != nil {
		t.Fatalf("create Data Application Reader: %v", err)
	}

	snapshot, err := reader.GetDailyBarsV2(ctx, app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-10-30",
		Limit: 100, AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2 real BaoStock request: %v", err)
	}
	if len(snapshot.Bars) == 0 {
		t.Fatal("real BaoStock Application Port returned no daily bars")
	}
	// BaoStock 当前返回交易日列表并推断非交易日，真实来源质量为 degraded。
	// Reader 必须保留该来源质量，不得因 MariaDB 已持久化快照就把它提升为可信日历。
	if snapshot.TradingCalendarRef != nil || snapshot.QualityStatus != app.QualityDegraded ||
		snapshot.CoverageEvidence.CalendarQualityStatus != app.QualityDegraded ||
		!containsReaderString(snapshot.Warnings, "daily_bar_coverage_calendar_quality_unreliable") {
		t.Fatalf("real V2 snapshot must keep inferred BaoStock calendar degraded and untrusted: %+v", snapshot)
	}
	if snapshot.CoverageEvidence.CoverageBasis != "exchange_calendar" {
		t.Fatalf("real V2 coverage basis = %q, want original exchange_calendar source basis", snapshot.CoverageEvidence.CoverageBasis)
	}

	var sample app.DailyBar
	for _, bar := range snapshot.Bars {
		if bar.ExactValues != nil {
			sample = bar
			break
		}
	}
	if sample.ExactValues == nil {
		t.Fatalf("real Provider raw decimal evidence did not reach the Application Port: %+v", snapshot.Bars[0])
	}
	if strings.TrimSpace(sample.SourceVersion) == "" || sample.FetchedAt.IsZero() {
		t.Fatalf("real V2 bar must preserve source version and fetched_at: %+v", sample)
	}
	for name, value := range map[string]string{
		"open": sample.ExactValues.Open, "high": sample.ExactValues.High,
		"low": sample.ExactValues.Low, "close": sample.ExactValues.Close,
		"raw_volume": sample.ExactValues.RawVolume,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("real V2 exact %s is empty: %+v", name, sample.ExactValues)
		}
	}
	for name, evidence := range map[string]*app.DailyBarDecimalEvidence{
		"open": sample.ExactValues.OpenEvidence, "high": sample.ExactValues.HighEvidence,
		"low": sample.ExactValues.LowEvidence, "close": sample.ExactValues.CloseEvidence,
		"raw_volume": sample.ExactValues.RawVolumeEvidence,
	} {
		if evidence == nil || strings.TrimSpace(evidence.SourceToken) == "" ||
			evidence.EvidenceKind != string(model.DailyBarPrecisionEvidenceObservedToken) {
			t.Fatalf("real V2 %s precision evidence is missing or not an observed source token: %+v", name, evidence)
		}
	}

	report, err := json.Marshal(map[string]any{
		"provider": "baostock", "symbol": snapshot.Symbol, "interval": snapshot.Interval,
		"bar_count": len(snapshot.Bars), "quality_status": snapshot.QualityStatus,
		"calendar_ref":            snapshot.TradingCalendarRef,
		"calendar_quality_status": snapshot.CoverageEvidence.CalendarQualityStatus,
		"coverage_sufficient":     snapshot.CoverageEvidence.CoverageSufficient,
		"sample_trade_date":       sample.TradeDate,
		"exact_values":            sample.ExactValues,
		"source_version":          sample.SourceVersion,
	})
	if err != nil {
		t.Fatalf("marshal V2 evidence report: %v", err)
	}
	t.Logf("LIVE_DAILY_BARS_V2_APPLICATION_RESULT %s", report)
}
