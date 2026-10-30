package provider

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type liveGoldenQuote struct {
	Source        string  `json:"source"`
	QuoteTime     string  `json:"quote_time"`
	CurrentPrice  float64 `json:"current_price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	PrevClose     float64 `json:"prev_close"`
	Volume        int64   `json:"volume"`
	VolumeUnit    string  `json:"volume_unit"`
	Amount        float64 `json:"amount"`
	AmountUnit    string  `json:"amount_unit"`
	QualityStatus string  `json:"quality_status"`
}

type liveGoldenBar struct {
	TradeDate string  `json:"trade_date"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

type liveGoldenDailySource struct {
	Source        string          `json:"source"`
	SourceVersion string          `json:"source_version"`
	VolumeUnit    string          `json:"volume_unit"`
	Amount        *float64        `json:"amount"`
	AmountUnit    string          `json:"amount_unit"`
	QualityStatus string          `json:"quality_status"`
	Bars          []liveGoldenBar `json:"bars"`
}

type liveGoldenFixture struct {
	CapturedAt string `json:"captured_at_utc"`
	Commit     string `json:"source_commit"`
	RunID      int64  `json:"workflow_run_id"`
	Quote      struct {
		Symbol  string          `json:"symbol"`
		Sina    liveGoldenQuote `json:"sina"`
		Tencent liveGoldenQuote `json:"tencent"`
	} `json:"quote_comparison"`
	Daily struct {
		Symbol     string                `json:"symbol"`
		Adjustment string                `json:"adjustment_basis"`
		Sina       liveGoldenDailySource `json:"sina"`
		Tencent    liveGoldenDailySource `json:"tencent"`
	} `json:"daily_bars_comparison"`
}

type liveCrossMarketQuote struct {
	Source         string   `json:"source"`
	SourceVersion  string   `json:"source_version"`
	Symbol         string   `json:"symbol"`
	QuoteTime      string   `json:"quote_time"`
	VolumeUnit     string   `json:"volume_unit"`
	AmountUnit     string   `json:"amount_unit"`
	QualityStatus  string   `json:"quality_status"`
	QualityReasons []string `json:"quality_reasons"`
}

type liveCrossMarketDailySource struct {
	Source          string          `json:"source"`
	SourceVersion   string          `json:"source_version"`
	AdjustmentBasis string          `json:"adjustment_basis"`
	QualityStatus   string          `json:"quality_status"`
	VolumeUnit      string          `json:"volume_unit"`
	AmountUnit      string          `json:"amount_unit"`
	QualityReasons  []string        `json:"quality_reasons"`
	Bars            []liveGoldenBar `json:"bars"`
}

type liveCrossMarketSample struct {
	Market    string                       `json:"market"`
	Symbol    string                       `json:"symbol"`
	Quotes    []liveCrossMarketQuote       `json:"quotes"`
	DailyBars []liveCrossMarketDailySource `json:"daily_bars"`
}

type liveCrossMarketFixture struct {
	SchemaVersion int                     `json:"schema_version"`
	WorkflowRunID int64                   `json:"workflow_run_id"`
	SourceCommit  string                  `json:"source_commit"`
	CapturedAt    string                  `json:"captured_at"`
	Runner        string                  `json:"runner"`
	Markets       []liveCrossMarketSample `json:"markets"`
}

func loadLiveGoldenFixture(t *testing.T) liveGoldenFixture {
	t.Helper()
	path := filepath.Join("testdata", "live_market_golden_20261030.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read live golden fixture: %v", err)
	}
	var fixture liveGoldenFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatalf("decode live golden fixture: %v", err)
	}
	if fixture.CapturedAt == "" || fixture.Commit == "" || fixture.RunID == 0 {
		t.Fatal("live golden fixture is missing capture provenance")
	}
	return fixture
}

func assertGoldenNear(t *testing.T, field string, left, right, tolerance float64) {
	t.Helper()
	if math.IsNaN(left) || math.IsInf(left, 0) || math.IsNaN(right) || math.IsInf(right, 0) ||
		math.Abs(left-right) > tolerance {
		t.Fatalf("%s differs: left=%v right=%v tolerance=%v", field, left, right, tolerance)
	}
}

func liveGoldenDailySnapshot(source liveGoldenDailySource, symbol, adjustment string) CrossProviderDailySnapshot {
	bars := make([]CrossProviderDailyBar, 0, len(source.Bars))
	for _, bar := range source.Bars {
		bars = append(bars, CrossProviderDailyBar{
			TradeDate: bar.TradeDate, Open: bar.Open, High: bar.High, Low: bar.Low, Close: bar.Close,
			Volume: bar.Volume, VolumeUnit: source.VolumeUnit, Amount: source.Amount, AmountUnit: source.AmountUnit,
		})
	}
	return CrossProviderDailySnapshot{
		Provider: source.Source, SourceVersion: source.SourceVersion, Symbol: symbol,
		AdjustmentBasis: adjustment, QualityStatus: source.QualityStatus, Bars: bars,
	}
}

