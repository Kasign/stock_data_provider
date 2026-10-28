package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type fakeProvider struct {
	name              string
	quote             *model.Quote
	bars              []model.DailyBar
	quoteErr, barsErr error
	seenEnd           *string
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) FetchQuote(context.Context, string) (*model.Quote, error) {
	return f.quote, f.quoteErr
}
func (f fakeProvider) FetchDailyBars(_ context.Context, _ string, _ string, end string, _ int, _ model.AdjustmentBasis) ([]model.DailyBar, error) {
	if f.seenEnd != nil {
		*f.seenEnd = end
	}
	return f.bars, f.barsErr
}

type transientProvider struct {
	name                   string
	quote                  *model.Quote
	quoteFailuresRemaining int
	quoteCalls             int
}

func (p *transientProvider) Name() string { return p.name }
func (p *transientProvider) FetchQuote(context.Context, string) (*model.Quote, error) {
	p.quoteCalls++
	if p.quoteFailuresRemaining > 0 {
		p.quoteFailuresRemaining--
		return nil, errors.New("temporary provider error")
	}
	return p.quote, nil
}
func (p *transientProvider) FetchDailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, error) {
	return nil, errors.New("not used by test")
}

func TestHistoricalMarketDateNormalizersRejectFutureDates(t *testing.T) {
	futureDate := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := normalizeLimitPoolDate(futureDate); err == nil || !strings.Contains(err.Error(), "must not be in the future") {
		t.Fatalf("future limit-pool date must be rejected, got %v", err)
	}
	if _, err := normalizeDragonTigerDate(futureDate); err == nil || !strings.Contains(err.Error(), "must not be in the future") {
		t.Fatalf("future dragon-tiger date must be rejected, got %v", err)
	}
}

func validFundamentalsSnapshotForTest() *model.FundamentalsSnapshot {
	now := time.Now().UTC()
	categories := []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"}
	sections := make([]model.FundamentalSection, 0, len(categories))
	for _, category := range categories {
		sections = append(sections, model.FundamentalSection{
			Category:        category,
			HasData:         true,
			ReportDate:      "2026-06-30",
			PublicationDate: "2026-08-29",
			Metrics:         []model.FundamentalMetric{{Key: "test_metric", Name: "测试指标", Value: 1, Unit: "ratio", SourceField: "testField"}},
			QualityStatus:   model.QualityValid,
		})
	}
	return &model.FundamentalsSnapshot{
		Symbol: "600519.SH", Year: 2026, Quarter: 2, HasData: true,
		Sections: sections, Source: "baostock", SourceVersion: "fixture-fundamentals-v1", FetchedAt: now, QualityStatus: model.QualityValid,
	}
}

func TestValidateFundamentalsSnapshotRejectsDuplicateMetricKeys(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	snapshot.Sections[0].Metrics = append(snapshot.Sections[0].Metrics, snapshot.Sections[0].Metrics[0])
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "duplicate metric key") {
		t.Fatalf("duplicate metric keys within a section must be rejected, got %v", err)
	}
}

func TestValidateFundamentalsSnapshotRejectsBlankMetricMetadata(t *testing.T) {
	cases := []struct {
		name string
		mutate func(*model.FundamentalMetric)
	}{
		{name: "blank key", mutate: func(metric *model.FundamentalMetric) { metric.Key = "  " }},
		{name: "blank name", mutate: func(metric *model.FundamentalMetric) { metric.Name = "  " }},
		{name: "blank unit", mutate: func(metric *model.FundamentalMetric) { metric.Unit = "  " }},
		{name: "blank source field", mutate: func(metric *model.FundamentalMetric) { metric.SourceField = "  " }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validFundamentalsSnapshotForTest()
			tc.mutate(&snapshot.Sections[0].Metrics[0])
			if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "invalid metric metadata/value") {
				t.Fatalf("blank metric metadata must be rejected, got %v", err)
			}
		})
	}
}

func TestValidateFundamentalsSnapshotRequiresSourceVersion(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	snapshot.SourceVersion = ""
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("fundamentals snapshot without source version must be rejected, got %v", err)
	}
}

func TestValidateFundamentalsSnapshotRejectsFuturePublicationDate(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	snapshot.Sections[0].PublicationDate = time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "publication date is in the future") {
		t.Fatalf("future publication date must be rejected, got %v", err)
	}
}

func TestValidateFundamentalsSnapshotRejectsValidSectionWithoutPublicationDate(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	snapshot.Sections[0].PublicationDate = ""
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid without a publication date") {
		t.Fatalf("valid fundamentals without a publication date must be rejected, got %v", err)
	}
}

func TestValidateFundamentalsSnapshotAcceptsPointInTimeValidSnapshot(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err != nil {
		t.Fatalf("valid published fundamentals snapshot should pass: %v", err)
	}
}


func TestValidateFundamentalsSnapshotAcceptsOptionalFinancialStatements(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	snapshot.QualityStatus = model.QualityDegraded
	for _, category := range []string{"income_statement", "balance_sheet", "cash_flow_statement"} {
		snapshot.Sections = append(snapshot.Sections, model.FundamentalSection{
			Category: category, ReportDate: fundamentalPeriodEnd(snapshot.Year, snapshot.Quarter),
			PublicationDate: "2026-08-29", HasData: true,
			Metrics: []model.FundamentalMetric{{
				Key: "statement_metric", Name: "报表指标", Value: 1, Unit: "CNY", SourceField: "statement_metric",
			}},
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"statement_units_or_coverage_not_fully_verified"},
		})
	}
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err != nil {
		t.Fatalf("six standard groups plus three statement groups should be accepted as degraded: %v", err)
	}
}

func TestValidateFundamentalsSnapshotStillRequiresAllSixIndicatorGroups(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	for index, section := range snapshot.Sections {
		if section.Category == "dupont" {
			snapshot.Sections = append(snapshot.Sections[:index], snapshot.Sections[index+1:]...)
			break
		}
	}
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "expected six indicator groups") {
		t.Fatalf("snapshot without all six standard groups must be rejected, got %v", err)
	}
}

func TestFundamentalsRejectsFutureReportPeriodBeforeCacheOrProviderLookup(t *testing.T) {
	now := time.Now().In(marketLocation())
	today := now.Format("2006-01-02")
	futureQuarter := 0
	for quarter := 1; quarter <= 4; quarter++ {
		if fundamentalPeriodEnd(now.Year(), quarter) > today {
			futureQuarter = quarter
			break
		}
	}
	if futureQuarter == 0 {
		t.Skip("no future report quarter remains in the current calendar year")
	}

	// 空服务用于确认门禁优先于缓存和 Provider 访问。
	svc := &DataService{}
	_, _, err := svc.Fundamentals(context.Background(), "600519.SH", now.Year(), futureQuarter)
	if err == nil || !strings.Contains(err.Error(), "fundamental report period must not be in the future") {
		t.Fatalf("future fundamentals period must be rejected before dependencies are accessed, got %v", err)
	}
}

func valuationFloatPointerForValidationTest(value float64) *float64 { return &value }

