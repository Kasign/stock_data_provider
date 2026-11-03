package marketdata

import (
	"math"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

func TestNormalizeDailyBarsSortsDeduplicatesAndDropsInvalidRows(t *testing.T) {
	input := []domain.DailyBar{
		{Date: "2026-10-20", Open: 10, High: 12, Low: 9, Close: 11, Volume: 100, Amount: 1000},
		{Date: "2026-10-19", Open: 8, High: 10, Low: 7, Close: 9, Volume: 50, Amount: 500},
		{Date: "2026-10-20", Open: 10, High: 13, Low: 9, Close: 12, Volume: 110, Amount: 1100},
		{Date: "2026-10-21", Open: 10, High: 9, Low: 8, Close: 9, Volume: 100, Amount: 1000},
		{Date: "not-a-date", Open: 1, High: 2, Low: 1, Close: 2},
	}
	got, stats := NormalizeDailyBars(input)
	if len(got) != 2 {
		t.Fatalf("want 2 valid unique bars, got %#v", got)
	}
	if got[0].Date != "2026-10-19" || got[1].Date != "2026-10-20" {
		t.Fatalf("bars not sorted: %#v", got)
	}
	if got[1].Close != 12 {
		t.Fatalf("duplicate date should keep last valid row, got %#v", got[1])
	}
	if stats.DuplicateDates != 1 || stats.DroppedInvalid != 2 {
		t.Fatalf("unexpected normalization stats: %#v", stats)
	}
}

func TestNormalizeDailyBarsRejectsNonFiniteData(t *testing.T) {
	bars, stats := NormalizeDailyBars([]domain.DailyBar{{Date: "2026-10-20", Open: 10, High: 12, Low: 9, Close: 11, Volume: math.NaN()}})
	if len(bars) != 0 || stats.DroppedInvalid != 1 {
		t.Fatalf("expected non-finite volume to be discarded: bars=%#v stats=%#v", bars, stats)
	}
}

func TestNormalizeDailyBarsPreservesExplicitlyMissingOptionalFields(t *testing.T) {
	bars, stats := NormalizeDailyBars([]domain.DailyBar{{
		Date: "2026-10-20", Open: 10, High: 12, Low: 9, Close: 11,
		Volume: 0, Amount: 0, PctChg: 0, MissingFields: []string{"amount", "pct_chg", "unsupported"},
	}})
	if stats.DroppedInvalid != 0 || len(bars) != 1 {
		t.Fatalf("missing optional metrics should not invalidate otherwise usable OHLC: bars=%#v stats=%#v", bars, stats)
	}
	want := []string{"amount", "pct_chg"}
	if len(bars[0].MissingFields) != len(want) || bars[0].MissingFields[0] != want[0] || bars[0].MissingFields[1] != want[1] {
		t.Fatalf("missing-field markers should be sanitized and sorted, got %#v", bars[0].MissingFields)
	}
}

func TestFilterBarsAfterDatePreventsFutureBars(t *testing.T) {
	bars := []domain.DailyBar{{Date: "2026-10-19"}, {Date: "2026-10-20"}, {Date: "2026-10-21"}}
	filtered, dropped := FilterBarsAfterDate(bars, "2026-10-20")
	if dropped != 1 || len(filtered) != 2 || filtered[1].Date != "2026-10-20" {
		t.Fatalf("unexpected cutoff filtering: bars=%#v dropped=%d", filtered, dropped)
	}
}

func TestMarketDateUsesExchangeTimezoneNotStringSuffixHeuristic(t *testing.T) {
	at, err := time.Parse(time.RFC3339, "2026-10-20T23:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if got := MarketDate("600396.SH", at); got != "2026-10-21" {
		t.Fatalf("A-share suffix should use Shanghai time, got %s", got)
	}
	if got := MarketDate("AAPL", at); got != "2026-10-20" {
		t.Fatalf("US ticker should use New York time, got %s", got)
	}
}

func TestMarkLatestCNIntradayBarRequiresLiveSessionAndMatchingDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 10, 30, 0, 0, location)
	bars := []domain.DailyBar{{Date: "2026-10-20"}}
	if !MarkLatestCNIntradayBar(bars, "600396", asOf, asOf.Add(2*time.Second)) || !bars[0].IsPartial {
		t.Fatalf("current session's latest bar should be marked partial: %#v", bars)
	}
	past := asOf.Add(-24 * time.Hour)
	bars[0].IsPartial = false
	if MarkLatestCNIntradayBar(bars, "600396", past, asOf) || bars[0].IsPartial {
		t.Fatalf("historical reference must not be marked as a live partial bar: %#v", bars)
	}
}
