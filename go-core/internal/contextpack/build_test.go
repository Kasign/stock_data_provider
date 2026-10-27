package contextpack

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

func TestToLegacyAnalyzerContextProvidesExpectedLegacyKeys(t *testing.T) {
	price := 10.5
	profit := 0.62
	concentration := 0.12
	avgCost := 9.7
	ma5 := 10.0
	ma10 := 9.5
	ma20 := 9.0
	ctx := domain.AnalysisContext{Code: "600396.SH", Name: "测试股票", Market: "cn", AsOf: time.Date(2026, 10, 20, 7, 0, 0, 0, time.UTC), Quote: &domain.Quote{Code: "600396.SH", Price: &price, VolumeRatio: floatPointer(1.4)}, DailyBars: []domain.DailyBar{{Date: "2026-10-19", Open: 9, High: 10, Low: 8.8, Close: 9.8, PctChg: 1.0, AdjustmentBasis: "none"}, {Date: "2026-10-20", Open: 10, High: 11, Low: 9.9, Close: 10.2, PctChg: 4.0, AdjustmentBasis: "none"}}, Technical: domain.TechnicalSnapshot{MA5: &ma5, MA10: &ma10, MA20: &ma20, MAAlignment: "bullish", TrendStatus: "uptrend"}, Chip: &domain.ChipDistribution{ProfitRatio: &profit, Concentration90: &concentration, CostAverage: &avgCost}}
	got := ToLegacyAnalyzerContext(ctx)
	for _, key := range []string{"code", "stock_name", "today", "yesterday", "realtime", "chip", "ma_status", "trend_analysis"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing legacy prompt key %q", key)
		}
	}
	if got["market"] != "cn" {
		t.Errorf("A-share suffix .SH misclassified as market %v", got["market"])
	}
	today, ok := got["today"].(map[string]any)
	if !ok {
		t.Fatalf("today must be map, got %T", got["today"])
	}
	if today["close"] != 10.2 {
		t.Errorf("today close mismatch: %v", today["close"])
	}
	chip, ok := got["chip"].(map[string]any)
	if !ok || chip["profit_ratio"] != 0.62 || chip["avg_cost"] != 9.7 {
		t.Errorf("chip normalization incorrect: %#v", got["chip"])
	}
}

func TestInferMarketUnderstandsKnownSymbols(t *testing.T) {
	tests := map[string]string{"600396": "cn", "600396.SH": "cn", "SH600396": "cn", "HK00700": "hk", "0700.HK": "hk", "AAPL": "us", "7203.T": "jp", "005930.KS": "kr", "2330.TW": "tw"}
	for code, want := range tests {
		if got := inferMarket(code); got != want {
			t.Errorf("inferMarket(%q)=%q want %q", code, got, want)
		}
	}
}
func floatPointer(v float64) *float64 { return &v }

func TestToLegacyContextKeepsMissingDailyMetricsUnknown(t *testing.T) {
	ctx := domain.AnalysisContext{
		Code: "600396", Name: "测试股票", Market: "cn", AsOf: time.Date(2026, 10, 20, 7, 0, 0, 0, time.UTC),
		DailyBars: []domain.DailyBar{{Date: "2026-10-20", Open: 10, High: 12, Low: 9, Close: 11, Volume: 0, Amount: 0, PctChg: 0, MissingFields: []string{"amount", "pct_chg"}}},
	}
	legacy := ToLegacyAnalyzerContext(ctx)
	today := legacy["today"].(map[string]any)
	if today["amount"] != nil || today["pct_chg"] != nil {
		t.Fatalf("missing daily fields must remain unknown, got %#v", today)
	}
	quality := ctx.DataQuality
	_ = quality
}

func TestRealtimeUnadjustedPriceDoesNotOverrideAdjustedCurrentPrice(t *testing.T) {
	realTimePrice := 100.0
	bars := []domain.DailyBar{{Date: "2026-10-20", Open: 90, High: 95, Low: 89, Close: 93, AdjustmentBasis: "forward_adjusted"}}
	ctx := domain.AnalysisContext{Code: "600519.SH", Market: "cn", Quote: &domain.Quote{Code: "600519.SH", Source: "tencent", Price: &realTimePrice, AdjustmentBasis: "none", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: "valid", QuoteTime: "2026-10-20T08:00:00Z"}, DailyBars: bars}
	legacy := ToLegacyAnalyzerContext(ctx)
	if _, exists := legacy["current_price"]; exists {
		t.Fatalf("live price must be omitted when realtime and daily adjustment bases differ: %#v", legacy["current_price"])
	}
	if legacy["last_completed_close"] != 93.0 || legacy["last_completed_close_semantics"] != "historical_completed_daily_close_not_realtime_price" {
		t.Fatalf("last completed close must be clearly identified as historical, got %#v", legacy)
	}
	realtime := legacy["realtime"].(map[string]any)
	if realtime["price_basis_comparable_to_daily_bars"] != false || realtime["price_omitted_from_cross_basis_comparisons"] != true {
		t.Fatalf("expected an explicit cross-basis comparison guard: %#v", realtime)
	}
	if realtime["price"] != 100.0 {
		t.Fatalf("realtime price should remain available as a separate labelled fact: %#v", realtime)
	}
}