func TestLiveGoldenCrossProviderQuoteAndDailyBarsParity(t *testing.T) {
	fixture := loadLiveGoldenFixture(t)

	if fixture.Quote.Symbol != "600519.SH" {
		t.Fatalf("quote fixture symbol=%q", fixture.Quote.Symbol)
	}
	sinaQuote, tencentQuote := fixture.Quote.Sina, fixture.Quote.Tencent
	if sinaQuote.Source != "sina" || tencentQuote.Source != "tencent" ||
		sinaQuote.QuoteTime == "" || tencentQuote.QuoteTime == "" {
		t.Fatalf("quote provenance is incomplete: sina=%+v tencent=%+v", sinaQuote, tencentQuote)
	}
	for _, field := range []struct {
		name        string
		left, right float64
		tolerance   float64
	}{
		{"current_price", sinaQuote.CurrentPrice, tencentQuote.CurrentPrice, 0.01},
		{"change", sinaQuote.Change, tencentQuote.Change, 0.01},
		{"change_percent", sinaQuote.ChangePercent, tencentQuote.ChangePercent, 0.001},
		{"open", sinaQuote.Open, tencentQuote.Open, 0.01},
		{"high", sinaQuote.High, tencentQuote.High, 0.01},
		{"low", sinaQuote.Low, tencentQuote.Low, 0.01},
		{"prev_close", sinaQuote.PrevClose, tencentQuote.PrevClose, 0.01},
		{"amount", sinaQuote.Amount, tencentQuote.Amount, 1},
	} {
		assertGoldenNear(t, "quote."+field.name, field.left, field.right, field.tolerance)
	}
	if sinaQuote.VolumeUnit != "shares" || tencentQuote.VolumeUnit != "shares" {
		t.Fatalf("quote volume units are not normalized: sina=%q tencent=%q", sinaQuote.VolumeUnit, tencentQuote.VolumeUnit)
	}
	if delta := math.Abs(float64(sinaQuote.Volume - tencentQuote.Volume)); delta > 100 {
		t.Fatalf("quote volume difference=%v shares exceeds captured-source tolerance", delta)
	}
	if sinaQuote.QualityStatus != "valid" || tencentQuote.QualityStatus != "degraded" {
		t.Fatalf("quote quality must preserve source-specific confidence: sina=%q tencent=%q", sinaQuote.QualityStatus, tencentQuote.QualityStatus)
	}

	if fixture.Daily.Symbol != "600036.SH" || fixture.Daily.Adjustment != "none" {
		t.Fatalf("daily fixture identity/adjustment mismatch: %+v", fixture.Daily)
	}
	left, right := fixture.Daily.Sina, fixture.Daily.Tencent
	if left.Source != "sina" || right.Source != "tencent" ||
		left.SourceVersion == "" || right.SourceVersion == "" {
		t.Fatalf("daily source provenance is incomplete: sina=%+v tencent=%+v", left, right)
	}
	if left.VolumeUnit != "shares" || right.VolumeUnit != "shares" {
		t.Fatalf("daily volume unit must be explicitly normalized to shares: sina=%q tencent=%q", left.VolumeUnit, right.VolumeUnit)
	}
	if left.Amount != nil || right.Amount != nil || left.AmountUnit != "unknown" || right.AmountUnit != "unknown" {
		t.Fatalf("daily amount must remain null/unknown for both sources: sina=%+v tencent=%+v", left, right)
	}
	if left.QualityStatus != "degraded" || right.QualityStatus != "degraded" {
		t.Fatalf("missing daily amount must remain degraded: sina=%q tencent=%q", left.QualityStatus, right.QualityStatus)
	}
	if len(left.Bars) != 3 || len(right.Bars) != 3 {
		t.Fatalf("daily fixture row counts differ from expected 3: sina=%d tencent=%d", len(left.Bars), len(right.Bars))
	}
	for index := range left.Bars {
		a, b := left.Bars[index], right.Bars[index]
		if a.TradeDate != b.TradeDate {
			t.Fatalf("daily row %d date mismatch: sina=%q tencent=%q", index, a.TradeDate, b.TradeDate)
		}
		for _, field := range []struct {
			name        string
			left, right float64
			tolerance   float64
		}{
			{"open", a.Open, b.Open, 0.001},
			{"high", a.High, b.High, 0.001},
			{"low", a.Low, b.Low, 0.001},
			{"close", a.Close, b.Close, 0.001},
		} {
			assertGoldenNear(t, "daily."+a.TradeDate+"."+field.name, field.left, field.right, field.tolerance)
		}
		if delta := math.Abs(float64(a.Volume - b.Volume)); delta > 50 {
			t.Fatalf("daily %s volume difference=%v shares exceeds captured rounding tolerance", a.TradeDate, delta)
		}
	}
	comparison := CompareCrossProviderDailyBars(
		liveGoldenDailySnapshot(left, fixture.Daily.Symbol, fixture.Daily.Adjustment),
		liveGoldenDailySnapshot(right, fixture.Daily.Symbol, fixture.Daily.Adjustment),
	)
	if comparison.Status != CrossProviderDailyNotComparable || comparison.Reason != "quality_status_must_be_valid" {
		t.Fatalf("degraded live golden fixture must be rejected as a trusted comparison baseline: %+v", comparison)
	}

}


