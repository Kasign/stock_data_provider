package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildDailyBarsCloseSeriesInputPreservesExactCloseAndPITEvidence(t *testing.T) {
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	query := DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-11-17", EndDate: "2026-11-18",
		Limit: 2, AdjustmentBasis: AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	snapshot := DailyBarsV2Snapshot{
		Symbol: "600036.SH", Interval: "1d", AdjustmentBasis: AdjustmentNone,
		QualityStatus: QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: closeSeriesStringPointer("data_calendar:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		CoverageEvidence: DailyBarCoverageEvidence{
			CalendarQualityStatus: QualityValid, ExpectedTradingDays: 2, MinimumRequiredBars: 2,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: []DailyBar{
			closeSeriesTestBar("2026-11-17", "43.49", "43.4900", 4, cutoff, observed),
			closeSeriesTestBar("2026-11-18", "43.12", "43.120", 3, cutoff, observed),
		},
	}

	got, err := BuildDailyBarsCloseSeriesInput(query, snapshot)
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesInput() error = %v", err)
	}
	if got.Payload.SchemaVersion != "1.0.0" || got.Payload.PayloadKind != "daily_bars_close_series" {
		t.Fatalf("unexpected payload identity: %+v", got.Payload)
	}
	if got.Payload.Bars[0].Close != "43.49" || got.Payload.Bars[0].CloseEvidence.SourceToken != "43.4900" {
		t.Fatalf("exact Close evidence was not preserved: %+v", got.Payload.Bars[0])
	}
	if got.Payload.Bars[0].CloseEvidence.SourceFractionDigits == nil || *got.Payload.Bars[0].CloseEvidence.SourceFractionDigits != 4 {
		t.Fatalf("source fraction digits were not preserved: %+v", got.Payload.Bars[0].CloseEvidence)
	}
	if got.Payload.Bars[0].FetchedAt[len(got.Payload.Bars[0].FetchedAt)-1:] != "Z" {
		t.Fatalf("fetched_at is not canonical UTC: %q", got.Payload.Bars[0].FetchedAt)
	}
	encoded, err := json.Marshal(got.Payload)
	if err != nil {
		t.Fatalf("json.Marshal(payload) error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	coverage, ok := document["coverage_evidence"].(map[string]any)
	if !ok || coverage["calendar_quality_status"] != "valid" {
		t.Fatalf("coverage evidence JSON keys do not match the contract: %s", encoded)
	}
	bars, ok := document["bars"].([]any)
	if !ok || len(bars) == 0 {
		t.Fatalf("bars JSON array missing: %s", encoded)
	}
	firstBar, ok := bars[0].(map[string]any)
	if !ok {
		t.Fatalf("first bar JSON object missing: %s", encoded)
	}
	closeEvidence, ok := firstBar["close_evidence"].(map[string]any)
	if !ok || closeEvidence["source_fraction_digits"] != float64(4) {
		t.Fatalf("Close evidence JSON keys do not match the contract: %s", encoded)
	}
	if len(got.Sources) != 1 || got.Sources[0].SourceVersion != nil ||
		got.Sources[0].ProviderInterfaceVersion != dailyBarsCloseSeriesEastmoneyAPIVersion {
		t.Fatalf("provider data version was fabricated or interface version was lost: %+v", got.Sources)
	}
	if !got.SystemAsOf.Equal(cutoff) {
		t.Fatalf("SystemAsOf = %s, want %s", got.SystemAsOf, cutoff)
	}
}

func TestBuildDailyBarsCloseSeriesInputRejectsUntrustedInputs(t *testing.T) {
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	baseQuery := DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-11-17", EndDate: "2026-11-17",
		Limit: 1, AdjustmentBasis: AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	baseSnapshot := DailyBarsV2Snapshot{
		Symbol: "600036.SH", Interval: "1d", AdjustmentBasis: AdjustmentNone,
		QualityStatus: QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: closeSeriesStringPointer("data_calendar:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		CoverageEvidence: DailyBarCoverageEvidence{
			CalendarQualityStatus: QualityValid, ExpectedTradingDays: 1, MinimumRequiredBars: 1,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: []DailyBar{closeSeriesTestBar("2026-11-17", "43.49", "43.49", 2, cutoff, observed)},
	}
	tests := []struct {
		name   string
		mutate func(*DailyBarsV2Snapshot, *DailyBarsQuery)
	}{
		{name: "missing calendar ref", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.TradingCalendarRef = nil }},
		{name: "degraded calendar", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) {
			s.CoverageEvidence.CalendarQualityStatus = QualityDegraded
		}},
		{name: "degraded aggregate quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.QualityStatus = QualityDegraded }},
		{name: "stale aggregate quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.QualityStatus = QualityStale }},
		{name: "invalid aggregate quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.QualityStatus = QualityInvalid }},
		{name: "unknown aggregate quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.QualityStatus = QualityUnknown }},
		{name: "degraded bar quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars[0].QualityStatus = QualityDegraded }},
		{name: "stale bar quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars[0].QualityStatus = QualityStale }},
		{name: "invalid bar quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars[0].QualityStatus = QualityInvalid }},
		{name: "unknown bar quality", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars[0].QualityStatus = QualityUnknown }},
		{name: "insufficient coverage", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.CoverageEvidence.CoverageSufficient = false }},
		{name: "float-only close", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars[0].ExactValues = nil }},
		{name: "source version is not mapped", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars[0].SourceVersion = "eastmoney-stock-api-v2" }},
		{name: "source token differs from exact close", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) {
			s.Bars[0].ExactValues.CloseEvidence.SourceToken = "43.50"
		}},
		{name: "query is not PIT", mutate: func(_ *DailyBarsV2Snapshot, q *DailyBarsQuery) { q.SystemObservedAtCutoff = nil }},
		{name: "duplicate trade date", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Bars = append(s.Bars, s.Bars[0]) }},
		{name: "empty warning is rejected", mutate: func(s *DailyBarsV2Snapshot, _ *DailyBarsQuery) { s.Warnings = []string{"valid", " "} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := baseQuery
			snapshot := baseSnapshot
			snapshot.Bars = append([]DailyBar(nil), baseSnapshot.Bars...)
			if snapshot.Bars[0].ExactValues != nil {
				exact := *snapshot.Bars[0].ExactValues
				if exact.CloseEvidence != nil {
					evidence := *exact.CloseEvidence
					exact.CloseEvidence = &evidence
				}
				snapshot.Bars[0].ExactValues = &exact
			}
			test.mutate(&snapshot, &query)
			if _, err := BuildDailyBarsCloseSeriesInput(query, snapshot); err == nil {
				t.Fatal("BuildDailyBarsCloseSeriesInput() error = nil, want rejection")
			}
		})
	}
}

func closeSeriesTestBar(tradeDate, closeValue, sourceToken string, fractionDigits int, cutoff, observed time.Time) DailyBar {
	fraction := fractionDigits
	return DailyBar{
		Symbol: "600036.SH", TradeDate: tradeDate, Period: "1d",
		AdjustmentBasis: AdjustmentNone,
		Source:          dailyBarsCloseSeriesSourceEastmoney, SourceVersion: dailyBarsCloseSeriesEastmoneyAPIVersion,
		FetchedAt: cutoff.Add(-2 * time.Minute), NormalizedAt: cutoff.Add(-time.Minute),
		SystemObservedAt: &observed, NormalizationVersion: "4",
		QualityStatus: QualityValid,
		ExactValues: &DailyBarExactValues{
			Close: closeValue,
			CloseEvidence: &DailyBarDecimalEvidence{
				EvidenceKind: "observed_token", SourceToken: sourceToken, SourceFractionDigits: &fraction,
			},
		},
	}
}

func closeSeriesStringPointer(value string) *string { return &value }

func TestBuildDailyBarsCloseSeriesInputRejectsMalformedDecimalEvidence(t *testing.T) {
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	query := DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-11-17", EndDate: "2026-11-17",
		Limit: 1, AdjustmentBasis: AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	snapshot := DailyBarsV2Snapshot{
		Symbol: "600036.SH", Interval: "1d", AdjustmentBasis: AdjustmentNone,
		QualityStatus: QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: closeSeriesStringPointer("calendar-ref"),
		CoverageEvidence: DailyBarCoverageEvidence{
			CalendarQualityStatus: QualityValid, ExpectedTradingDays: 1, MinimumRequiredBars: 1,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: []DailyBar{closeSeriesTestBar("2026-11-17", "43.49", "43.490", 4, cutoff, observed)},
	}
	_, err := BuildDailyBarsCloseSeriesInput(query, snapshot)
	if err == nil || !strings.Contains(err.Error(), "source fraction digits") {
		t.Fatalf("error = %v, want malformed source decimal rejection", err)
	}
}