func TestUntrustedRealtimeQuoteNumbersAreOmitted(t *testing.T) {
	price := 100.0
	quote := &domain.Quote{Code: "600519.SH", Price: &price, QuoteTime: "2026-10-20T08:00:00Z", QualityStatus: "stale", QualityReasons: []string{"quote_older_than_180_seconds_during_trading_clock"}}
	ctx := domain.AnalysisContext{Code: "600519.SH", Name: "测试", Quote: quote, DailyBars: []domain.DailyBar{{Date: "2026-10-19", Open: 90, High: 95, Low: 89, Close: 93, AdjustmentBasis: "none"}}}
	got := ToLegacyAnalyzerContext(ctx)
	if _, exists := got["current_price"]; exists {
		t.Fatalf("stale quote must not relabel historical close as current_price: %#v", got["current_price"])
	}
	if got["last_completed_close"] != 93.0 {
		t.Fatalf("stale quote should retain a separately labelled historical reference, got %#v", got["last_completed_close"])
	}
	realtime := got["realtime"].(map[string]any)
	if realtime["price_omitted_due_to_quality"] != true {
		t.Fatalf("stale quote should have an omission marker: %#v", realtime)
	}
	if _, ok := realtime["price"]; ok {
		t.Fatalf("stale realtime numeric price must not be sent to LLM: %#v", realtime)
	}
}

func TestPartialDailyBarIsNotPublishedAsCompletedToday(t *testing.T) {
	ctx := domain.AnalysisContext{Code: "600519.SH", Name: "测试", DailyBars: []domain.DailyBar{
		{Date: "2026-10-19", Open: 90, High: 95, Low: 89, Close: 93, AdjustmentBasis: "none"},
		{Date: "2026-10-20", Open: 93, High: 101, Low: 92, Close: 100, IsPartial: true, AdjustmentBasis: "none"},
	}}
	got := ToLegacyAnalyzerContext(ctx)
	today := got["today"].(map[string]any)
	if today["date"] != "2026-10-19" || today["close"] != 93.0 {
		t.Fatalf("today must remain the latest completed daily bar, got %#v", today)
	}
	partial := got["intraday_partial_bar"].(map[string]any)
	if partial["is_partial"] != true || partial["close"] != 100.0 {
		t.Fatalf("partial intraday bar should be a separate labelled overlay, got %#v", partial)
	}
}

func TestLegacyDailyDataNullsUnknownUnitMetrics(t *testing.T) {
	price := 10.5
	bars := []domain.DailyBar{{
		Date: "2026-10-19", Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: 123456, Amount: 1296000, PctChg: 1.2,
		VolumeUnit: "unknown", AmountUnit: "unknown", AdjustmentBasis: "unknown",
		QualityStatus: "unknown", MissingFields: nil,
	}}
	ctx := ToLegacyAnalyzerContext(BuildStockContext(domain.StockBundle{Code: "600519", Quote: &domain.Quote{Code: "600519", Price: &price}, DailyBars: bars}, domain.TechnicalSnapshot{}, nil, "2026-10-19T08:00:00Z"))
	for _, key := range []string{"daily_data", "history"} {
		rows, ok := ctx[key].([]map[string]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("%s has unexpected shape: %#v", key, ctx[key])
		}
		if rows[0]["volume"] != nil || rows[0]["amount"] != nil {
			t.Fatalf("unknown unit metrics leaked as numbers through %s: %#v", key, rows[0])
		}
		if rows[0]["volume_unit"] != "unknown" || rows[0]["adjustment_basis"] != "unknown" {
			t.Fatalf("provenance did not survive %s serialization: %#v", key, rows[0])
		}
	}
}