func TestLiveCrossMarketGoldenDailyBarsRemainNotComparableWhenDegraded(t *testing.T) {
	path := filepath.Join("testdata", "live_market_cross_market_golden_20261030.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cross-market golden fixture: %v", err)
	}
	var fixture liveCrossMarketFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatalf("decode cross-market golden fixture: %v", err)
	}
	if fixture.SchemaVersion != 1 || fixture.WorkflowRunID != 41042950111 ||
		fixture.SourceCommit != "720f2fc685376e29452ce3742ba398ff79089a03" ||
		fixture.CapturedAt == "" || len(fixture.Markets) != 2 {
		t.Fatalf("cross-market fixture provenance is incomplete: %+v", fixture)
	}
	for _, market := range fixture.Markets {
		t.Run(market.Market, func(t *testing.T) {
			if market.Symbol == "" || len(market.Quotes) != 2 || len(market.DailyBars) != 2 {
				t.Fatalf("market fixture is incomplete: %+v", market)
			}
			for _, quote := range market.Quotes {
				if quote.Symbol != market.Symbol || quote.Source == "" || quote.SourceVersion == "" ||
					quote.QuoteTime == "" || quote.VolumeUnit != "shares" || quote.AmountUnit != "CNY" {
					t.Fatalf("quote sample identity or units are invalid: %+v", quote)
				}
			}
			left, right := market.DailyBars[0], market.DailyBars[1]
			if left.Source == right.Source || left.SourceVersion == "" || right.SourceVersion == "" ||
				left.AdjustmentBasis != "none" || right.AdjustmentBasis != "none" ||
				left.QualityStatus != "degraded" || right.QualityStatus != "degraded" ||
				left.VolumeUnit != "shares" || right.VolumeUnit != "shares" ||
				left.AmountUnit != "unknown" || right.AmountUnit != "unknown" {
				t.Fatalf("daily-bar sample must preserve source, adjustment and degraded unit evidence: left=%+v right=%+v", left, right)
			}
			leftSnapshot := CrossProviderDailySnapshot{
				Provider: left.Source, SourceVersion: left.SourceVersion, Symbol: market.Symbol,
				AdjustmentBasis: left.AdjustmentBasis, QualityStatus: left.QualityStatus,
				Bars: crossMarketGoldenBars(left.Bars, left.VolumeUnit, left.AmountUnit),
			}
			rightSnapshot := CrossProviderDailySnapshot{
				Provider: right.Source, SourceVersion: right.SourceVersion, Symbol: market.Symbol,
				AdjustmentBasis: right.AdjustmentBasis, QualityStatus: right.QualityStatus,
				Bars: crossMarketGoldenBars(right.Bars, right.VolumeUnit, right.AmountUnit),
			}
			got := CompareCrossProviderDailyBars(leftSnapshot, rightSnapshot)
			if got.Status != CrossProviderDailyNotComparable || got.Reason != "quality_status_must_be_valid" {
				t.Fatalf("degraded cross-market golden bars must not pass comparison: %+v", got)
			}
		})
	}
}

func crossMarketGoldenBars(bars []liveGoldenBar, volumeUnit, amountUnit string) []CrossProviderDailyBar {
	out := make([]CrossProviderDailyBar, 0, len(bars))
	for _, bar := range bars {
		out = append(out, CrossProviderDailyBar{
			TradeDate: bar.TradeDate, Open: bar.Open, High: bar.High, Low: bar.Low,
			Close: bar.Close, Volume: bar.Volume, VolumeUnit: volumeUnit, AmountUnit: amountUnit,
		})
	}
	return out
}
