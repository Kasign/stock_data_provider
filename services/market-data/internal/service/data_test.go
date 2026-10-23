package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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
	return &model.Quote{Symbol: "600519.SH", Name: "贵州茅台", CurrentPrice: &p, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &vol, Amount: &amount, QuoteTime: &qt, FetchedAt: now, NormalizedAt: now, Source: source, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
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
	p.bars = []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &v, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}
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
		{Symbol: "600519.SH", TradeDate: "2026-10-16", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
		{Symbol: "600519.SH", TradeDate: "2026-10-19", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
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
	cached := []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amt, AdjustmentBasis: model.AdjustmentBackward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}
	payload, _ := json.Marshal(map[string]any{"bars": cached, "warnings": []string{}})
	mem.values["md:v4:bars:600519.SH:::forward_adjusted:120"] = payload
	providerVol := int64(20000)
	providerAmt := float64(200000)
	p := fakeProvider{name: "eastmoney", bars: []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &providerVol, Amount: &providerAmt, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}}
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

func TestDailyBarsRejectsProviderAdjustmentMismatchAndUsesFallback(t *testing.T) {
	wrongVol, rightVol := int64(10000), int64(20000)
	amount := float64(100000)
	wrong := fakeProvider{name: "eastmoney", bars: []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &wrongVol, Amount: &amount, AdjustmentBasis: model.AdjustmentNone, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}}
	right := fakeProvider{name: "tencent", bars: []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &rightVol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "tencent", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}}
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
	valid := model.DailyBar{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &vol, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}
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
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5,
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
	old := model.DailyBar{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid, NormalizationVer: "1"}
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
		{Symbol: "600519.SH", TradeDate: currentDate, Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
		{Symbol: "600519.SH", TradeDate: futureDate, Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
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
	bar := model.DailyBar{TradeDate: "2026-10-21", IsPartial: true}
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
	a := model.DailyBar{Source: "eastmoney", TradeDate: "2026-10-20", AdjustmentBasis: model.AdjustmentNone, NormalizationVer: normalize.Version}
	b := a
	b.Source = "tencent"
	b.TradeDate = "2026-10-21"
	series, reason := coherentStoredSeries([]model.DailyBar{a, b}, model.AdjustmentNone)
	if reason != "" || len(series) != 1 || series[0].Source == "" {
		t.Fatalf("should return a single source series, got %+v reason=%s", series, reason)
	}
	_, reason = coherentStoredSeries([]model.DailyBar{{Source: "eastmoney", TradeDate: "2026-10-20", AdjustmentBasis: model.AdjustmentForward, NormalizationVer: normalize.Version}}, model.AdjustmentForward)
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

func TestStockBoardsValidatesIdentityAndReturnsSnapshot(t *testing.T) {
	now := time.Now().UTC()
	change := 1.25
	snapshot := &model.StockBoardSnapshot{
		Symbol: "600519.SH", Total: 1, HasData: true,
		Items:  []model.StockBoard{{Code: "BK0477", Name: "酿酒行业", ChangePercent: &change, Source: "eastmoney", FetchedAt: now, QualityStatus: model.QualityDegraded}},
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
}

func TestStockBoardsRejectsProviderSymbolMismatch(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &model.StockBoardSnapshot{Symbol: "000001.SZ", Total: 1, HasData: true, Items: []model.StockBoard{{Code: "BK1", Name: "测试", Source: "eastmoney", FetchedAt: now, QualityStatus: model.QualityDegraded}}, Source: "eastmoney", FetchedAt: now, QualityStatus: model.QualityDegraded}
	p := stockBoardsFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBoardsProviders([]provider.StockBoardsProvider{p})
	_, _, err := svc.StockBoards(context.Background(), "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "all stock-boards providers failed") {
		t.Fatalf("expected stock-board identity mismatch, got %v", err)
	}
}