func TestUnknownMatchingAdjustmentBasisStillOmitsRealtimePrice(t *testing.T) {
	price := 11.0
	barClose := 10.5
	ctx := ToLegacyAnalyzerContext(BuildStockContext(domain.StockBundle{
		Code: "600519", Quote: &domain.Quote{Code: "600519", Source: "test", Price: &price, QuoteTime: "2026-10-20T08:00:00Z", QualityStatus: "valid", AdjustmentBasis: "unknown", VolumeUnit: "shares", AmountUnit: "CNY"},
		DailyBars: []domain.DailyBar{{Date: "2026-10-19", Open: 10, High: 11, Low: 9, Close: barClose, AdjustmentBasis: "unknown"}},
	}, domain.TechnicalSnapshot{}, nil, "2026-10-20T08:00:00Z"))
	if _, ok := ctx["current_price"].(float64); ok {
		t.Fatalf("unknown adjustment basis must not expose a numeric current price: %#v", ctx["current_price"])
	}
	today := ctx["today"].(map[string]any)
	if today["close"] != nil {
		t.Fatalf("unknown adjustment basis close must be null in the LLM payload: %#v", today)
	}
	realtime, ok := ctx["realtime"].(map[string]any)
	if !ok || realtime["price_basis_comparable_to_daily_bars"] != false || realtime["price_omitted_from_cross_basis_comparisons"] != true {
		t.Fatalf("expected explicit price-basis guard: %#v", ctx["realtime"])
	}
	if realtime["price"] != 11.0 {
		t.Fatalf("real-time quote should remain a separate labelled fact: %#v", realtime)
	}
}

func TestLegacyContextOmitsTencentVolumeWhenUnitWasOnlyAssumed(t *testing.T) {
	price, volume, amount := 100.0, float64(10000), float64(1000000)
	quote := &domain.Quote{
		Code: "600519.SH", Price: &price, Volume: &volume, Amount: &amount,
		Source: "tencent", QuoteTime: "2026-10-20T08:00:00Z", QualityStatus: "degraded",
		VolumeUnit: "shares", AmountUnit: "CNY", RawVolumeUnit: "hands_assumed",
		AdjustmentBasis: "none", QualityReasons: []string{"volume_unit_assumed_without_turnover_crosscheck"},
	}
	ctx := ToLegacyAnalyzerContext(domain.AnalysisContext{
		Code: "600519.SH", Quote: quote,
		DailyBars: []domain.DailyBar{{Date: "2026-10-19", Open: 99, High: 101, Low: 98, Close: 100, AdjustmentBasis: "none"}},
	})
	realtime := ctx["realtime"].(map[string]any)
	if _, exists := realtime["volume"]; exists {
		t.Fatalf("volume with assumed source unit must not reach the LLM payload: %#v", realtime)
	}
	if realtime["volume_omitted_due_to_unknown_unit"] != true {
		t.Fatalf("should expose why realtime volume was omitted: %#v", realtime)
	}
	if realtime["amount"] != amount {
		t.Fatalf("amount uses its own CNY contract and should remain available, got %#v", realtime["amount"])
	}
}

func TestTrustedRealtimeVolumeRejectsAssumedUnit(t *testing.T) {
	volume := float64(100000)
	for _, tc := range []struct {
		unit string
		want bool
	}{
		{unit: "hands", want: true},
		{unit: "shares", want: true},
		{unit: "hands_inferred", want: true},
		{unit: "shares_inferred", want: true},
		{unit: "hands_assumed", want: false},
		{unit: "unknown", want: false},
		{unit: "", want: false},
	} {
		q := &domain.Quote{Volume: &volume, VolumeUnit: "shares", RawVolumeUnit: tc.unit}
		if got := trustedRealtimeVolume(q); got != tc.want {
			t.Errorf("trustedRealtimeVolume(raw unit %q)=%v, want %v", tc.unit, got, tc.want)
		}
	}
}

func TestLegacyContextNullsMixedAdjustmentBasisPriceRows(t *testing.T) {
	bars := []domain.DailyBar{
		{Date: "2026-10-15", Open: 10, High: 11, Low: 9, Close: 10.5, PctChg: 1, AdjustmentBasis: "forward_adjusted"},
		{Date: "2026-10-16", Open: 10, High: 12, Low: 9, Close: 11, PctChg: 2, AdjustmentBasis: "none"},
	}
	legacy := ToLegacyAnalyzerContext(domain.AnalysisContext{Code: "600519.SH", DailyBars: bars})
	rows := legacy["daily_data"].([]map[string]any)
	if rows[0]["close"] != nil || rows[0]["pct_chg"] != nil {
		t.Fatalf("price row on a different adjustment basis must be null: %#v", rows[0])
	}
	if rows[1]["close"] != float64(11) {
		t.Fatalf("latest known basis row should remain available: %#v", rows[1])
	}
}

func TestToLegacyAnalyzerContextPassesThroughDragonTigerSummary(t *testing.T) {
	summary := map[string]any{
		"is_on_list": true, "recent_count": 2, "latest_date": "2026-10-26",
		"quality_status": "degraded", "recent_count_is_lower_bound": true,
	}
	ctx := domain.AnalysisContext{Code: "600519.SH", DragonTiger: summary}
	legacy := ToLegacyAnalyzerContext(ctx)
	if legacy["dragon_tiger_context"] != summary || legacy["dragon_tiger"] != summary {
		t.Fatalf("dragon-tiger summary not propagated into legacy analyzer context: %#v", legacy)
	}
}