func validValuationSnapshotForValidationTest() *model.ValuationSnapshot {
	now := time.Now().UTC()
	symbol := "600519.SH"
	item := model.ValuationItem{
		Symbol: symbol, Code: "600519",
		PETTM: valuationFloatPointerForValidationTest(18.4), PEMRQ: valuationFloatPointerForValidationTest(6.1), PEStatic: valuationFloatPointerForValidationTest(22.5),
		PBMRQ: valuationFloatPointerForValidationTest(6.1), PSTTM: valuationFloatPointerForValidationTest(7.2), PCFTTM: valuationFloatPointerForValidationTest(9.0),
		Source: "tushare", SourceVersion: "tushare-daily-basic-v2", FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	return &model.ValuationSnapshot{
		RequestedSymbols: []string{symbol}, Total: 1, Items: []model.ValuationItem{item},
		ProviderReportDate: time.Now().In(marketLocation()).Format("2006-01-02"),
		Source:             "tushare", SourceVersion: "tushare-daily-basic-v2", FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
}

func TestValidateValuationSnapshotRejectsFutureReportDateAndTimestamp(t *testing.T) {
	t.Run("future report date", func(t *testing.T) {
		snapshot := validValuationSnapshotForValidationTest()
		snapshot.ProviderReportDate = time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
		if err := validateValuationSnapshot(snapshot.RequestedSymbols, snapshot); err == nil || !strings.Contains(err.Error(), "provider_report_date") {
			t.Fatalf("future valuation report date must be rejected, got %v", err)
		}
	})
	t.Run("future provider timestamp", func(t *testing.T) {
		snapshot := validValuationSnapshotForValidationTest()
		future := time.Now().UTC().Add(5 * time.Minute)
		snapshot.ProviderTimestamp = &future
		if err := validateValuationSnapshot(snapshot.RequestedSymbols, snapshot); err == nil || !strings.Contains(err.Error(), "provider_timestamp is in the future") {
			t.Fatalf("future valuation timestamp must be rejected, got %v", err)
		}
	})
	t.Run("missing report date cannot be valid", func(t *testing.T) {
		snapshot := validValuationSnapshotForValidationTest()
		snapshot.ProviderReportDate = ""
		snapshot.QualityStatus = model.QualityValid
		snapshot.ProviderTimestamp = nil
		if err := validateValuationSnapshot(snapshot.RequestedSymbols, snapshot); err == nil || !strings.Contains(err.Error(), "claims valid quality") {
			t.Fatalf("missing report date/time must not be marked valid, got %v", err)
		}
	})
}

func validQuote(source string) *model.Quote {
	now := time.Now().UTC()
	p := 1313.31
	change := -18.57
	pct := -1.394
	open := 1331.88
	high := 1331.98
	low := 1312.0
	prev := 1331.88
	vol := int64(3413600)
	amount := 4525398959.0
	qt := now
	return &model.Quote{Symbol: "600519.SH", Name: "贵州茅台", CurrentPrice: &p, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &vol, Amount: &amount, QuoteTime: &qt, FetchedAt: now, NormalizedAt: now, Source: source, SourceVersion: source + "-quote-v1", VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
}
func TestQuoteFallsBackFromOldNormalizationVersion(t *testing.T) {
	old := validQuote("eastmoney")
	old.NormalizationVer = "old-normalization-v1"
	current := validQuote("tencent")
	providers := []provider.Provider{
		fakeProvider{name: "eastmoney", quote: old},
		fakeProvider{name: "tencent", quote: current},
	}
	svc := New(providers, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, hit, err := svc.Quote(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("Quote returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tencent" || got.NormalizationVer != normalize.Version {
		t.Fatalf("old-normalization quote must fall back to current version: %+v", got)
	}
}

func TestQuoteFallsBackWhenProviderSourceVersionIsMissing(t *testing.T) {
	missingVersion := validQuote("eastmoney")
	missingVersion.SourceVersion = ""
	fallback := validQuote("tencent")
	providers := []provider.Provider{
		fakeProvider{name: "eastmoney", quote: missingVersion},
		fakeProvider{name: "tencent", quote: fallback},
	}
	svc := New(providers, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, hit, err := svc.Quote(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("Quote returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tencent" || got.SourceVersion != fallback.SourceVersion || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("quote without source version must fall back to a versioned provider: %+v", got)
	}
}

func TestQuotePreservesPriceWhenAmountMissingAndMarksUnitUnknown(t *testing.T) {
	q := validQuote("eastmoney")
	q.Amount = nil
	q.AmountUnit = "CNY" // emulate a provider that hard-codes unit metadata with no value
	p := fakeProvider{name: "eastmoney", quote: q}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, _, err := s.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("missing turnover should not invalidate otherwise valid quote: %v", err)
	}
	if got.CurrentPrice == nil || got.Volume == nil || got.Amount != nil {
		t.Fatalf("price/share volume should remain usable and absent amount should remain null: %+v", got)
	}
	if got.AmountUnit != "unknown" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing amount must be explicit in quality contract: %+v", got)
	}
}

func TestQuoteRetriesTransientProviderFailureBeforeAcceptingResponse(t *testing.T) {
	p := &transientProvider{name: "eastmoney", quote: validQuote("eastmoney"), quoteFailuresRemaining: 1}
	s := NewConfiguredWithResilience([]provider.Provider{p}, []provider.Provider{p}, []provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		ResilienceConfig{Retries: 1, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q, _, err := s.Quote(context.Background(), "600519")
	if err != nil {
		t.Fatal(err)
	}
	if p.quoteCalls != 2 {
		t.Fatalf("expected one retry (2 calls), got %d", p.quoteCalls)
	}
	if q.Source != "eastmoney" {
		t.Fatalf("unexpected quote source %q", q.Source)
	}
}

func TestQuoteCircuitBreakerSkipsProviderAfterRepeatedFailure(t *testing.T) {
	p := &transientProvider{name: "eastmoney", quoteFailuresRemaining: 100}
	s := NewConfiguredWithResilience([]provider.Provider{p}, []provider.Provider{p}, []provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 1, CircuitCooldown: time.Minute}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, _, err := s.Quote(context.Background(), "600519"); err == nil {
		t.Fatal("first provider failure should return an error")
	}
	callsAfterFailure := p.quoteCalls
	if _, _, err := s.Quote(context.Background(), "600519"); err == nil {
		t.Fatal("open circuit with no fallback should fail explicitly")
	}
	if p.quoteCalls != callsAfterFailure {
		t.Fatalf("open circuit should skip provider call: before=%d after=%d", callsAfterFailure, p.quoteCalls)
	}
}

func TestQuoteFailoverRecordsActualSource(t *testing.T) {
	primary := fakeProvider{name: "eastmoney", quoteErr: errors.New("timeout")}
	fallback := fakeProvider{name: "tencent", quote: validQuote("tencent")}
	s := New([]provider.Provider{primary, fallback}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q, _, err := s.Quote(context.Background(), "600519")
	if err != nil {
		t.Fatal(err)
	}
	if q.Source != "tencent" || q.FallbackFrom != "eastmoney" || q.QualityStatus != model.QualityDegraded {
		t.Fatalf("fallback metadata missing: %+v", q)
	}
}
func TestHistoricalBundleDoesNotFetchRealtimeQuote(t *testing.T) {
	p := fakeProvider{name: "eastmoney", quoteErr: errors.New("quote should not be called")}
	v := int64(1000)
	amount := float64(10500)
	p.bars = []model.DailyBar{{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &v, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 60, slog.New(slog.NewTextHandler(io.Discard, nil)))
	asOf, _ := time.Parse(time.RFC3339, "2026-10-20T08:00:00Z")
	b, err := s.Bundle(context.Background(), "600519", 60, asOf, true, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if b.Quote != nil {
		t.Fatal("historical bundle must not include latest quote")
	}
	if len(b.Warnings) == 0 {
		t.Fatal("expected historical quote warning")
	}
}

func TestHistoricalBundleCutsOffCurrentIncompleteDayAndWarnsAboutAdjustmentLookahead(t *testing.T) {
	loc := marketLocation()
	// Monday before market close: the previous calendar day is Sunday, but
	// Friday's completed candle must still be present in the resulting bundle.
	asOf := time.Date(2026, 10, 19, 13, 0, 0, 0, loc)
	seenEnd := ""
	vol := int64(1000)
	amount := float64(10500)
	p := fakeProvider{name: "eastmoney", seenEnd: &seenEnd, bars: []model.DailyBar{
		{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-16", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
		{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-19", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
	}}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 60, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b, err := s.Bundle(context.Background(), "600519", 60, asOf, true, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if seenEnd != "2026-10-19" {
		t.Fatalf("expected provider query through cutoff date, got %s", seenEnd)
	}
	if len(b.DailyBars) != 1 || b.DailyBars[0].TradeDate != "2026-10-16" {
		t.Fatalf("expected Friday completed bar only, got %+v", b.DailyBars)
	}
	foundAdjustmentWarning, foundCutoffWarning := false, false
	for _, warning := range b.Warnings {
		if warning == "historical_adjusted_bars_may_use_current_corporate_action_factors; point_in_time_adjustment_not_verified" {
			foundAdjustmentWarning = true
		}
		if warning == "as_of_date_daily_bar_excluded_before_market_close" {
			foundCutoffWarning = true
		}
	}
	if !foundAdjustmentWarning || !foundCutoffWarning || b.QualityStatus != model.QualityDegraded {
		t.Fatalf("historical adjusted prices and cutoff must disclose uncertainty: %+v", b)
	}
	if b.Quote != nil {
		t.Fatal("historical bundle must not contain the latest real-time quote")
	}
}

func TestQuoteFreshnessRejectsStaleAndFutureTimestamps(t *testing.T) {
	loc := marketLocation()
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, loc) // Monday during the A-share morning session.
	stale := now.Add(-4 * time.Minute).UTC()
	q := validQuote("eastmoney")
	q.QuoteTime = &stale
	applyQuoteFreshness(q, now)
	if q.QualityStatus != model.QualityStale {
		t.Fatalf("old quote must be stale during session, got %s", q.QualityStatus)
	}
	future := now.Add(2 * time.Minute).UTC()
	q = validQuote("eastmoney")
	q.QuoteTime = &future
	applyQuoteFreshness(q, now)
	if q.QualityStatus != model.QualityDegraded {
		t.Fatalf("future provider timestamp must be degraded, got %s", q.QualityStatus)
	}
}

func TestQuoteFreshnessChecksFutureTimestampsOutsideTradingHours(t *testing.T) {
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, marketLocation()) // Saturday.
	future := now.Add(2 * time.Minute).UTC()
	q := validQuote("eastmoney")
	q.QuoteTime = &future
	applyQuoteFreshness(q, now)
	if q.QualityStatus != model.QualityDegraded {
		t.Fatalf("future quote timestamps must be degraded off-session too, got %s", q.QualityStatus)
	}
	old := now.Add(-25 * time.Hour).UTC()
	q = validQuote("eastmoney")
	q.QuoteTime = &old
	applyQuoteFreshness(q, now)
	if q.QualityStatus != model.QualityStale {
		t.Fatalf("quotes older than 24h outside session should be stale, got %s", q.QualityStatus)
	}
}

func TestBundleRejectsAnyFutureAsOf(t *testing.T) {
	s := New([]provider.Provider{fakeProvider{name: "eastmoney"}}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := s.Bundle(context.Background(), "600519", 60, time.Now().Add(5*time.Second), true, model.AdjustmentForward)
	if err == nil {
		t.Fatal("future as_of must be rejected to prevent temporal leakage")
	}
}

type memoryCache struct{ values map[string][]byte }

func newMemoryCache() *memoryCache { return &memoryCache{values: map[string][]byte{}} }
func (m *memoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m.values[key]
	return v, ok, nil
}
func (m *memoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.values[key] = append([]byte(nil), value...)
	return nil
}
func (m *memoryCache) Ping(context.Context) error { return nil }

func TestQuoteDoesNotReturnStaleCacheWithoutAttemptingRefresh(t *testing.T) {
	mem := newMemoryCache()
	stale := validQuote("eastmoney")
	old := time.Now().Add(-25 * time.Hour).UTC()
	stale.QuoteTime = &old
	payload, _ := json.Marshal(stale)
	mem.values["md:v4:quote:600519.SH"] = payload
	fresh := validQuote("tencent")
	s := New([]provider.Provider{fakeProvider{name: "tencent", quote: fresh}}, mem, nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, cacheHit, err := s.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if cacheHit || got.Source != "tencent" || got.QualityStatus != model.QualityValid {
		t.Fatalf("expected stale cache to be bypassed for a fresh provider quote: hit=%v quote=%+v", cacheHit, got)
	}
}

func TestQuotePrefersFreshProviderOverStalePrimary(t *testing.T) {
	stale := validQuote("eastmoney")
	old := time.Now().Add(-25 * time.Hour).UTC()
	stale.QuoteTime = &old
	fresh := validQuote("tencent")
	s := New([]provider.Provider{
		fakeProvider{name: "eastmoney", quote: stale},
		fakeProvider{name: "tencent", quote: fresh},
	}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, cacheHit, err := s.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if cacheHit || got.Source != "tencent" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("stale primary should be bypassed and fresh fallback annotated: hit=%v quote=%+v", cacheHit, got)
	}
	if !containsReason(got.QualityReasons, "provider_fallback_used") {
		t.Fatalf("fallback source must be auditable: %+v", got.QualityReasons)
	}
}

func TestQuoteReturnsExplicitlyStaleCandidateOnlyWhenNoProviderIsFresh(t *testing.T) {
	stale := validQuote("eastmoney")
	old := time.Now().Add(-25 * time.Hour).UTC()
	stale.QuoteTime = &old
	s := New([]provider.Provider{fakeProvider{name: "eastmoney", quote: stale}}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, cacheHit, err := s.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if cacheHit || got.QualityStatus != model.QualityStale || !containsReason(got.QualityReasons, "no_provider_returned_a_fresh_timestamped_quote") {
		t.Fatalf("fallback quote must remain explicitly stale when no fresh provider exists: hit=%v quote=%+v", cacheHit, got)
	}
}

func containsReason(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestQuoteMaps504OnlyWhenEveryProviderTimesOut(t *testing.T) {
	primary := fakeProvider{name: "eastmoney", quoteErr: context.DeadlineExceeded}
	empty := fakeProvider{name: "tencent"}
	s := New([]provider.Provider{primary, empty}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, _, err := s.Quote(context.Background(), "600519.SH")
	if err == nil {
		t.Fatal("expected all providers to fail")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("mixed timeout and empty response must not be misclassified as all-provider timeout: %v", err)
	}
}

func TestDailyBarsRejectsCacheWithWrongAdjustmentBasis(t *testing.T) {
	mem := newMemoryCache()
	vol := int64(10000)
	amt := float64(100000)
	cached := []model.DailyBar{{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amt, AdjustmentBasis: model.AdjustmentBackward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}
	payload, _ := json.Marshal(map[string]any{"bars": cached, "warnings": []string{}})
	mem.values["md:v4:bars:600519.SH:::forward_adjusted:120"] = payload
	providerVol := int64(20000)
	providerAmt := float64(200000)
	p := fakeProvider{name: "eastmoney", bars: []model.DailyBar{{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &providerVol, Amount: &providerAmt, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}}
	s := New([]provider.Provider{p}, mem, nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bars, hit, _, err := s.DailyBars(context.Background(), "600519.SH", "", "", 120, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("wrong-basis cache entry must not be returned")
	}
	if len(bars) != 1 || *bars[0].Volume != providerVol {
		t.Fatalf("expected provider data, got %+v", bars)
	}
}

func dailyBarForVersionContractTest(sourceVersion, tradeDate string) model.DailyBar {
	volume := int64(10000)
	amount := 100000.0
	return model.DailyBar{
		Symbol: "600519.SH", TradeDate: tradeDate, Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount,
		AdjustmentBasis: model.AdjustmentForward, Source: "fixture-provider", SourceVersion: sourceVersion,
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityDegraded,
	}
}

func TestDailyBarsFallsBackFromOldNormalizationVersion(t *testing.T) {
	old := dailyBarForVersionContractTest("eastmoney-daily-bars-v1", "2026-10-20")
	old.NormalizationVer = "old-normalization-v1"
	current := dailyBarForVersionContractTest("tencent-daily-bars-v1", "2026-10-20")
	providers := []provider.Provider{
		fakeProvider{name: "eastmoney", bars: []model.DailyBar{old}},
		fakeProvider{name: "tencent", bars: []model.DailyBar{current}},
	}
	svc := New(providers, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))

	bars, hit, _, err := svc.DailyBars(context.Background(), "600519.SH", "", "", 120, model.AdjustmentForward)
	if err != nil || hit {
		t.Fatalf("DailyBars returned hit=%v err=%v", hit, err)
	}
	if len(bars) != 1 || bars[0].Source != "tencent" || bars[0].NormalizationVer != normalize.Version {
		t.Fatalf("old-normalization daily bars must fall back to current version: %+v", bars)
	}
}

func TestDailyBarsFallsBackWhenProviderSourceVersionIsMissing(t *testing.T) {
	primaryBar := dailyBarForVersionContractTest("placeholder", "2026-10-20")
	primaryBar.SourceVersion = ""
	fallbackBar := dailyBarForVersionContractTest("tencent-daily-bars-v1", "2026-10-20")
	primary := fakeProvider{name: "eastmoney", bars: []model.DailyBar{primaryBar}}
	fallback := fakeProvider{name: "tencent", bars: []model.DailyBar{fallbackBar}}
	svc := New([]provider.Provider{primary, fallback}, cache.NewDisabled(), nil, time.Second, time.Minute, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	bars, hit, _, err := svc.DailyBars(context.Background(), "600519.SH", "2026-10-20", "2026-10-20", 1, model.AdjustmentForward)
	if err != nil || hit {
		t.Fatalf("DailyBars returned hit=%v err=%v", hit, err)
	}
	if len(bars) != 1 || bars[0].Source != "tencent" || bars[0].SourceVersion != "tencent-daily-bars-v1" {
		t.Fatalf("daily bars without source version must trigger fallback: %+v", bars)
	}
}

func TestDailyBarsRejectsMixedProviderSourceVersionsAsAWhole(t *testing.T) {
	primary := fakeProvider{name: "eastmoney", bars: []model.DailyBar{
		dailyBarForVersionContractTest("eastmoney-daily-bars-v1", "2026-10-20"),
		dailyBarForVersionContractTest("eastmoney-daily-bars-v2", "2026-10-21"),
	}}
	fallbackVersion := "tencent-daily-bars-v1"
	fallback := fakeProvider{name: "tencent", bars: []model.DailyBar{
		dailyBarForVersionContractTest(fallbackVersion, "2026-10-20"),
		dailyBarForVersionContractTest(fallbackVersion, "2026-10-21"),
	}}
	svc := New([]provider.Provider{primary, fallback}, cache.NewDisabled(), nil, time.Second, time.Minute, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	bars, hit, _, err := svc.DailyBars(context.Background(), "600519.SH", "2026-10-20", "2026-10-21", 2, model.AdjustmentForward)
	if err != nil || hit {
		t.Fatalf("DailyBars returned hit=%v err=%v", hit, err)
	}
	if len(bars) != 2 || bars[0].Source != "tencent" || bars[1].Source != "tencent" ||
		bars[0].SourceVersion != fallbackVersion || bars[1].SourceVersion != fallbackVersion {
		t.Fatalf("mixed-version response must be discarded as a whole: %+v", bars)
	}
}

func TestDailyBarsCacheWithoutSourceVersionIsBypassed(t *testing.T) {
	mem := newMemoryCache()
	legacy := dailyBarForVersionContractTest("", "2026-10-20")
	legacy.FetchedAt = time.Now().UTC()
	legacy.NormalizedAt = time.Now().UTC()
	legacy.NormalizationVer = normalize.Version
	payload, err := json.Marshal(map[string]any{"bars": []model.DailyBar{legacy}, "warnings": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:bars:600519.SH:2026-10-20:2026-10-20:forward_adjusted:1"] = payload
	fresh := dailyBarForVersionContractTest("tencent-daily-bars-v1", "2026-10-20")
	svc := New([]provider.Provider{fakeProvider{name: "tencent", bars: []model.DailyBar{fresh}}}, mem, nil,
		time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))

	bars, hit, _, err := svc.DailyBars(context.Background(), "600519.SH", "2026-10-20", "2026-10-20", 1, model.AdjustmentForward)
	if err != nil || hit {
		t.Fatalf("daily-bar cache without source version must be bypassed: hit=%v err=%v", hit, err)
	}
	if len(bars) != 1 || bars[0].Source != "tencent" || bars[0].SourceVersion != fresh.SourceVersion {
		t.Fatalf("expected versioned Provider response after cache bypass: %+v", bars)
	}
}

func TestCoherentStoredSeriesUsesStableTieBreakers(t *testing.T) {
	oldFetch := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	newFetch := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	makeSeries := func(source, version string, fetched time.Time) []model.DailyBar {
		return []model.DailyBar{
			{Source: source, SourceVersion: version, TradeDate: "2026-10-20", FetchedAt: fetched, AdjustmentBasis: model.AdjustmentNone, NormalizationVer: normalize.Version},
			{Source: source, SourceVersion: version, TradeDate: "2026-10-21", FetchedAt: fetched, AdjustmentBasis: model.AdjustmentNone, NormalizationVer: normalize.Version},
		}
	}
	older := makeSeries("eastmoney", "eastmoney-bars-v1", oldFetch)
	newer := makeSeries("tencent", "tencent-bars-v1", newFetch)

	for _, bars := range [][]model.DailyBar{
		append(append([]model.DailyBar(nil), older...), newer...),
		append(append([]model.DailyBar(nil), newer...), older...),
	} {
		got, reason := coherentStoredSeries(bars, model.AdjustmentNone)
		source := ""
		if len(got) > 0 {
			source = got[0].Source
		}
		if reason != "" || len(got) != 2 || source != "tencent" {
			t.Fatalf("equal-coverage groups must prefer latest fetched series regardless of input order: source=%q bars=%+v reason=%s", source, got, reason)
		}
	}

	// 同一采集时刻和覆盖度仍打平时，按分组 key 字典序稳定决胜。
	equalFetchTencent := makeSeries("tencent", "tencent-bars-v1", newFetch)
	equalFetchEastmoney := makeSeries("eastmoney", "eastmoney-bars-v1", newFetch)
	got, reason := coherentStoredSeries(append(equalFetchTencent, equalFetchEastmoney...), model.AdjustmentNone)
	source := ""
	if len(got) > 0 {
		source = got[0].Source
	}
	if reason != "" || len(got) != 2 || source != "eastmoney" {
		t.Fatalf("fully tied groups must use deterministic lexical key ordering: source=%q bars=%+v reason=%s", source, got, reason)
	}
}

func TestCoherentStoredSeriesDoesNotMixSourceVersions(t *testing.T) {
	first := model.DailyBar{
		Source: "eastmoney", SourceVersion: "eastmoney-bars-v1", TradeDate: "2026-10-20",
		AdjustmentBasis: model.AdjustmentNone, NormalizationVer: normalize.Version,
	}
	second := first
	second.TradeDate = "2026-10-21"
	second.SourceVersion = "eastmoney-bars-v2"
	series, reason := coherentStoredSeries([]model.DailyBar{first, second}, model.AdjustmentNone)
	if reason != "" || len(series) != 1 {
		t.Fatalf("stored daily bars must be partitioned by source version: bars=%+v reason=%s", series, reason)
	}
}

func TestDailyBarsRejectsProviderAdjustmentMismatchAndUsesFallback(t *testing.T) {
	wrongVol, rightVol := int64(10000), int64(20000)
	amount := float64(100000)
	wrong := fakeProvider{name: "eastmoney", bars: []model.DailyBar{{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &wrongVol, Amount: &amount, AdjustmentBasis: model.AdjustmentNone, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}}
	right := fakeProvider{name: "tencent", bars: []model.DailyBar{{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &rightVol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "tencent", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}}
	s := New([]provider.Provider{wrong, right}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bars, hit, _, err := s.DailyBars(context.Background(), "600519.SH", "", "", 120, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if hit || len(bars) != 1 || bars[0].Source != "tencent" || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("wrong-basis source should fail over and be marked degraded: hit=%t bars=%+v", hit, bars)
	}
}

func TestDailyBarsRejectsStartAfterEnd(t *testing.T) {
	s := New(nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, nil)
	_, _, _, err := s.DailyBars(context.Background(), "600519", "2026-10-21", "2026-10-20", 20, model.AdjustmentForward)
	if err == nil {
		t.Fatal("start date after end date must be rejected")
	}
}

func TestQuoteCacheWithoutSourceVersionIsBypassed(t *testing.T) {
	mem := newMemoryCache()
	legacy := validQuote("eastmoney")
	legacy.SourceVersion = ""
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:quote:600519.SH"] = payload
	fresh := validQuote("tencent")
	svc := New([]provider.Provider{fakeProvider{name: "tencent", quote: fresh}}, mem, nil, time.Second, time.Second, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, hit, err := svc.Quote(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("quote cache without source version must be bypassed: hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tencent" {
		t.Fatalf("expected fresh versioned quote after cache bypass, got %+v", got)
	}
}

func TestQuoteCacheReturnsCanonicalSymbol(t *testing.T) {
	mem := newMemoryCache()
	q := validQuote("eastmoney")
	q.Symbol = "600519" // Older cache payload lacked an explicit exchange suffix.
	payload, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:quote:600519.SH"] = payload
	s := New(nil, mem, nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, hit, err := s.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if !hit || got.Symbol != "600519.SH" {
		t.Fatalf("cache quote must return canonical symbol: hit=%t quote=%+v", hit, got)
	}
}

func TestDailyBarsRejectsMixedAdjustmentResponseAsAWhole(t *testing.T) {
	vol := int64(10000)
	amount := float64(100000)
	valid := model.DailyBar{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}
	wrong := valid
	wrong.TradeDate = "2026-10-19"
	wrong.AdjustmentBasis = model.AdjustmentNone
	primary := fakeProvider{name: "eastmoney", bars: []model.DailyBar{valid, wrong}}
	fallbackBar := valid
	fallbackBar.Source = "tencent"
	fallback := fakeProvider{name: "tencent", bars: []model.DailyBar{fallbackBar}}
	s := New([]provider.Provider{primary, fallback}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bars, _, _, err := s.DailyBars(context.Background(), "600519", "", "", 120, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Source != "tencent" || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("mixed-basis response must be discarded in favor of fallback: %+v", bars)
	}
}

func TestNearLiveAsOfDoesNotIncludeQuoteTimestampAfterCutoff(t *testing.T) {
	asOf := time.Now().Add(-20 * time.Second).UTC()
	q := validQuote("eastmoney")
	providerTime := asOf.Add(10 * time.Second) // after cutoff but still not in the future relative to wall clock
	q.QuoteTime = &providerTime
	vol := int64(1000)
	amount := float64(10500)
	p := fakeProvider{name: "eastmoney", quote: q, bars: []model.DailyBar{{
			SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
	}}}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 60, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b, err := s.Bundle(context.Background(), "600519", 60, asOf, true, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if b.Quote != nil {
		t.Fatal("quote newer than explicit as_of must not enter the historical bundle")
	}
	found := false
	for _, warning := range b.Warnings {
		if warning == "realtime_quote_excluded_after_or_unknown_as_of_timestamp" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected explicit timestamp cutoff warning, got %+v", b.Warnings)
	}
}

func TestQuoteCacheWithOldNormalizationVersionIsBypassed(t *testing.T) {
	mem := newMemoryCache()
	old := validQuote("eastmoney")
	old.NormalizationVer = "1"
	payload, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:quote:600519.SH"] = payload
	fresh := validQuote("tencent")
	s := New([]provider.Provider{fakeProvider{name: "tencent", quote: fresh}}, mem, nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, hit, err := s.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if hit || got.Source != "tencent" || got.NormalizationVer != normalize.Version {
		t.Fatalf("old normalization cache must be refreshed: hit=%t quote=%+v", hit, got)
	}
}

func TestDailyBarsCacheWithOldNormalizationVersionIsBypassed(t *testing.T) {
	mem := newMemoryCache()
	volume := int64(1000)
	amount := float64(10500)
	old := model.DailyBar{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid, NormalizationVer: "1"}
	payload, err := json.Marshal(map[string]any{"bars": []model.DailyBar{old}, "warnings": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:bars:600519.SH:::forward_adjusted:120"] = payload
	providerBar := old
	providerBar.Source = "tencent"
	providerBar.NormalizationVer = normalize.Version
	p := fakeProvider{name: "tencent", bars: []model.DailyBar{providerBar}}
	s := New([]provider.Provider{p}, mem, nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bars, hit, _, err := s.DailyBars(context.Background(), "600519.SH", "", "", 120, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if hit || len(bars) != 1 || bars[0].Source != "tencent" || bars[0].NormalizationVer != normalize.Version {
		t.Fatalf("old normalization bars must be refreshed: hit=%t bars=%+v", hit, bars)
	}
}

func TestCurrentDailyBarRemainsPartialDuringLunchAndPostClosePublicationWindow(t *testing.T) {
	date := "2026-10-21"
	cases := []struct {
		name string
		time string
		want bool
	}{
		{name: "before open", time: "09:00", want: true},
		{name: "morning session", time: "10:30", want: true},
		{name: "lunch break", time: "11:45", want: true},
		{name: "afternoon session", time: "14:00", want: true},
		{name: "post close publication grace", time: "15:03", want: true},
		{name: "completed candle", time: "15:06", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := time.ParseInLocation("2006-01-02 15:04", date+" "+tc.time, marketLocation())
			if err != nil {
				t.Fatal(err)
			}
			if got := isCurrentIncompleteSessionBar(date, parsed); got != tc.want {
				t.Fatalf("isCurrentIncompleteSessionBar(%s, %s)=%v, want %v", date, tc.time, got, tc.want)
			}
		})
	}
}

func TestDailyBarsRejectsExplicitFutureEndBeforeCallingProvider(t *testing.T) {
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	future := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	seenEnd := ""
	p := fakeProvider{name: "eastmoney", seenEnd: &seenEnd}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, _, _, err := s.DailyBars(context.Background(), "600519", today, future, 120, model.AdjustmentForward)
	if err == nil || !strings.Contains(err.Error(), "must not be after today") {
		t.Fatalf("expected explicit future end date rejection, got %v", err)
	}
	if seenEnd != "" {
		t.Fatalf("provider must not be called for future end date; saw end=%q", seenEnd)
	}
}

func TestDailyBarsBoundsImplicitEndDateToTodayAndDropsFutureRows(t *testing.T) {
	currentDate := time.Now().In(marketLocation()).Format("2006-01-02")
	futureDate := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	seenEnd := ""
	volume := int64(1000)
	amount := 10500.0
	p := fakeProvider{name: "eastmoney", seenEnd: &seenEnd, bars: []model.DailyBar{
		{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: currentDate, Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
		{SourceVersion: "test-v1", Symbol: "600519.SH", TradeDate: futureDate, Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
	}}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bars, _, warnings, err := s.DailyBars(context.Background(), "600519", "", "", 120, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if seenEnd != currentDate {
		t.Fatalf("implicit end date must be bounded to current China-market date: got %q want %q", seenEnd, currentDate)
	}
	if len(bars) != 1 || bars[0].TradeDate != currentDate {
		t.Fatalf("future daily bars must not be accepted: %+v", bars)
	}
	if !containsTestWarning(warnings, "bar_after_end_date_dropped") {
		t.Fatalf("expected future-bar rejection warning, got %v", warnings)
	}
}

func containsTestWarning(warnings []string, target string) bool {
	for _, warning := range warnings {
		if warning == target {
			return true
		}
	}
	return false
}

func TestPreferPartialBarsChoosesNewestSeriesWhenCountsTie(t *testing.T) {
	current := []model.DailyBar{
		{SourceVersion: "test-v1", TradeDate: "2026-10-20"},
		{SourceVersion: "test-v1", TradeDate: "2026-10-21"},
	}
	newer := []model.DailyBar{
		{SourceVersion: "test-v1", TradeDate: "2026-10-21"},
		{SourceVersion: "test-v1", TradeDate: "2026-10-22"},
	}
	longerButOlder := []model.DailyBar{
		{SourceVersion: "test-v1", TradeDate: "2026-10-18"},
		{SourceVersion: "test-v1", TradeDate: "2026-10-19"},
		{SourceVersion: "test-v1", TradeDate: "2026-10-20"},
	}

	if !preferPartialBars(newer, current) {
		t.Fatal("equal-size candidate with a newer trade date should replace the current partial series")
	}
	if preferPartialBars(current, newer) {
		t.Fatal("older equal-size candidate must not replace the newer partial series")
	}
	if !preferPartialBars(longerButOlder, newer) {
		t.Fatal("a longer validated partial series should win even when its newest date is older")
	}
	if preferPartialBars(current, current) {
		t.Fatal("equal-size series with equal newest date should preserve the current provider-priority candidate")
	}
}

func TestCoverageSufficientRejectsSparseLookback(t *testing.T) {
	if coverageSufficient("", "2026-10-21", 120, 12) {
		t.Fatal("sparse lookback should trigger fallback")
	}
	if coverageSufficient("", "2026-10-21", 120, 60) {
		t.Fatal("half coverage must be treated as truncated and trigger fallback")
	}
	if !coverageSufficient("", "2026-10-21", 120, 96) {
		t.Fatal("80-percent lookback coverage should pass")
	}
	if coverageSufficient("2026-01-01", "2026-03-31", 120, 12) {
		t.Fatal("a very sparse explicit date range must be retained as a fallback candidate")
	}
	if !coverageSufficient("2026-01-01", "2026-01-05", 120, 2) {
		t.Fatal("a small date range with plausible weekday coverage should pass")
	}
	if coverageSufficient("0001-01-01", "2026-10-21", 2000, 1000) {
		t.Fatal("bounded long historical ranges with only half the requested bars must trigger fallback")
	}
	if !coverageSufficient("0001-01-01", "2026-10-21", 2000, 1400) {
		t.Fatal("long historical date ranges should use bounded coverage calculation")
	}
}

func TestDailyBarsCachePartialCandleRefreshesAfterClose(t *testing.T) {
	loc := marketLocation()
	bar := model.DailyBar{SourceVersion: "test-v1", TradeDate: "2026-10-21", IsPartial: true}
	before := time.Date(2026, 10, 21, 15, 3, 0, 0, loc)
	after := time.Date(2026, 10, 21, 15, 6, 0, 0, loc)
	if dailyBarsCacheNeedsPostCloseRefresh([]model.DailyBar{bar}, before) {
		t.Fatal("must retain cache during publication grace")
	}
	if !dailyBarsCacheNeedsPostCloseRefresh([]model.DailyBar{bar}, after) {
		t.Fatal("must refresh cached partial bar after publication grace")
	}
}

func TestCoherentStoredSeriesNeverMixesProvidersAndRejectsUnknownAdjustedReference(t *testing.T) {
	a := model.DailyBar{SourceVersion: "test-v1", Source: "eastmoney", TradeDate: "2026-10-20", AdjustmentBasis: model.AdjustmentNone, NormalizationVer: normalize.Version}
	b := a
	b.Source = "tencent"
	b.TradeDate = "2026-10-21"
	series, reason := coherentStoredSeries([]model.DailyBar{a, b}, model.AdjustmentNone)
	if reason != "" || len(series) != 1 || series[0].Source == "" {
		t.Fatalf("should return a single source series, got %+v reason=%s", series, reason)
	}
	_, reason = coherentStoredSeries([]model.DailyBar{{SourceVersion: "test-v1", Source: "eastmoney", TradeDate: "2026-10-20", AdjustmentBasis: model.AdjustmentForward, NormalizationVer: normalize.Version}}, model.AdjustmentForward)
	if reason == "" {
		t.Fatal("adjusted series with unknown reference date must be rejected")
	}
}

func TestCallerCancellationDoesNotTripQuoteCircuitBreaker(t *testing.T) {
	p := fakeProvider{name: "eastmoney"}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := s.Quote(ctx, "600519.SH")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected caller cancellation, got %v", err)
	}
	state := s.breaker.states["eastmoney:quote"]
	if state != nil && state.failures != 0 {
		t.Fatalf("caller cancellation must not count as provider failure: %+v", state)
	}
}

func TestCallerCancellationDoesNotTripDailyBarCircuitBreaker(t *testing.T) {
	p := fakeProvider{name: "eastmoney"}
	s := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := s.DailyBars(ctx, "600519.SH", "", "", 120, model.AdjustmentForward)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected caller cancellation, got %v", err)
	}
	state := s.breaker.states["eastmoney:daily_bars"]
	if state != nil && state.failures != 0 {
		t.Fatalf("caller cancellation must not count as provider failure: %+v", state)
	}
}

func TestDailyBarsFallbackWarningsExposeMissingAmount(t *testing.T) {
	volume := int64(3351400)
	primary := fakeProvider{name: "eastmoney", barsErr: errors.New("temporary EOF")}
	fallback := fakeProvider{
		name: "tencent",
		bars: []model.DailyBar{{
			SourceVersion: "test-v1",
			Symbol: "600519.SH", TradeDate: "2026-10-21", Period: "1d",
			Open: 1331.88, High: 1331.98, Low: 1302.36, Close: 1309.69,
			Volume: &volume, Amount: nil, AdjustmentBasis: model.AdjustmentForward,
			Source: "tencent", VolumeUnit: "shares", AmountUnit: "unknown",
			QualityStatus:  model.QualityDegraded,
			QualityReasons: []string{"tencent_daily_kline_has_no_verified_amount_field"},
		}},
	}
	s := NewConfiguredWithResilience(
		[]provider.Provider{primary, fallback},
		[]provider.Provider{primary, fallback},
		[]provider.Provider{primary, fallback},
		cache.NewDisabled(), nil, time.Second, time.Second, 1, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	bars, cacheHit, warnings, err := s.DailyBars(
		context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 1, model.AdjustmentForward,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cacheHit || len(bars) != 1 || bars[0].Source != "tencent" {
		t.Fatalf("expected the Tencent fallback candle, hit=%v bars=%+v", cacheHit, bars)
	}
	if bars[0].Amount != nil || bars[0].AmountUnit != "unknown" || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("missing turnover must remain explicit and degraded, got %+v", bars[0])
	}
	foundFallback, foundAmountMissing := false, false
	for _, warning := range warnings {
		foundFallback = foundFallback || warning == "daily_bar_provider_fallback_used"
		foundAmountMissing = foundAmountMissing || warning == "daily_bar_amount_missing_from_selected_provider"
	}
	if !foundFallback || !foundAmountMissing {
		t.Fatalf("expected response-level fallback and missing-amount warnings, got %v", warnings)
	}
}

func TestProviderFailureSummaryRedactsURLQuery(t *testing.T) {
	err := errors.New(`eastmoney request: Get "https://push2his.eastmoney.com/api/qt/stock/kline/get?secid=1.600036&ut=public-token": EOF`)
	kind, message := providerFailureSummary(err)
	if kind != "connection_closed" {
		t.Fatalf("error kind = %q, want connection_closed", kind)
	}
	if strings.Contains(message, "public-token") || strings.Contains(message, "secid=1.600036") {
		t.Fatalf("provider error message leaked URL query parameters: %q", message)
	}
	if !strings.Contains(message, "https://push2his.eastmoney.com/api/qt/stock/kline/get?[query redacted]") {
		t.Fatalf("expected endpoint path to remain visible with query redacted, got %q", message)
	}
}

func TestProviderFailureSummaryClassifiesQuoteContractFailures(t *testing.T) {
	for _, message := range []string{
		"quote change is inconsistent with current_price minus prev_close",
		"Tencent quote volume unit cannot be confirmed; refusing to assume 100-share lots",
		"quote OHLC invariant failed",
	} {
		kind, _ := providerFailureSummary(errors.New(message))
		if kind != "data_contract" {
			t.Errorf("provider failure %q classified as %q, want data_contract", message, kind)
		}
	}
}

func TestProviderFailureSummaryClassifiesNetworkUnavailable(t *testing.T) {
	kind, message := providerFailureSummary(errors.New("dial tcp 61.129.129.199:443: connect: Network unreachable"))
	if kind != "network_connect" {
		t.Fatalf("error kind = %q, want network_connect", kind)
	}
	if !strings.Contains(message, "Network unreachable") {
		t.Fatalf("expected sanitized failure message to preserve network symptom, got %q", message)
	}
}

type stockBoardsFakeProvider struct {
	snapshot *model.StockBoardSnapshot
	err      error
}

func (f stockBoardsFakeProvider) Name() string { return "eastmoney" }
func (f stockBoardsFakeProvider) FetchStockBoards(context.Context, string) (*model.StockBoardSnapshot, error) {
	return f.snapshot, f.err
}

func TestStockBoardsNormalizesProviderAndCachedSymbols(t *testing.T) {
	change := 1.25
	makeSnapshot := func(symbol string) *model.StockBoardSnapshot {
		now := time.Now().UTC()
		return &model.StockBoardSnapshot{
			Symbol: symbol, Total: 1, HasData: true,
			Items: []model.StockBoard{{
				Code: "BK0477", Name: "酿酒行业", ChangePercent: &change,
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			}},
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
		}
	}
	t.Run("provider response", func(t *testing.T) {
		p := stockBoardsFakeProvider{snapshot: makeSnapshot("600519")}
		svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
			ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
		svc.SetStockBoardsProviders([]provider.StockBoardsProvider{p})
		got, hit, err := svc.StockBoards(context.Background(), "600519.SH")
		if err != nil || hit {
			t.Fatalf("StockBoards returned hit=%v err=%v", hit, err)
		}
		if got == nil || got.Symbol != "600519.SH" {
			t.Fatalf("provider board symbol must be returned canonically, got %+v", got)
		}
	})
	t.Run("cache payload", func(t *testing.T) {
		legacy := makeSnapshot("600519")
		encoded, err := json.Marshal(legacy)
		if err != nil { t.Fatal(err) }
		mem := newMemoryCache()
		mem.values["md:v1:stock-boards:600519.SH"] = encoded
		svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
		got, hit, err := svc.StockBoards(context.Background(), "600519.SH")
		if err != nil || !hit {
			t.Fatalf("StockBoards cache returned hit=%v err=%v", hit, err)
		}
		if got == nil || got.Symbol != "600519.SH" {
			t.Fatalf("cached board symbol must be returned canonically, got %+v", got)
		}
	})
}

func TestStockBoardsValidatesIdentityAndReturnsSnapshot(t *testing.T) {
	now := time.Now().UTC()
	change := 1.25
	snapshot := &model.StockBoardSnapshot{
		Symbol: "600519.SH", Total: 1, HasData: true,
		Items:  []model.StockBoard{{Code: "BK0477", Name: "酿酒行业", ChangePercent: &change, Source: "eastmoney", SourceVersion: "test-stock-boards-v1", FetchedAt: now, QualityStatus: model.QualityDegraded}},
		Source: "eastmoney", SourceVersion: "test-stock-boards-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
	p := stockBoardsFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBoardsProviders([]provider.StockBoardsProvider{p})
	got, hit, err := svc.StockBoards(context.Background(), "600519")
	if err != nil || hit {
		t.Fatalf("StockBoards returned hit=%v err=%v", hit, err)
	}
	if got.Symbol != "600519.SH" || !got.HasData || got.Total != 1 || got.Items[0].Code != "BK0477" {
		t.Fatalf("unexpected stock boards result: %+v", got)
	}
	versionMismatch := *snapshot
	versionMismatch.Items = append([]model.StockBoard(nil), snapshot.Items...)
	versionMismatch.Items[0].SourceVersion = "older-version"
	if err := validateStockBoardsSnapshot("600519.SH", &versionMismatch); err == nil || !strings.Contains(err.Error(), "source/version") {
		t.Fatalf("mismatched stock-board item version must be rejected, got %v", err)
	}
}

func TestStockBoardsRejectsProviderSymbolMismatch(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &model.StockBoardSnapshot{Symbol: "000001.SZ", Total: 1, HasData: true, Items: []model.StockBoard{{Code: "BK1", Name: "测试", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded}}, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded}
	p := stockBoardsFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBoardsProviders([]provider.StockBoardsProvider{p})
	_, _, err := svc.StockBoards(context.Background(), "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "all stock-boards providers failed") {
		t.Fatalf("expected stock-board identity mismatch, got %v", err)
	}
}

func TestQuoteCacheRejectsLegacyUnverifiedTencentVolume(t *testing.T) {
	now := time.Now().UTC()
	for _, reason := range []string{
		"volume_unit_assumed_without_turnover_crosscheck",
		"volume_unit_unverified",
	} {
		q := &model.Quote{
			QualityStatus:  model.QualityDegraded,
			QuoteTime:      &now,
			QualityReasons: []string{reason},
		}
		if quoteCacheEligible(q) {
			t.Fatalf("quote cache must reject an unverified Tencent volume reason %q", reason)
		}
	}
}


func TestValidateMarketBreadthRequiresSourceVersion(t *testing.T) {
	amount := 100.0
	snapshot := &model.MarketBreadthSnapshot{
		Market: "cn_a_share", TotalSecurities: 3, CoveredSecurities: 3,
		AdvancingCount: 1, DecliningCount: 1, UnchangedCount: 1, AmountCoverageCount: 3,
		TotalAmountCNY: &amount, ProviderReportDate: time.Now().In(marketLocation()).Format("2006-01-02"),
		Source: "eastmoney", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
	}
	if err := validateMarketBreadth(snapshot); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("market-breadth snapshot without source version must be rejected, got %v", err)
	}
}

func TestValidateMarketBreadthRequiresDegradedQualityForPartialAmountCoverage(t *testing.T) {
	snapshot := &model.MarketBreadthSnapshot{
		Market:              "cn_a_share",
		TotalSecurities:     3,
		CoveredSecurities:   3,
		AdvancingCount:      1,
		DecliningCount:      1,
		UnchangedCount:      1,
		AmountCoverageCount: 2,
		ProviderReportDate:  "2026-10-23",
		Source:              "eastmoney",
		SourceVersion:       "test-v1",
		FetchedAt:           time.Now().UTC(),
		QualityStatus:       model.QualityValid,
	}
	if err := validateMarketBreadth(snapshot); err == nil || !strings.Contains(err.Error(), "cannot have valid quality") {
		t.Fatalf("partial amount coverage must not be labelled valid, got %v", err)
	}

	snapshot.QualityStatus = model.QualityDegraded
	if err := validateMarketBreadth(snapshot); err != nil {
		t.Fatalf("partial amount coverage with degraded quality should remain usable: %v", err)
	}
}


func TestValidateIndexQuoteSnapshotRejectsInvalidPricesAndQualityMismatch(t *testing.T) {
	now := time.Now().UTC()
	changePercent := 1.0
	for _, tc := range []struct {
		name  string
		price float64
	}{
		{name: "zero", price: 0},
		{name: "negative", price: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := &model.IndexQuoteSnapshot{
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
				Items: []model.MarketIndexQuote{{
					Symbol: "000001.SH", Code: "000001", Name: "上证指数",
					LatestPrice: &tc.price, ChangePercent: &changePercent,
					AmountUnit: "unknown", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
					QualityStatus: model.QualityValid,
				}},
			}
			if err := validateIndexQuoteSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "latest price") {
				t.Fatalf("non-positive index price must be rejected, got %v", err)
			}
		})
	}

	price := 3200.0
	snapshot := &model.IndexQuoteSnapshot{
		Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
		Items: []model.MarketIndexQuote{{
			Symbol: "000001.SH", Code: "000001", Name: "上证指数",
			LatestPrice: &price, ChangePercent: &changePercent,
			AmountUnit: "unknown", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}},
	}
	if err := validateIndexQuoteSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
		t.Fatalf("snapshot must inherit item degradation, got %v", err)
	}
}


func TestIndexQuoteRejectsInvalidSymbolOrCode(t *testing.T) {
	now := time.Now().UTC()
	price, changePercent := 3200.0, 1.0
	cases := []struct {
		name, symbol, code string
	}{
		{name: "missing exchange suffix", symbol: "000001", code: "000001"},
		{name: "symbol code mismatch", symbol: "000001.SH", code: "999999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := &model.IndexQuoteSnapshot{
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
				Items: []model.MarketIndexQuote{{
					Symbol: tc.symbol, Code: tc.code, Name: "测试指数",
					LatestPrice: &price, ChangePercent: &changePercent, AmountUnit: "unknown",
					Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
				}},
			}
			if err := validateIndexQuoteSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "invalid symbol/code") {
				t.Fatalf("invalid index symbol/code must be rejected, got %v", err)
			}
		})
	}
}

func TestIndexQuoteRejectsMismatchedItemSourceVersion(t *testing.T) {
	now := time.Now().UTC()
	price, changePercent := 3200.0, 1.0
	snapshot := &model.IndexQuoteSnapshot{
		Source: "eastmoney", SourceVersion: "current-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
		Items: []model.MarketIndexQuote{{
			Symbol: "000001.SH", Code: "000001", Name: "上证指数",
			LatestPrice: &price, ChangePercent: &changePercent, AmountUnit: "unknown",
			Source: "eastmoney", SourceVersion: "old-v0", FetchedAt: now, QualityStatus: model.QualityDegraded,
		}},
	}
	if err := validateIndexQuoteSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "source/version") {
		t.Fatalf("mismatched index quote version must be rejected, got %v", err)
	}
}

func TestValidateLimitPoolRejectsInvalidNumericFieldsAndQualityMismatch(t *testing.T) {
	now := time.Now().UTC()
	change := 2.5
	makeSnapshot := func() *model.LimitPoolSnapshot {
		return &model.LimitPoolSnapshot{
			PoolType: "up", TradeDate: "2026-10-23", Total: 1, HasData: true,
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			Items: []model.LimitPoolItem{{
				Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", ChangePercent: &change,
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			}},
		}
	}

	t.Run("source version mismatch", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.Items[0].SourceVersion = "older-version"
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "source/version") {
			t.Fatalf("mismatched limit-pool item version must be rejected, got %v", err)
		}
	})
	 t.Run("symbol code mismatch", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.Items[0].Code = "000001"
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "symbol/code mismatch") {
			t.Fatalf("limit-pool symbol/code mismatch must be rejected, got %v", err)
		}
	})
	t.Run("non-finite amount", func(t *testing.T) {
		snapshot := makeSnapshot()
		bad := math.NaN()
		snapshot.Items[0].SealMoney = &bad
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "non-finite") {
			t.Fatalf("non-finite amount must be rejected, got %v", err)
		}
	})
	t.Run("negative count", func(t *testing.T) {
		snapshot := makeSnapshot()
		bad := -1
		snapshot.Items[0].ConsecutiveBoards = &bad
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "negative count") {
			t.Fatalf("negative count must be rejected, got %v", err)
		}
	})
	t.Run("aggregate quality", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.QualityStatus = model.QualityValid
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("snapshot quality must include item degradation, got %v", err)
		}
	})

	t.Run("future provider report date", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.ProviderReportDate = time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "report date is in the future") {
			t.Fatalf("future provider report date must be rejected, got %v", err)
		}
	})
	t.Run("provider report date mismatch", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.ProviderReportDate = "2026-10-22"
		if err := validateLimitPoolSnapshot("up", "2026-10-23", 30, snapshot); err == nil || !strings.Contains(err.Error(), "does not match requested trade date") {
			t.Fatalf("mismatched provider report date must be rejected to permit source fallback, got %v", err)
		}
	})
}


func TestMarkLimitPoolTruncatedDowngradesIncompleteSnapshot(t *testing.T) {
	now := time.Now().UTC()
	change := 1.5
	snapshot := &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: "2026-10-23", ProviderReportDate: "2026-10-23",
		Total: 2, HasData: true, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityValid,
		Items: []model.LimitPoolItem{{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", ChangePercent: &change,
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
		}},
	}
	markLimitPoolTruncated(snapshot)
	if snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("incomplete limit pool must be degraded, got %q", snapshot.QualityStatus)
	}
	found := false
	for _, reason := range snapshot.QualityReasons {
		if reason == "provider_limit_pool_response_truncated" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("truncation reason missing: %+v", snapshot.QualityReasons)
	}
	markLimitPoolTruncated(snapshot)
	count := 0
	for _, reason := range snapshot.QualityReasons {
		if reason == "provider_limit_pool_response_truncated" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("truncation reason must not be duplicated: %+v", snapshot.QualityReasons)
	}
}


func TestValidateSectorRankingSnapshotRejectsInvalidTotalsAndQuality(t *testing.T) {
	now := time.Now().UTC()
	changePercent := 1.25
	makeSnapshot := func() *model.SectorRankingSnapshot {
		return &model.SectorRankingSnapshot{
			Category: "industry", Total: 1, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
			Items: []model.SectorRanking{{
				Rank: 1, Code: "BK0477", Name: "酿酒行业", Category: "industry",
				ChangePercent: &changePercent, AmountUnit: "unknown", Source: "eastmoney", SourceVersion: "test-v1",
				FetchedAt: now, QualityStatus: model.QualityDegraded,
			}},
		}
	}

	t.Run("source version mismatch", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.Items[0].SourceVersion = "older-version"
		if err := validateSectorRankingSnapshot("industry", 10, snapshot); err == nil || !strings.Contains(err.Error(), "source/version") {
			t.Fatalf("mismatched sector-ranking item version must be rejected, got %v", err)
		}
	})
	t.Run("total below returned count", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.Total = 0
		if err := validateSectorRankingSnapshot("industry", 10, snapshot); err == nil || !strings.Contains(err.Error(), "total") {
			t.Fatalf("invalid total must be rejected, got %v", err)
		}
	})
	t.Run("non-finite change percent", func(t *testing.T) {
		snapshot := makeSnapshot()
		bad := math.NaN()
		snapshot.Items[0].ChangePercent = &bad
		if err := validateSectorRankingSnapshot("industry", 10, snapshot); err == nil || !strings.Contains(err.Error(), "non-finite") {
			t.Fatalf("non-finite change percent must be rejected, got %v", err)
		}
	})
	t.Run("aggregate quality", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.QualityStatus = model.QualityValid
		if err := validateSectorRankingSnapshot("industry", 10, snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("snapshot quality must include item degradation, got %v", err)
		}
	})
}


func TestSnapshotQualityMustReflectDegradedItemsAndNonAtomicBreadth(t *testing.T) {
	now := time.Now().UTC()
	t.Run("non-atomic market breadth", func(t *testing.T) {
		amount := 100.0
		snapshot := &model.MarketBreadthSnapshot{
			Market: "cn_a_share", TotalSecurities: 3, CoveredSecurities: 3,
			AdvancingCount: 1, DecliningCount: 1, UnchangedCount: 1,
			TotalAmountCNY: &amount, AmountCoverageCount: 3,
			ProviderReportDate: "2026-10-23", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityValid,
			QualityReasons: []string{"market_amount_quotes_collected_non_atomically"},
		}
		if err := validateMarketBreadth(snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("non-atomic breadth must not be marked valid, got %v", err)
		}
	})
	t.Run("stock board item degradation", func(t *testing.T) {
		change := 1.2
		snapshot := &model.StockBoardSnapshot{
			Symbol: "600519.SH", Total: 1, HasData: true, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityValid,
			Items: []model.StockBoard{{Code: "BK0477", Name: "酿酒行业", ChangePercent: &change,
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded}},
		}
		if err := validateStockBoardsSnapshot("600519.SH", snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("stock-board snapshot must inherit item degradation, got %v", err)
		}
	})
	t.Run("hot ranking item degradation", func(t *testing.T) {
		snapshot := &model.HotRankingSnapshot{
			RankingType: "popular", Total: 1, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
			Items: []model.HotRankingItem{{Rank: 1, Symbol: "600519.SH", Code: "600519",
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded}},
		}
		if err := validateHotRankingSnapshot("popular", snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("hot-ranking snapshot must inherit item degradation, got %v", err)
		}
	})
}


func TestSnapshotSourceTimesRejectMissingOrFutureValues(t *testing.T) {
	now := time.Now().UTC()
	t.Run("future index quote time", func(t *testing.T) {
		price, changePercent := 3200.0, 1.0
		future := now.Add(10 * time.Minute)
		snapshot := &model.IndexQuoteSnapshot{
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			Items: []model.MarketIndexQuote{{
				Symbol: "000001.SH", Code: "000001", Name: "上证指数",
				LatestPrice: &price, ChangePercent: &changePercent,
				QuoteTime: &future, QuoteTimeSemantics: "provider_snapshot_time",
				AmountUnit: "unknown", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
				QualityStatus: model.QualityDegraded,
			}},
		}
		if err := validateIndexQuoteSnapshot(snapshot); err == nil || !strings.Contains(err.Error(), "future provider quote timestamp") {
			t.Fatalf("future index quote time must be rejected, got %v", err)
		}
	})
	t.Run("hot ranking without provider time", func(t *testing.T) {
		snapshot := &model.HotRankingSnapshot{
			RankingType: "popular", Total: 1, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
			Items: []model.HotRankingItem{{Rank: 1, Symbol: "600519.SH", Code: "600519",
				Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid}},
	}
		if err := validateHotRankingSnapshot("popular", snapshot); err == nil || !strings.Contains(err.Error(), "without provider time") {
			t.Fatalf("hot ranking without provider time must not be valid, got %v", err)
		}
	})
}


type stockBoardsCountingFakeProvider struct {
	snapshot *model.StockBoardSnapshot
	calls    int
}

func (f *stockBoardsCountingFakeProvider) Name() string { return "eastmoney" }

func (f *stockBoardsCountingFakeProvider) FetchStockBoards(context.Context, string) (*model.StockBoardSnapshot, error) {
	f.calls++
	return f.snapshot, nil
}

func TestStockBoardsCachesUnknownEmptyResultBriefly(t *testing.T) {
	now := time.Now().UTC()
	p := &stockBoardsCountingFakeProvider{
		snapshot: &model.StockBoardSnapshot{
			Symbol: "600519.SH", Total: 0, HasData: false, Items: []model.StockBoard{},
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityUnknown,
			QualityReasons: []string{"provider_empty_membership_unconfirmed"},
		},
	}
	mem := newMemoryCache()
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBoardsProviders([]provider.StockBoardsProvider{p})

	first, hit, err := svc.StockBoards(context.Background(), "600519")
	if err != nil || hit {
		t.Fatalf("first StockBoards request hit=%v err=%v", hit, err)
	}
	if first.HasData || first.QualityStatus != model.QualityUnknown || p.calls != 1 {
		t.Fatalf("first empty membership response was not preserved: snapshot=%+v calls=%d", first, p.calls)
	}

	second, hit, err := svc.StockBoards(context.Background(), "600519.SH")
	if err != nil || !hit {
		t.Fatalf("second StockBoards request should hit the short-lived unknown cache: hit=%v err=%v", hit, err)
	}
	if second.HasData || second.QualityStatus != model.QualityUnknown || p.calls != 1 {
		t.Fatalf("cached result must remain unknown and avoid another provider call: snapshot=%+v calls=%d", second, p.calls)
	}
}


type fundamentalsCountingFakeProvider struct {
	snapshot *model.FundamentalsSnapshot
	calls    int
}

func (p *fundamentalsCountingFakeProvider) Name() string { return "baostock" }

func (p *fundamentalsCountingFakeProvider) FetchFundamentals(context.Context, string, int, int) (*model.FundamentalsSnapshot, error) {
	p.calls++
	return p.snapshot, nil
}

func setFundamentalsUnknownQuality(snapshot *model.FundamentalsSnapshot) {
	snapshot.QualityStatus = model.QualityUnknown
	for index := range snapshot.Sections {
		snapshot.Sections[index].QualityStatus = model.QualityUnknown
	}
}

func TestFundamentalsUnknownSnapshotWithDataUsesShortCacheTTL(t *testing.T) {
	now := time.Now().UTC()
	stale := validFundamentalsSnapshotForTest()
	setFundamentalsUnknownQuality(stale)
	stale.FetchedAt = now.Add(-10 * time.Minute)
	encoded, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v1:fundamentals:600519.SH:2026:2"] = encoded

	fresh := validFundamentalsSnapshotForTest()
	setFundamentalsUnknownQuality(fresh)
	fresh.FetchedAt = now
	p := &fundamentalsCountingFakeProvider{snapshot: fresh}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetFundamentalsProviders([]provider.FundamentalsProvider{p})

	got, hit, err := svc.Fundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil || hit {
		t.Fatalf("two-hour-old unknown fundamentals should refresh: hit=%v err=%v", hit, err)
	}
	if got == nil || got.QualityStatus != model.QualityUnknown || p.calls != 1 {
		t.Fatalf("refreshed fundamentals quality/calls mismatch: snapshot=%+v calls=%d", got, p.calls)
	}
}


type stockBasicFakeProvider struct {
	name  string
	item  *model.StockBasic
	calls int
}

func (p *stockBasicFakeProvider) Name() string {
	if strings.TrimSpace(p.name) == "" {
		return "eastmoney"
	}
	return p.name
}

func (p *stockBasicFakeProvider) FetchStockBasic(context.Context, string) (*model.StockBasic, error) {
	p.calls++
	return p.item, nil
}

func stockBasicIdentityFixtureForTest(symbol string) *model.StockBasic {
	return &model.StockBasic{
		Symbol: symbol, Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney",
		SourceVersion: "eastmoney-stock-basic-v2", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded, NormalizationVer: normalize.Version,
	}
}

func TestStockBasicNormalizesProviderAndCachedSymbols(t *testing.T) {
	t.Run("provider response", func(t *testing.T) {
		p := &stockBasicFakeProvider{name: "eastmoney", item: stockBasicIdentityFixtureForTest("600519")}
		svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
			ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
		svc.SetStockBasicProviders([]provider.StockBasicProvider{p})
		got, hit, err := svc.StockBasic(context.Background(), "600519.SH")
		if err != nil || hit {
			t.Fatalf("StockBasic returned hit=%v err=%v", hit, err)
		}
		if got == nil || got.Symbol != "600519.SH" {
			t.Fatalf("provider stock symbol must be returned canonically, got %+v", got)
		}
	})
	t.Run("cache payload", func(t *testing.T) {
		legacy := stockBasicIdentityFixtureForTest("600519")
		encoded, err := json.Marshal(legacy)
		if err != nil { t.Fatal(err) }
		mem := newMemoryCache()
		mem.values["md:v1:stock-basic:600519.SH"] = encoded
		svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
		got, hit, err := svc.StockBasic(context.Background(), "600519.SH")
		if err != nil || !hit {
			t.Fatalf("StockBasic cache returned hit=%v err=%v", hit, err)
		}
		if got == nil || got.Symbol != "600519.SH" {
			t.Fatalf("cached stock symbol must be returned canonically, got %+v", got)
		}
	})
}

func TestStockBasicFallsBackFromOldNormalizationVersion(t *testing.T) {
	now := time.Now().UTC()
	old := &stockBasicFakeProvider{name: "eastmoney", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney", SourceVersion: "eastmoney-stock-basic-v2",
		FetchedAt: now, QualityStatus: model.QualityDegraded, NormalizationVer: "old-normalization-v1",
	}}
	current := &stockBasicFakeProvider{name: "tushare", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "tushare", SourceVersion: "tushare-stock-basic-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded, NormalizationVer: normalize.Version,
	}}
	svc := NewWithCapabilities([]provider.NamedProvider{old, current}, nil, nil, newMemoryCache(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBasicProviders([]provider.StockBasicProvider{old, current})

	got, hit, err := svc.StockBasic(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("StockBasic returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.NormalizationVer != normalize.Version {
		t.Fatalf("old normalization contract must fall back to current version: %+v", got)
	}
	if old.calls != 1 || current.calls != 1 {
		t.Fatalf("normalization-version mismatch must trigger fallback, calls=%d/%d", old.calls, current.calls)
	}
}

func TestStockBasicCacheWithOldNormalizationVersionIsBypassed(t *testing.T) {
	now := time.Now().UTC()
	legacy := &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney", SourceVersion: "eastmoney-stock-basic-v2",
		FetchedAt: now.Add(-time.Minute), QualityStatus: model.QualityDegraded, NormalizationVer: "old-normalization-v1",
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v1:stock-basic:600519.SH"] = encoded
	current := &stockBasicFakeProvider{name: "tushare", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "tushare", SourceVersion: "tushare-stock-basic-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded, NormalizationVer: normalize.Version,
	}}
	svc := NewWithCapabilities([]provider.NamedProvider{current}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBasicProviders([]provider.StockBasicProvider{current})

	got, hit, err := svc.StockBasic(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("old-normalization cache should be bypassed: hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || current.calls != 1 {
		t.Fatalf("expected a fresh current-normalization record, got %+v calls=%d", got, current.calls)
	}
}

func TestValidateStockBasicRejectsOutDateBeforeIPODate(t *testing.T) {
	item := &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", IPODate: "2020-01-02", OutDate: "2019-12-31",
		SecurityType: 1, SecurityTypeLabel: "stock", ListingStatus: 0, ListingStatusLabel: "delisted",
		Source: "eastmoney", SourceVersion: "eastmoney-stock-basic-v2",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded, NormalizationVer: normalize.Version,
	}
	if err := validateStockBasic("600519.SH", item); err == nil || !strings.Contains(err.Error(), "out_date must not precede ipo_date") {
		t.Fatalf("out date before IPO date must be rejected, got %v", err)
	}
}

func TestValidateStockBasicRequiresSourceVersion(t *testing.T) {
	item := &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded, NormalizationVer: normalize.Version,
	}
	if err := validateStockBasic("600519.SH", item); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("stock-basic item without source version must be rejected, got %v", err)
	}
}

func TestValidateStockBasicRejectsInvalidShareCounts(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*model.StockBasic)
		want  string
	}{
		{name: "total shares NaN", setup: func(item *model.StockBasic) { value := math.NaN(); item.TotalShares = &value }, want: "total_shares is non-finite"},
		{name: "float shares infinity", setup: func(item *model.StockBasic) { value := math.Inf(1); item.FloatShares = &value }, want: "float_shares is non-finite"},
		{name: "total shares zero", setup: func(item *model.StockBasic) { value := 0.0; item.TotalShares = &value }, want: "total_shares must be positive"},
		{name: "float shares negative", setup: func(item *model.StockBasic) { value := -1.0; item.FloatShares = &value }, want: "float_shares must not be negative"},
		{name: "float shares exceed total", setup: func(item *model.StockBasic) { value := 1_000_001.0; item.FloatShares = &value }, want: "float_shares must not exceed total_shares"},
		{name: "float market cap exceeds total", setup: func(item *model.StockBasic) { total, floating := 1_000_000.0, 1_000_001.0; item.MarketCapCNY = &total; item.FloatMarketCapCNY = &floating }, want: "float_market_cap_cny must not exceed market_cap_cny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total, floating := 1_000_000.0, 800_000.0
			item := &model.StockBasic{
				Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
				ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney", SourceVersion: "eastmoney-stock-basic-v2",
				TotalShares: &total, FloatShares: &floating, FetchedAt: time.Now().UTC(),
				QualityStatus: model.QualityDegraded, NormalizationVer: normalize.Version,
			}
			tc.setup(item)
			if err := validateStockBasic("600519.SH", item); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected stock-basic share validation error %q, got %v", tc.want, err)
			}
		})
	}
}

func TestStockBasicFallsBackWhenProviderHasInconsistentShareCounts(t *testing.T) {
	now := time.Now().UTC()
	total, invalidFloat := 1_000_000.0, 1_100_000.0
	first := &stockBasicFakeProvider{name: "eastmoney", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney", SourceVersion: "eastmoney-stock-basic-v2",
		TotalShares: &total, FloatShares: &invalidFloat, FetchedAt: now, QualityStatus: model.QualityDegraded,
		NormalizationVer: normalize.Version,
	}}
	goodTotal, goodFloat := 1_000_000.0, 800_000.0
	second := &stockBasicFakeProvider{name: "tushare", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "tushare", SourceVersion: "tushare-stock-basic-v1",
		TotalShares: &goodTotal, FloatShares: &goodFloat, FetchedAt: now, QualityStatus: model.QualityDegraded,
		NormalizationVer: normalize.Version,
	}}
	svc := NewWithCapabilities([]provider.NamedProvider{first, second}, nil, nil, newMemoryCache(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBasicProviders([]provider.StockBasicProvider{first, second})
	got, hit, err := svc.StockBasic(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("StockBasic hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.FloatShares == nil || *got.FloatShares != goodFloat {
		t.Fatalf("expected fallback after invalid share counts, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("invalid share counts must trigger provider fallback, calls=%d/%d", first.calls, second.calls)
	}
}

func TestStockBasicFallsBackFromUnknownQualityProvider(t *testing.T) {
	now := time.Now().UTC()
	first := &stockBasicFakeProvider{name: "eastmoney", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityUnknown, NormalizationVer: normalize.Version,
	}}
	marketCap := 2000000000000.0
	second := &stockBasicFakeProvider{name: "tushare", item: &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: 1, ListingStatusLabel: "listed", Source: "tushare", SourceVersion: "test-v1", FetchedAt: now,
		MarketCapCNY: &marketCap, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"}, NormalizationVer: normalize.Version,
	}}
	svc := NewWithCapabilities([]provider.NamedProvider{first, second}, nil, nil, newMemoryCache(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBasicProviders([]provider.StockBasicProvider{first, second})

	got, hit, err := svc.StockBasic(context.Background(), "600519")
	if err != nil || hit {
		t.Fatalf("StockBasic hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded fallback after unknown first source, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both stock-basic sources to be tried, calls=%d/%d", first.calls, second.calls)
	}
}

func TestStockBasicRetainsUnknownAfterFallbackExhaustion(t *testing.T) {
	now := time.Now().UTC()
	p := &stockBasicFakeProvider{
		item: &model.StockBasic{
			Symbol: "600519.SH", Name: "贵州茅台",
			SecurityType: 1, SecurityTypeLabel: "stock",
			ListingStatus: 1, ListingStatusLabel: "listed",
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityUnknown,
			NormalizationVer: normalize.Version,
		},
	}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, newMemoryCache(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBasicProviders([]provider.StockBasicProvider{p})

	got, hit, err := svc.StockBasic(context.Background(), "600519")
	if err != nil || hit {
		t.Fatalf("StockBasic hit=%v err=%v", hit, err)
	}
	if got.QualityStatus != model.QualityUnknown {
		t.Fatalf("unknown Provider quality must remain unknown after fallback exhaustion, got %q", got.QualityStatus)
	}
	foundReason := false
	for _, reason := range got.QualityReasons {
		if reason == "stock_basic_fallback_exhausted_without_usable_quality" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("unknown stock-basic record is missing its fallback-exhaustion reason: %+v", got.QualityReasons)
	}
}


type supplementCountingFakeProvider struct {
	chip      *model.ChipDistribution
	flow      *model.CapitalFlow
	chipCalls int
	flowCalls int
}

func (p *supplementCountingFakeProvider) Name() string { return "miaoxiang" }

func (p *supplementCountingFakeProvider) FetchChipDistribution(context.Context, string) (*model.ChipDistribution, error) {
	p.chipCalls++
	return p.chip, nil
}

func (p *supplementCountingFakeProvider) FetchCapitalFlow(context.Context, string) (*model.CapitalFlow, error) {
	p.flowCalls++
	return p.flow, nil
}

func TestSupplementContractsRequireSourceVersion(t *testing.T) {
	now := time.Now().UTC()
	chip := &model.ChipDistribution{
		Symbol: "600519.SH", AverageCost: 12.5, Source: "eastmoney", FetchedAt: now,
		QualityStatus: model.QualityDegraded,
	}
	if err := validateChipDistributionSnapshot("600519.SH", chip); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("chip-distribution without source version must be rejected, got %v", err)
	}

	mainNetInflow := 120.0
	flow := &model.CapitalFlow{
		Symbol: "600519.SH", MainNetInflow: &mainNetInflow, Observations: 1,
		Source: "eastmoney", FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	if err := validateCapitalFlowSnapshot("600519.SH", flow); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("capital-flow without source version must be rejected, got %v", err)
	}
}

func TestSupplementCachesReuseProviderContractValidation(t *testing.T) {
	now := time.Now().UTC()
	t.Run("invalid chip ratio cache is bypassed", func(t *testing.T) {
		badRatio, goodRatio := 1.2, 0.8
		cached := &model.ChipDistribution{
			Symbol: "600519.SH", AverageCost: 12.5, ProfitRatio: &badRatio,
			Source: "miaoxiang", SourceVersion: "test", FetchedAt: now.Add(-time.Minute),
			QualityStatus: model.QualityDegraded,
		}
		encoded, err := json.Marshal(cached)
		if err != nil {
			t.Fatal(err)
		}
		mem := newMemoryCache()
		mem.values["md:v1:chip:600519.SH"] = encoded
		p := &supplementCountingFakeProvider{chip: &model.ChipDistribution{
			Symbol: "600519.SH", AverageCost: 12.5, ProfitRatio: &goodRatio,
			SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded,
		}}
		svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
			ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
		svc.SetSupplementProviders([]provider.SupplementProvider{p})
		got, hit, err := svc.ChipDistribution(context.Background(), "600519")
		if err != nil || hit || p.chipCalls != 1 {
			t.Fatalf("invalid cached chip ratio should trigger provider refresh: hit=%v calls=%d err=%v", hit, p.chipCalls, err)
		}
		if got.ProfitRatio == nil || *got.ProfitRatio != goodRatio {
			t.Fatalf("fresh chip ratio was not returned: %+v", got)
		}
	})
	t.Run("incomplete flow sum cache is bypassed", func(t *testing.T) {
		badFiveDay, goodFiveDay := 300.0, 500.0
		cached := &model.CapitalFlow{
			Symbol: "600519.SH", Observations: 2, Inflow5D: &badFiveDay,
			Source: "miaoxiang", SourceVersion: "test", FetchedAt: now.Add(-time.Minute),
			QualityStatus: model.QualityDegraded,
		}
		encoded, err := json.Marshal(cached)
		if err != nil {
			t.Fatal(err)
		}
		mem := newMemoryCache()
		mem.values["md:v1:capital-flow:600519.SH"] = encoded
		p := &supplementCountingFakeProvider{flow: &model.CapitalFlow{
			Symbol: "600519.SH", Observations: 5, Inflow5D: &goodFiveDay,
			SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded,
		}}
		svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
			ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
		svc.SetSupplementProviders([]provider.SupplementProvider{p})
		got, hit, err := svc.CapitalFlow(context.Background(), "600519")
		if err != nil || hit || p.flowCalls != 1 {
			t.Fatalf("incomplete cached five-day sum should trigger provider refresh: hit=%v calls=%d err=%v", hit, p.flowCalls, err)
		}
		if got.Inflow5D == nil || *got.Inflow5D != goodFiveDay {
			t.Fatalf("fresh capital-flow sum was not returned: %+v", got)
		}
	})
}


func TestSupplementQualityCannotBeValidWithoutAvailabilityTime(t *testing.T) {
	now := time.Now().UTC()
	mainInflow, fiveDay := 120.0, 500.0
	p := &supplementCountingFakeProvider{
		chip: &model.ChipDistribution{
			Symbol: "600519.SH", AverageCost: 12.5, SourceVersion: "test",
			FetchedAt: now, QualityStatus: model.QualityValid,
		},
		flow: &model.CapitalFlow{
			Symbol: "600519.SH", MainNetInflow: &mainInflow, Inflow5D: &fiveDay,
			Observations: 5, SourceVersion: "test", FetchedAt: now,
			QualityStatus: model.QualityValid,
		},
	}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetSupplementProviders([]provider.SupplementProvider{p})

	chip, _, err := svc.ChipDistribution(context.Background(), "600519")
	if err != nil {
		t.Fatalf("ChipDistribution: %v", err)
	}
	if chip.QualityStatus != model.QualityDegraded || !strings.Contains(strings.Join(chip.QualityReasons, ","), "publication_availability_timestamp_unverified") {
		t.Fatalf("chip result without availability timestamp must be degraded: %+v", chip)
	}

	flow, _, err := svc.CapitalFlow(context.Background(), "600519")
	if err != nil {
		t.Fatalf("CapitalFlow: %v", err)
	}
	if flow.QualityStatus != model.QualityDegraded || !strings.Contains(strings.Join(flow.QualityReasons, ","), "publication_availability_timestamp_unverified") {
		t.Fatalf("capital-flow result without availability timestamp must be degraded: %+v", flow)
	}
}


func TestSupplementCacheRejectsOldValidQualityWithoutAvailabilityTime(t *testing.T) {
	now := time.Now().UTC()
	t.Run("chip", func(t *testing.T) {
		ratio := 0.8
		cached := &model.ChipDistribution{
			Symbol: "600519.SH", AverageCost: 12.5, ProfitRatio: &ratio,
			Source: "miaoxiang", SourceVersion: "legacy", FetchedAt: now.Add(-time.Minute),
			QualityStatus: model.QualityValid,
		}
		encoded, err := json.Marshal(cached)
		if err != nil {
			t.Fatal(err)
		}
		mem := newMemoryCache()
		mem.values["md:v1:chip:600519.SH"] = encoded
		fresh := &model.ChipDistribution{Symbol: "600519.SH", AverageCost: 12.5, ProfitRatio: &ratio,
			SourceVersion: "current", FetchedAt: now, QualityStatus: model.QualityDegraded}
		p := &supplementCountingFakeProvider{chip: fresh}
		svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
			ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
		svc.SetSupplementProviders([]provider.SupplementProvider{p})
		got, hit, err := svc.ChipDistribution(context.Background(), "600519")
		if err != nil || hit || p.chipCalls != 1 || got.QualityStatus != model.QualityDegraded {
			t.Fatalf("legacy valid chip cache must be refreshed and downgraded: hit=%v calls=%d got=%+v err=%v", hit, p.chipCalls, got, err)
		}
	})
	t.Run("capital flow", func(t *testing.T) {
		fiveDay := 500.0
		cached := &model.CapitalFlow{
			Symbol: "600519.SH", Observations: 5, Inflow5D: &fiveDay,
			Source: "miaoxiang", SourceVersion: "legacy", FetchedAt: now.Add(-time.Minute),
			QualityStatus: model.QualityValid,
		}
		encoded, err := json.Marshal(cached)
		if err != nil {
			t.Fatal(err)
		}
		mem := newMemoryCache()
		mem.values["md:v1:capital-flow:600519.SH"] = encoded
		fresh := &model.CapitalFlow{Symbol: "600519.SH", Observations: 5, Inflow5D: &fiveDay,
			SourceVersion: "current", FetchedAt: now, QualityStatus: model.QualityDegraded}
		p := &supplementCountingFakeProvider{flow: fresh}
		svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, mem, nil, time.Second, time.Minute, 120, 2000,
			ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
		svc.SetSupplementProviders([]provider.SupplementProvider{p})
		got, hit, err := svc.CapitalFlow(context.Background(), "600519")
		if err != nil || hit || p.flowCalls != 1 || got.QualityStatus != model.QualityDegraded {
			t.Fatalf("legacy valid capital-flow cache must be refreshed and downgraded: hit=%v calls=%d got=%+v err=%v", hit, p.flowCalls, got, err)
		}
	})
}


func TestValidateFundamentalsSnapshotRejectsFutureReportPeriod(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	futureYear := time.Now().In(marketLocation()).Year() + 1
	snapshot.Year = futureYear
	snapshot.Quarter = 1
	futureReportDate := fundamentalPeriodEnd(futureYear, 1)
	publicationDate := time.Now().In(marketLocation()).AddDate(0, 0, -1).Format("2006-01-02")
	for index := range snapshot.Sections {
		snapshot.Sections[index].ReportDate = futureReportDate
		snapshot.Sections[index].PublicationDate = publicationDate
	}
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "report period is in the future") {
		t.Fatalf("future financial report period must be rejected, got %v", err)
	}
}


func TestValidateFundamentalsSnapshotRejectsPublicationBeforeReportPeriodEnd(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	for index := range snapshot.Sections {
		snapshot.Sections[index].PublicationDate = "2026-06-29"
	}
	if err := validateFundamentalsSnapshot(snapshot.Symbol, snapshot.Year, snapshot.Quarter, snapshot); err == nil || !strings.Contains(err.Error(), "publication date precedes") {
		t.Fatalf("publication before period end must be rejected, got %v", err)
	}
}


func TestIndexCatalogUnknownCacheUsesShortTTL(t *testing.T) {
	if got := indexCatalogCacheTTL(&model.IndexCatalogSnapshot{QualityStatus: model.QualityUnknown}); got != 5*time.Minute {
		t.Fatalf("empty unknown index catalog TTL=%s, want 5m", got)
	}
	if got := indexCatalogCacheTTL(&model.IndexCatalogSnapshot{Items: []model.IndexCatalogItem{{Symbol: "000300.SH", Name: "沪深300", Tag: "cn_concept"}}, QualityStatus: model.QualityDegraded}); got != time.Hour {
		t.Fatalf("degraded index catalog TTL=%s, want 1h", got)
	}
}

func TestValidateIndexConstituentsRejectsFutureReportDate(t *testing.T) {
	futureDate := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	snapshot := &model.IndexConstituentSnapshot{
		IndexSymbol: "000300.SH", ProviderReportDate: futureDate,
		Items: []model.IndexConstituent{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"}},
		Total: 1, Source: "tushare", SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
	}
	if err := validateIndexConstituentsSnapshot("000300.SH", snapshot); err == nil || !strings.Contains(err.Error(), "in the future") {
		t.Fatalf("future constituent report date must be rejected, got %v", err)
	}
}


func TestIndexConstituentCoverageCountersAndLegacyDowngrade(t *testing.T) {
	now := time.Now().UTC()
	makeSnapshot := func() *model.IndexConstituentSnapshot {
		return &model.IndexConstituentSnapshot{
			IndexSymbol: "000300.SH", ProviderReportDate: time.Now().In(marketLocation()).Format("2006-01-02"),
			Items: []model.IndexConstituent{
				{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"},
				{Symbol: "000001.SZ", Code: "000001", Name: "平安银行"},
			},
			Total: 2, ProviderReturned: 3, ExcludedCount: 1,
			Source: "tushare", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			QualityReasons: []string{"one_or_more_constituent_names_missing_from_current_stock_directory"},
		}
	}

	t.Run("consistent source counts", func(t *testing.T) {
		if err := validateIndexConstituentsSnapshot("000300.SH", makeSnapshot()); err != nil {
			t.Fatalf("consistent source/accepted/excluded counts must pass: %v", err)
		}
	})
	t.Run("inconsistent source counts", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.ExcludedCount = 0
		if err := validateIndexConstituentsSnapshot("000300.SH", snapshot); err == nil || !strings.Contains(err.Error(), "row counts are inconsistent") {
			t.Fatalf("mismatched source counts must be rejected, got %v", err)
		}
	})
	t.Run("excluded rows cannot be valid", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.QualityStatus = model.QualityValid
		if err := validateIndexConstituentsSnapshot("000300.SH", snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("snapshot with excluded rows must not be valid, got %v", err)
		}
	})
	t.Run("legacy snapshot is downgraded", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.ProviderReturned = 0
		snapshot.ExcludedCount = 0
		snapshot.QualityStatus = model.QualityValid
		snapshot.QualityReasons = nil
		markIndexConstituentCoverage(snapshot)
		if snapshot.QualityStatus != model.QualityDegraded {
			t.Fatalf("legacy snapshot without coverage metadata must be degraded, got %q", snapshot.QualityStatus)
		}
		found := false
		for _, reason := range snapshot.QualityReasons {
			if reason == "provider_constituent_source_coverage_unreported" {
				found = true
			}
		}
		if !found {
			t.Fatalf("legacy coverage reason missing: %+v", snapshot.QualityReasons)
		}
	})
}

func TestValidateIndexDailyBarsRejectsQualityAndUnitMismatch(t *testing.T) {
	now := time.Now().UTC()
	volume := int64(1000)
	amount := 100000.0
	makeSnapshot := func() *model.IndexDailyBarsSnapshot {
		return &model.IndexDailyBarsSnapshot{
			IndexSymbol: "000300.SH", StartDate: "2026-10-26", EndDate: "2026-10-26",
			Items: []model.IndexDailyBar{{
				IndexSymbol: "000300.SH", TradeDate: "2026-10-26",
				Open: 10, High: 11, Low: 9, Close: 10.5,
				Volume: &volume, Amount: &amount, VolumeUnit: "shares", AmountUnit: "CNY",
				Source: "tushare", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			}},
			Total: 1, Source: "tushare", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
		}
	}
	t.Run("parent quality", func(t *testing.T) {
		snapshot := makeSnapshot()
		if err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-26", "2026-10-26", snapshot); err == nil || !strings.Contains(err.Error(), "cannot be valid") {
			t.Fatalf("valid parent with degraded child must be rejected, got %v", err)
		}
	})
	t.Run("unknown volume unit", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.Items[0].QualityStatus = model.QualityDegraded
		snapshot.Items[0].VolumeUnit = "unknown"
		if err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-26", "2026-10-26", snapshot); err == nil || !strings.Contains(err.Error(), "unsupported volume_unit") {
			t.Fatalf("volume with unknown unit must be rejected, got %v", err)
		}
	})
	t.Run("unverified amount unit", func(t *testing.T) {
		snapshot := makeSnapshot()
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.Items[0].QualityStatus = model.QualityValid
		snapshot.Items[0].AmountUnit = "provider_defined"
		if err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-26", "2026-10-26", snapshot); err == nil || !strings.Contains(err.Error(), "provider_defined amount unit") {
			t.Fatalf("valid bar with unverified amount unit must be rejected, got %v", err)
		}
	})
	t.Run("non-finite change percent", func(t *testing.T) {
		snapshot := makeSnapshot()
		bad := math.NaN()
		snapshot.Items[0].ChangePercent = &bad
		if err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-26", "2026-10-26", snapshot); err == nil || !strings.Contains(err.Error(), "non-finite change_percent") {
			t.Fatalf("non-finite change percent must be rejected, got %v", err)
		}
	})
}


func TestSupplementFetchedAtUsesServiceReceiveTime(t *testing.T) {
	future := time.Now().UTC().Add(24 * time.Hour)
	mainInflow, fiveDay := 120.0, 500.0
	p := &supplementCountingFakeProvider{
		chip: &model.ChipDistribution{
			Symbol: "600519.SH", AverageCost: 12.5, SourceVersion: "test",
			FetchedAt: future, QualityStatus: model.QualityDegraded,
		},
		flow: &model.CapitalFlow{
			Symbol: "600519.SH", MainNetInflow: &mainInflow, Inflow5D: &fiveDay,
			Observations: 5, SourceVersion: "test", FetchedAt: future,
			QualityStatus: model.QualityDegraded,
		},
	}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetSupplementProviders([]provider.SupplementProvider{p})

	start := time.Now().UTC()
	chip, _, err := svc.ChipDistribution(context.Background(), "600519")
	end := time.Now().UTC()
	if err != nil {
		t.Fatalf("ChipDistribution: %v", err)
	}
	if chip.FetchedAt.Before(start) || chip.FetchedAt.After(end) {
		t.Fatalf("chip fetched_at must be stamped at service receive time: start=%s fetched_at=%s end=%s", start, chip.FetchedAt, end)
	}

	start = time.Now().UTC()
	flow, _, err := svc.CapitalFlow(context.Background(), "600519")
	end = time.Now().UTC()
	if err != nil {
		t.Fatalf("CapitalFlow: %v", err)
	}
	if flow.FetchedAt.Before(start) || flow.FetchedAt.After(end) {
		t.Fatalf("capital-flow fetched_at must be stamped at service receive time: start=%s fetched_at=%s end=%s", start, flow.FetchedAt, end)
	}
}

func TestChipDistributionRejectsFutureProviderDate(t *testing.T) {
	futureDate := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	p := &supplementCountingFakeProvider{
		chip: &model.ChipDistribution{
			Symbol: "600519.SH", Date: futureDate, AverageCost: 12.5,
			SourceVersion: "test", FetchedAt: time.Now().UTC(),
			QualityStatus: model.QualityDegraded,
		},
	}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetSupplementProviders([]provider.SupplementProvider{p})

	if _, _, err := svc.ChipDistribution(context.Background(), "600519"); err == nil || !strings.Contains(err.Error(), "provider date is in the future") {
		t.Fatalf("future chip-distribution date must be rejected, got %v", err)
	}
}

func TestFundamentalsFetchedAtUsesServiceReceiveTime(t *testing.T) {
	snapshot := validFundamentalsSnapshotForTest()
	snapshot.FetchedAt = time.Now().UTC().Add(24 * time.Hour)
	p := &fundamentalsCountingFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetFundamentalsProviders([]provider.FundamentalsProvider{p})

	start := time.Now().UTC()
	got, hit, err := svc.Fundamentals(context.Background(), "600519.SH", 2026, 2)
	end := time.Now().UTC()
	if err != nil || hit {
		t.Fatalf("Fundamentals hit=%v err=%v", hit, err)
	}
	if got.FetchedAt.Before(start) || got.FetchedAt.After(end) {
		t.Fatalf("fundamentals fetched_at must be stamped at service receive time: start=%s fetched_at=%s end=%s", start, got.FetchedAt, end)
	}
}


func TestQuoteTimestampsAreStampedByService(t *testing.T) {
	q := validQuote("eastmoney")
	future := time.Now().UTC().Add(24 * time.Hour)
	q.FetchedAt = future
	q.NormalizedAt = future
	p := fakeProvider{name: "eastmoney", quote: q}
	svc := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	start := time.Now().UTC()
	got, _, err := svc.Quote(context.Background(), "600519")
	end := time.Now().UTC()
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if got.FetchedAt.Before(start) || got.FetchedAt.After(end) {
		t.Fatalf("quote fetched_at must use service receive time: start=%s fetched_at=%s end=%s", start, got.FetchedAt, end)
	}
	if got.NormalizedAt.Before(start) || got.NormalizedAt.After(end) {
		t.Fatalf("quote normalized_at must use service normalization time: start=%s normalized_at=%s end=%s", start, got.NormalizedAt, end)
	}
}

func TestDailyBarTimestampsAreStampedByService(t *testing.T) {
	volume := int64(1000)
	amount := 10500.0
	future := time.Now().UTC().Add(24 * time.Hour)
	p := fakeProvider{
		name: "eastmoney",
		bars: []model.DailyBar{{
			SourceVersion: "test-v1",
			Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
			Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentNone,
			Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY",
			QualityStatus: model.QualityValid, FetchedAt: future, NormalizedAt: future,
		}},
	}
	svc := New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	start := time.Now().UTC()
	bars, hit, _, err := svc.DailyBars(context.Background(), "600519", "2026-10-20", "2026-10-20", 1, model.AdjustmentNone)
	end := time.Now().UTC()
	if err != nil || hit {
		t.Fatalf("DailyBars hit=%v err=%v", hit, err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one validated daily bar, got %d", len(bars))
	}
	if bars[0].FetchedAt.Before(start) || bars[0].FetchedAt.After(end) {
		t.Fatalf("daily-bar fetched_at must use service receive time: start=%s fetched_at=%s end=%s", start, bars[0].FetchedAt, end)
	}
	if bars[0].NormalizedAt.Before(start) || bars[0].NormalizedAt.After(end) {
		t.Fatalf("daily-bar normalized_at must use service normalization time: start=%s normalized_at=%s end=%s", start, bars[0].NormalizedAt, end)
	}
}


func TestQuoteCacheWithFutureServiceTimestampsIsBypassed(t *testing.T) {
	mem := newMemoryCache()
	cached := validQuote("eastmoney")
	future := time.Now().UTC().Add(24 * time.Hour)
	cached.FetchedAt = future
	cached.NormalizedAt = future
	payload, err := json.Marshal(cached)
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:quote:600519.SH"] = payload

	fresh := validQuote("tencent")
	svc := New([]provider.Provider{fakeProvider{name: "tencent", quote: fresh}}, mem, nil, time.Second, time.Second, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, hit, err := svc.Quote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if hit || got.Source != "tencent" {
		t.Fatalf("quote cache with future service timestamps must be bypassed: hit=%v source=%s", hit, got.Source)
	}
}

func TestDailyBarsCacheWithFutureServiceTimestampsIsBypassed(t *testing.T) {
	mem := newMemoryCache()
	volume := int64(1000)
	amount := 10500.0
	future := time.Now().UTC().Add(24 * time.Hour)
	cached := model.DailyBar{
		SourceVersion: "test-v1",
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward,
		Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY",
		QualityStatus: model.QualityValid, NormalizationVer: normalize.Version,
		FetchedAt: future, NormalizedAt: future,
	}
	payload, err := json.Marshal(map[string]any{"bars": []model.DailyBar{cached}, "warnings": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	mem.values["md:v4:bars:600519.SH:2026-10-20:2026-10-20:forward_adjusted:1"] = payload

	providerBar := cached
	providerBar.Source = "tencent"
	providerBar.FetchedAt = time.Time{}
	providerBar.NormalizedAt = time.Time{}
	svc := New([]provider.Provider{fakeProvider{name: "tencent", bars: []model.DailyBar{providerBar}}}, mem, nil, time.Second, time.Minute, 120,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	bars, hit, _, err := svc.DailyBars(context.Background(), "600519.SH", "2026-10-20", "2026-10-20", 1, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if hit || len(bars) != 1 || bars[0].Source != "tencent" {
		t.Fatalf("daily-bar cache with future service timestamps must be bypassed: hit=%v bars=%+v", hit, bars)
	}
}


func TestDailyBarsRejectsNonCanonicalRequestDate(t *testing.T) {
	svc := New([]provider.Provider{fakeProvider{name: "eastmoney"}}, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, _, _, err := svc.DailyBars(context.Background(), "600519", "2026-1-02", "2026-10-20", 10, model.AdjustmentNone); err == nil {
		t.Fatal("non-canonical start date must be rejected before provider/cache lookup")
	}
}
