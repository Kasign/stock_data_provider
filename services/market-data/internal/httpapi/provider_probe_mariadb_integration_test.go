package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/cache"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/normalize"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/provider"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/repository"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/service"
)

type httpProbeMariaDBDailyBarProvider struct {
	name   string
	method string
}

func (p httpProbeMariaDBDailyBarProvider) Name() string { return p.name }

func (p httpProbeMariaDBDailyBarProvider) FetchDailyBars(_ context.Context, symbol, _, _ string, _ int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	volume := int64(10000)
	amount := 100000.0
	now := time.Now().UTC()
	return []model.DailyBar{{
		Symbol: symbol, TradeDate: "2026-10-29", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &volume, Amount: &amount,
		AdjustmentBasis: adjustment, AdjustmentMethod: p.method, AdjustmentRefDate: "2026-11-03",
		Source: p.name, SourceVersion: p.name + "-v1",
		ProviderTime: &now, FetchedAt: now, NormalizedAt: now,
		VolumeUnit: "shares", AmountUnit: "CNY",
		QualityStatus: model.QualityValid, NormalizationVer: normalize.Version,
	}}, nil
}

type httpProbeMariaDBTradingCalendar struct{}

func (httpProbeMariaDBTradingCalendar) Name() string { return "http-probe-calendar" }

func (p httpProbeMariaDBTradingCalendar) FetchTradingCalendar(_ context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	if startDate != endDate {
		return nil, fmt.Errorf("integration fixture expects one-day calendar range, got %s..%s", startDate, endDate)
	}
	date, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}
	if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
		return nil, fmt.Errorf("integration fixture date must be a trading weekday: %s", startDate)
	}
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: startDate, EndDate: endDate,
		TotalDays: 1, TradingDays: 1,
		Items: []model.TradingCalendarDay{{Date: startDate, IsTradingDay: true}},
		Source: p.Name(), SourceVersion: "calendar-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityValid,
	}, nil
}

func newHTTPProbeMariaDBHandler(store *repository.Store, providers ...provider.DailyBarProvider) http.Handler {
	named := make([]provider.NamedProvider, 0, len(providers))
	for _, item := range providers {
		named = append(named, item)
	}
	svc := service.NewWithCapabilities(
		named, nil, providers, cache.NewDisabled(), store,
		time.Second, time.Minute, 120, 2000, service.ResilienceConfig{}, nil,
	)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{httpProbeMariaDBTradingCalendar{}})
	return NewWithCapabilityInterfaces(
		"probe-mariadb-test-key", "none", svc,
		named, nil, providers, 2000, 110*time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Handler()
}

func TestProviderProbeHTTPMariaDBEndToEnd(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	left := httpProbeMariaDBDailyBarProvider{name: "http-probe-left", method: "eastmoney_fqt_2"}
	right := httpProbeMariaDBDailyBarProvider{name: "http-probe-right", method: "baostock_pct_change"}

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		t.Fatalf("migrate MariaDB: %v", err)
	}

	postHandler := newHTTPProbeMariaDBHandler(store, left, right)
	body := `{"scope":"all","capabilities":["daily_bars"],"checks":["cross_provider"],"symbols":["600036.SH"],"daily_bar_start_date":"2026-10-29","daily_bar_end_date":"2026-10-29","daily_bar_adjustments":["backward_adjusted"],"timeout_seconds":5}`
	postRequest := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/probe", bytes.NewBufferString(body))
	postRequest.Header.Set("X-API-Key", "probe-mariadb-test-key")
	postResponse := httptest.NewRecorder()
	postHandler.ServeHTTP(postResponse, postRequest)
	if postResponse.Code != http.StatusOK {
		store.Close()
		t.Fatalf("POST probe status=%d body=%s", postResponse.Code, postResponse.Body.String())
	}

	var created model.ProviderProbeRun
	if err := json.Unmarshal(postResponse.Body.Bytes(), &created); err != nil {
		store.Close()
		t.Fatalf("decode POST probe response: %v", err)
	}
	if created.RunID == "" || !created.Persisted {
		store.Close()
		t.Fatalf("POST probe response must include a persisted run identity: %+v", created)
	}
	assertAdjustmentEvidence := func(run model.ProviderProbeRun) {
		t.Helper()
		for _, result := range run.CheckResults {
			if result.Check != "cross_provider" || result.LeftProvider != left.name || result.RightProvider != right.name {
				continue
			}
			if result.Status != "skipped" || result.ErrorCode != "comparison_not_comparable" ||
				result.ErrorMessage != "adjustment_method_mismatch" ||
				result.LeftAdjustmentMethod != left.method || result.RightAdjustmentMethod != right.method ||
				result.LeftAdjustmentRefDate != "2026-11-03" || result.RightAdjustmentRefDate != "2026-11-03" {
				t.Fatalf("unexpected cross-provider adjustment evidence: %+v", result)
			}
			return
		}
		t.Fatalf("cross-provider adjustment result is missing: %+v", run.CheckResults)
	}
	assertAdjustmentEvidence(created)

	if err := store.Close(); err != nil {
		t.Fatalf("close MariaDB after POST probe: %v", err)
	}

	reopened, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen MariaDB before HTTP history request: %v", err)
	}
	defer reopened.Close()

	historyHandler := newHTTPProbeMariaDBHandler(reopened)
	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/data-sources/probe/history?limit=10", nil)
	getRequest.Header.Set("X-API-Key", "probe-mariadb-test-key")
	getResponse := httptest.NewRecorder()
	historyHandler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET probe history status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	var history model.ProviderProbeHistory
	if err := json.Unmarshal(getResponse.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode GET probe history response: %v", err)
	}
	for _, persisted := range history.Items {
		if persisted.RunID != created.RunID {
			continue
		}
		if !persisted.Persisted {
			t.Fatalf("history response lost persisted flag: %+v", persisted)
		}
		assertAdjustmentEvidence(persisted)
		return
	}
	t.Fatalf("GET history did not return POST run %q: %+v", created.RunID, history.Items)
}
