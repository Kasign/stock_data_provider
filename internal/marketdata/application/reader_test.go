package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/service"
)

type fakeDataService struct {
	quote                    *model.Quote
	quoteErr                 error
	currentBars              []model.DailyBar
	currentWarnings          []string
	currentCoverage          service.DailyBarCoverageEvidence
	currentErr               error
	pitBars                  []model.DailyBar
	pitWarnings              []string
	pitCoverage              service.DailyBarCoverageEvidence
	pitErr                   error
	fundamentals             *model.FundamentalsSnapshot
	fundamentalsErr          error
	valuations               *model.ValuationSnapshot
	valuationsErr            error
	currentCalls             int
	pitCalls                 int
	currentFundamentalsCalls int
	pitFundamentalsCalls     int
	currentValuationsCalls   int
	pitValuationsCalls       int
	lastAsOf                 time.Time
	lastFundamentalsAsOf     time.Time
	lastValuationsAsOf       time.Time
}

func (f *fakeDataService) Quote(context.Context, string) (*model.Quote, bool, error) {
	return f.quote, false, f.quoteErr
}

func (f *fakeDataService) DailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, bool, []string, error) {
	f.currentCalls++
	return f.currentBars, false, f.currentWarnings, f.currentErr
}

func (f *fakeDataService) DailyBarsAsOf(_ context.Context, _, _, _ string, _ int, _ model.AdjustmentBasis, asOf time.Time) ([]model.DailyBar, []string, error) {
	f.pitCalls++
	f.lastAsOf = asOf
	return f.pitBars, f.pitWarnings, f.pitErr
}

func (f *fakeDataService) DailyBarsWithCoverageEvidence(ctx context.Context, symbol, start, end string, limit int, basis model.AdjustmentBasis) ([]model.DailyBar, bool, []string, service.DailyBarCoverageEvidence, error) {
	bars, cacheHit, warnings, err := f.DailyBars(ctx, symbol, start, end, limit, basis)
	return bars, cacheHit, warnings, f.currentCoverage, err
}

func (f *fakeDataService) DailyBarsAsOfWithCoverageEvidence(ctx context.Context, symbol, start, end string, limit int, basis model.AdjustmentBasis, asOf time.Time) ([]model.DailyBar, []string, service.DailyBarCoverageEvidence, error) {
	bars, warnings, err := f.DailyBarsAsOf(ctx, symbol, start, end, limit, basis, asOf)
	return bars, warnings, f.pitCoverage, err
}

func (f *fakeDataService) Fundamentals(context.Context, string, int, int) (*model.FundamentalsSnapshot, bool, error) {
	f.currentFundamentalsCalls++
	return f.fundamentals, false, f.fundamentalsErr
}

func (f *fakeDataService) FundamentalsAsOf(_ context.Context, _ string, _, _ int, asOf time.Time) (*model.FundamentalsSnapshot, error) {
	f.pitFundamentalsCalls++
	f.lastFundamentalsAsOf = asOf
	return f.fundamentals, f.fundamentalsErr
}

func (f *fakeDataService) Valuations(context.Context, []string) (*model.ValuationSnapshot, bool, error) {
	f.currentValuationsCalls++
	return f.valuations, false, f.valuationsErr
}

func (f *fakeDataService) ValuationsAsOf(_ context.Context, _ []string, asOf time.Time) (*model.ValuationSnapshot, error) {
	f.pitValuationsCalls++
	f.lastValuationsAsOf = asOf
	return f.valuations, f.valuationsErr
}

func TestReaderGetDailyBarsV2PreservesSameEvaluationCalendarRef(t *testing.T) {
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	observedAt := now.Add(time.Minute)
	closeValue := 10.5
	bar := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-11-06", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: closeValue,
		Source: "fixture", SourceVersion: "fixture-v1", FetchedAt: now,
		NormalizedAt: now, SystemObservedAt: &observedAt,
		QualityStatus: model.QualityValid, NormalizationVer: "norm-v1",
	}
	currentCoverage := service.DailyBarCoverageEvidence{
		CalendarRef: "sha256:calendar-current", CalendarQualityStatus: model.QualityValid,
		ExpectedTradingDays: 20, MinimumRequiredBars: 13, CoverageBasis: "exchange_calendar",
		CoverageSufficient: true,
	}
	pitCoverage := service.DailyBarCoverageEvidence{
		CalendarRef: "sha256:calendar-asof", CalendarQualityStatus: model.QualityValid,
		ExpectedTradingDays: 18, MinimumRequiredBars: 12, CoverageBasis: "exchange_calendar",
		CoverageSufficient: true, Warnings: []string{"calendar_fixture_note"},
	}
	backend := &fakeDataService{
		currentBars: []model.DailyBar{bar}, pitBars: []model.DailyBar{bar},
		currentCoverage: currentCoverage, pitCoverage: pitCoverage,
	}
	reader := &Reader{backend: backend}
	current, err := reader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06", Limit: 20,
		AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(current) error = %v", err)
	}
	if current.Interval != "1d" || current.TradingCalendarRef == nil || *current.TradingCalendarRef != currentCoverage.CalendarRef ||
		current.CoverageEvidence.ExpectedTradingDays != 20 || current.CoverageEvidence.MinimumRequiredBars != 13 ||
		current.CoverageEvidence.CoverageBasis != "exchange_calendar" || !current.CoverageEvidence.CoverageSufficient {
		t.Fatalf("current v2 snapshot did not preserve coverage calendar evidence: %+v", current)
	}
	cutoff := now.Add(2 * time.Hour)
	pit, err := reader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06", Limit: 20,
		AdjustmentBasis: app.AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(PIT) error = %v", err)
	}
	if pit.TradingCalendarRef == nil || *pit.TradingCalendarRef != pitCoverage.CalendarRef ||
		pit.CoverageEvidence.CalendarQualityStatus != app.QualityValid ||
		len(pit.CoverageEvidence.Warnings) != 1 ||
		pit.Warnings[0] != "calendar_fixture_note" {
		t.Fatalf("PIT v2 snapshot did not preserve as-of calendar evidence: %+v", pit)
	}
	if backend.currentCalls != 1 || backend.pitCalls != 1 || !backend.lastAsOf.Equal(cutoff) {
		t.Fatalf("v2 current/PIT backend calls or cutoff mismatch: current=%d PIT=%d asOf=%s", backend.currentCalls, backend.pitCalls, backend.lastAsOf)
	}

	degradedBackend := &fakeDataService{
		currentBars: []model.DailyBar{bar},
		currentCoverage: service.DailyBarCoverageEvidence{
			CalendarRef: "sha256:calendar-conflicted", CalendarQualityStatus: model.QualityDegraded,
			ExpectedTradingDays: 20, MinimumRequiredBars: 13, CoverageBasis: "exchange_calendar",
			CoverageSufficient: true, Warnings: []string{"daily_bar_coverage_uses_degraded_trading_calendar"},
		},
	}
	degradedReader := &Reader{backend: degradedBackend}
	degraded, err := degradedReader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06", Limit: 20,
		AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(degraded calendar) error = %v", err)
	}
	if degraded.TradingCalendarRef != nil || degraded.CoverageEvidence.CoverageBasis != "degraded_exchange_calendar" ||
		degraded.QualityStatus != app.QualityDegraded ||
		!containsReaderString(degraded.Warnings, "daily_bar_coverage_calendar_quality_unreliable") ||
		degraded.Bars[0].QualityStatus != app.QualityDegraded {
		t.Fatalf("v2 must not claim a reliable CalendarRef for a conflicted calendar: %+v", degraded)
	}
	*pit.TradingCalendarRef = "mutated"
	pit.CoverageEvidence.Warnings[0] = "mutated"
	if backend.pitCoverage.CalendarRef == "mutated" || backend.pitCoverage.Warnings[0] == "mutated" {
		t.Fatal("v2 snapshot must not alias backend calendar evidence")
	}

	unreferencedBackend := &fakeDataService{
		currentBars: []model.DailyBar{bar},
		currentCoverage: service.DailyBarCoverageEvidence{
			CalendarQualityStatus: model.QualityValid, ExpectedTradingDays: 20,
			MinimumRequiredBars: 13, CoverageBasis: "exchange_calendar_unreferenced",
			CoverageSufficient: true,
		},
	}
	unreferencedReader := &Reader{backend: unreferencedBackend}
	unreferenced, err := unreferencedReader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06", Limit: 20,
		AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(unreferenced calendar) error = %v", err)
	}
	if unreferenced.TradingCalendarRef != nil || unreferenced.QualityStatus != app.QualityDegraded ||
		len(unreferenced.Warnings) != 1 ||
		unreferenced.Warnings[0] != "daily_bar_coverage_calendar_reference_unavailable" ||
		unreferenced.Bars[0].QualityStatus != app.QualityDegraded {
		t.Fatalf("v2 must explicitly degrade a calendar without a durable ref: %+v", unreferenced)
	}

	unboundedBackend := &fakeDataService{
		currentBars: []model.DailyBar{bar},
		currentCoverage: service.DailyBarCoverageEvidence{
			CalendarRef: "sha256:calendar-not-accepted", CalendarQualityStatus: model.QualityValid,
			ExpectedTradingDays: 20, MinimumRequiredBars: 13,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
	}
	unboundedReader := &Reader{backend: unboundedBackend}
	unbounded, err := unboundedReader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", Limit: 20, AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(unbounded range) error = %v", err)
	}
	if unbounded.TradingCalendarRef != nil || unbounded.CoverageEvidence.CoverageBasis != "unbounded_range_estimate" ||
		unbounded.QualityStatus != app.QualityDegraded ||
		len(unbounded.Warnings) != 1 ||
		unbounded.Warnings[0] != "daily_bar_coverage_explicit_range_required_for_calendar_ref" {
		t.Fatalf("v2 must not expose a calendar ref for an unbounded query: %+v", unbounded)
	}
}

func TestReaderGetDailyBarsV2RejectsDegradedCalendarStatusWithoutWarning(t *testing.T) {
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	bar := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-11-06", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Source: "fixture", SourceVersion: "fixture-v1", FetchedAt: now,
		NormalizedAt: now, QualityStatus: model.QualityValid,
	}
	backend := &fakeDataService{
		currentBars: []model.DailyBar{bar},
		currentCoverage: service.DailyBarCoverageEvidence{
			CalendarRef:           "sha256:calendar-degraded",
			CalendarQualityStatus: model.QualityDegraded,
			CoverageBasis:         "exchange_calendar",
			CoverageSufficient:    true,
		},
	}
	reader := &Reader{backend: backend}
	got, err := reader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06",
		Limit: 20, AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(degraded calendar status) error = %v", err)
	}
	if got.TradingCalendarRef != nil || got.QualityStatus != app.QualityDegraded ||
		got.CoverageEvidence.CalendarQualityStatus != app.QualityDegraded ||
		got.CoverageEvidence.CoverageBasis != "degraded_exchange_calendar" ||
		!containsReaderString(got.Warnings, "daily_bar_coverage_calendar_quality_unreliable") {
		t.Fatalf("V2 must distrust a degraded calendar even when its warning list is empty: %+v", got)
	}
}

func TestReaderGetDailyBarsV2DegradesBarsWhenCalendarIsOnlyWeekdayEstimate(t *testing.T) {
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	bar := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-11-06", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Source: "fixture", SourceVersion: "fixture-v1", FetchedAt: now,
		NormalizedAt: now, QualityStatus: model.QualityValid,
	}
	backend := &fakeDataService{
		currentBars: []model.DailyBar{bar},
		currentCoverage: service.DailyBarCoverageEvidence{
			CalendarQualityStatus: model.QualityDegraded,
			CoverageBasis:         "weekday_estimate", CoverageSufficient: true,
			Warnings: []string{"daily_bar_coverage_calendar_not_observed_by_as_of"},
		},
	}
	reader := &Reader{backend: backend}
	got, err := reader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06",
		Limit: 20, AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(weekday estimate) error = %v", err)
	}
	if got.TradingCalendarRef != nil || got.CoverageEvidence.CoverageBasis != "weekday_estimate" ||
		got.QualityStatus != app.QualityDegraded || len(got.Bars) != 1 ||
		got.Bars[0].QualityStatus != app.QualityDegraded ||
		!containsReaderString(got.Warnings, "daily_bar_coverage_calendar_reference_unavailable") {
		t.Fatalf("V2 must degrade each bar when no observed exchange-calendar reference exists: %+v", got)
	}
}

func TestReaderGetDailyBarsV2DegradesBarsWhenExchangeCalendarReferenceIsMissing(t *testing.T) {
	now := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	bar := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-11-06", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Source: "fixture", SourceVersion: "fixture-v1", FetchedAt: now,
		NormalizedAt: now, QualityStatus: model.QualityValid,
	}
	backend := &fakeDataService{
		currentBars: []model.DailyBar{bar},
		currentCoverage: service.DailyBarCoverageEvidence{
			CalendarQualityStatus: model.QualityValid,
			CoverageBasis:         "exchange_calendar",
			CoverageSufficient:    true,
		},
	}
	reader := &Reader{backend: backend}
	got, err := reader.GetDailyBarsV2(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-06",
		Limit: 20, AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBarsV2(missing calendar ref) error = %v", err)
	}
	if got.TradingCalendarRef != nil || got.QualityStatus != app.QualityDegraded ||
		got.CoverageEvidence.CalendarQualityStatus != app.QualityDegraded ||
		len(got.Bars) != 1 || got.Bars[0].QualityStatus != app.QualityDegraded ||
		!containsReaderString(got.Warnings, "daily_bar_coverage_calendar_reference_unavailable") {
		t.Fatalf("V2 must not trust an exchange-calendar label without its persisted reference: %+v", got)
	}
}

func TestReaderGetQuotePreservesNullablePrecisionAndMetadata(t *testing.T) {
	price := 12.34
	volume := int64(9007199254740993)
	quoteTime := time.Date(2026, 11, 9, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	rawVolume := 123.5
	source := &model.Quote{
		Symbol: "600000.SH", Name: "浦发银行", CurrentPrice: &price, Volume: &volume,
		QuoteTime: &quoteTime, QuoteTimeSemantics: "provider_snapshot_time",
		Source: "eastmoney", SourceVersion: "fixture-v1", FetchedAt: quoteTime.Add(time.Second),
		NormalizedAt: quoteTime.Add(2 * time.Second), RawVolume: &rawVolume, RawVolumeUnit: "手",
		VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"amount_missing_from_provider"},
		FallbackFrom: "tushare", FallbackReason: "primary provider timeout",
		RequestID: "req-123", NormalizationVer: "norm-v1",
	}
	reader := &Reader{backend: &fakeDataService{quote: source}}

	got, err := reader.GetQuote(context.Background(), "600000.SH")
	if err != nil {
		t.Fatalf("GetQuote() error = %v", err)
	}
	if got.CurrentPrice == nil || *got.CurrentPrice != price {
		t.Fatalf("CurrentPrice = %v, want %v", got.CurrentPrice, price)
	}
	if got.Volume == nil || *got.Volume != volume {
		t.Fatalf("Volume = %v, want exact int64 %d", got.Volume, volume)
	}
	if got.Change != nil || got.Amount != nil {
		t.Fatalf("nullable values were changed: Change=%v Amount=%v", got.Change, got.Amount)
	}
	if got.QuoteTime == nil || !got.QuoteTime.Equal(quoteTime) || got.QuoteTimeSemantics != "provider_snapshot_time" {
		t.Fatalf("provider quote time metadata was not preserved: %+v", got)
	}
	if got.Source != "eastmoney" || got.SourceVersion != "fixture-v1" || got.NormalizationVersion != "norm-v1" {
		t.Fatalf("source/version metadata was not preserved: %+v", got)
	}
	if got.QualityStatus != app.QualityDegraded || len(got.QualityReasons) != 1 || got.FallbackFrom != "tushare" || got.RequestID != "req-123" {
		t.Fatalf("quality/fallback/request metadata was not preserved: %+v", got)
	}

	*got.Volume = 1
	got.QualityReasons[0] = "mutated"
	if *source.Volume != volume || source.QualityReasons[0] != "amount_missing_from_provider" {
		t.Fatal("Application DTO must not alias mutable pointers or slices owned by DataService")
	}
}

func TestReaderGetDailyBarsSelectsCurrentAndPITUseCases(t *testing.T) {
	providerTime := time.Date(2026, 11, 6, 7, 0, 0, 0, time.UTC)
	systemObservedAt := providerTime.Add(time.Minute)
	factorObservedAt := providerTime.Add(2 * time.Minute)
	volume := int64(1234567890123)
	amount := 987654.32
	changePercent := 1.25
	ma5, ma20, ma120, ma250 := 10.5, 10.2, 9.8, 8.7
	rawVolume, rawAmount := 1234567.89, 987654.32
	exactRawAmount, exactAmountCNY, exactChangePercent := "987654.321000", "987654.321000", "1.2300"
	exactScale := 6
	exactOpenEvidence := &model.DailyBarDecimalEvidence{
		EvidenceKind: model.DailyBarPrecisionEvidenceObservedToken,
		SourceToken:  "10.500400", SourceFractionDigits: &exactScale,
	}
	exactRawAmountEvidence := &model.DailyBarDecimalEvidence{
		EvidenceKind: model.DailyBarPrecisionEvidenceObservedToken,
		SourceToken:  exactRawAmount, SourceFractionDigits: &exactScale,
	}
	exactChangePercentScale := 4
	exactChangePercentEvidence := &model.DailyBarDecimalEvidence{
		EvidenceKind: model.DailyBarPrecisionEvidenceObservedToken,
		SourceToken:  exactChangePercent, SourceFractionDigits: &exactChangePercentScale,
	}
	current := model.DailyBar{
		Symbol:                                "600000.SH",
		TradeDate:                             "2026-11-06",
		IsPartial:                             true,
		Period:                                "1d",
		Open:                                  10,
		High:                                  11,
		Low:                                   9,
		Close:                                 10.5,
		MA5:                                   &ma5,
		MA20:                                  &ma20,
		MA120:                                 &ma120,
		MA250:                                 &ma250,
		Volume:                                &volume,
		Amount:                                &amount,
		ChangePercent:                         &changePercent,
		AdjustmentBasis:                       model.AdjustmentForward,
		AdjustmentMethod:                      "provider_forward",
		AdjustmentRefDate:                     "2026-11-06",
		AdjustmentFactorSource:                "eastmoney",
		AdjustmentFactorSourceVersion:         "factor-v2",
		AdjustmentFactorRevisionHash:          "factor-revision-v2",
		AdjustmentFactorSetHash:               "factor-set-v2",
		AdjustmentFactorReferenceRevisionHash: "reference-revision-v2",
		AdjustmentAlgorithmVersion:            "cumulative_adj_factor_v1",
		AdjustmentFactorSystemObservedAt:      &factorObservedAt,
		Source:                                "eastmoney",
		SourceVersion:                         "bars-v3",
		ProviderTime:                          &providerTime,
		FetchedAt:                             providerTime.Add(3 * time.Minute),
		NormalizedAt:                          providerTime.Add(4 * time.Minute),
		SystemObservedAt:                      &systemObservedAt,
		RawVolume:                             &rawVolume,
		RawVolumeUnit:                         "手",
		RawAmount:                             &rawAmount,
		RawAmountUnit:                         "CNY",
		RawAmountField:                        "amount",
		VolumeUnit:                            "shares",
		AmountUnit:                            "CNY",
		QualityStatus:                         model.QualityValid,
		QualityReasons:                        []string{"source_contract_verified"},
		FallbackFrom:                          "tushare",
		FallbackReason:                        "primary_timeout",
		RequestID:                             "req-bars",
		NormalizationVer:                      "norm-v1",
		ExactValues: &model.DailyBarExactValues{
			Open: "10.0001", High: "11.0002", Low: "9.0003", Close: "10.5004",
			RawVolume: "12345", RawAmount: &exactRawAmount, AmountCNY: &exactAmountCNY,
			OpenEvidence: exactOpenEvidence, RawAmountEvidence: exactRawAmountEvidence,
			AmountCNYEvidence: exactRawAmountEvidence, ChangePercent: &exactChangePercent,
			ChangePercentSourceField: "pct_chg",
			ChangePercentEvidence:    exactChangePercentEvidence,
		},
	}
	backend := &fakeDataService{
		currentBars:     []model.DailyBar{current},
		currentWarnings: []string{"bars_truncated_to_limit"},
		pitBars:         []model.DailyBar{current},
		pitWarnings:     []string{"daily_bars_reconstructed_from_recorded_revisions"},
	}
	reader := &Reader{backend: backend}

	live, err := reader.GetDailyBars(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", Limit: 120, AdjustmentBasis: app.AdjustmentForward,
	})
	if err != nil {
		t.Fatalf("GetDailyBars(current) error = %v", err)
	}
	if backend.currentCalls != 1 || backend.pitCalls != 0 {
		t.Fatalf("current/PIT calls = %d/%d, want 1/0", backend.currentCalls, backend.pitCalls)
	}
	if live.QualityStatus != app.QualityValid || live.Bars[0].VolumeUnit != "shares" || live.Bars[0].AmountUnit != "CNY" {
		t.Fatalf("current snapshot metadata mismatch: %+v", live)
	}
	if len(live.Bars) != 1 || live.Bars[0].AdjustmentReferenceDate != "2026-11-06" ||
		live.Bars[0].AdjustmentBasis != app.AdjustmentForward ||
		live.Bars[0].AdjustmentMethod != "provider_forward" ||
		live.Bars[0].AdjustmentFactorSource != "eastmoney" ||
		live.Bars[0].AdjustmentFactorSourceVersion != "factor-v2" ||
		live.Bars[0].AdjustmentFactorRevisionHash != "factor-revision-v2" ||
		live.Bars[0].AdjustmentFactorSetHash != "factor-set-v2" ||
		live.Bars[0].AdjustmentFactorReferenceRevisionHash != "reference-revision-v2" ||
		live.Bars[0].AdjustmentAlgorithmVersion != "cumulative_adj_factor_v1" ||
		live.Bars[0].NormalizationVersion != "norm-v1" ||
		!live.Bars[0].IsPartial || live.Bars[0].ChangePercent == nil || *live.Bars[0].ChangePercent != changePercent ||
		live.Bars[0].MA20 == nil || *live.Bars[0].MA20 != ma20 ||
		live.Bars[0].MA120 == nil || *live.Bars[0].MA120 != ma120 ||
		live.Bars[0].MA250 == nil || *live.Bars[0].MA250 != ma250 ||
		live.Bars[0].RawVolume == nil || *live.Bars[0].RawVolume != rawVolume ||
		live.Bars[0].RawVolumeUnit != "手" ||
		live.Bars[0].RawAmount == nil || *live.Bars[0].RawAmount != rawAmount ||
		live.Bars[0].RawAmountUnit != "CNY" || live.Bars[0].RawAmountField != "amount" ||
		live.Bars[0].FallbackFrom != "tushare" || live.Bars[0].FallbackReason != "primary_timeout" ||
		live.Bars[0].RequestID != "req-bars" || len(live.Bars[0].QualityReasons) != 1 {
		t.Fatalf("daily-bar provenance/nullable metadata was not preserved: %+v", live.Bars)
	}
	if live.Bars[0].ExactValues == nil || live.Bars[0].ExactValues.Close != "10.5004" ||
		live.Bars[0].ExactValues.RawVolume != "12345" || live.Bars[0].ExactValues.RawAmount == nil ||
		*live.Bars[0].ExactValues.RawAmount != exactRawAmount || live.Bars[0].ExactValues.AmountCNY == nil ||
		*live.Bars[0].ExactValues.AmountCNY != exactAmountCNY ||
		live.Bars[0].ExactValues.OpenEvidence == nil ||
		live.Bars[0].ExactValues.OpenEvidence.SourceToken != "10.500400" ||
		live.Bars[0].ExactValues.OpenEvidence.SourceFractionDigits == nil ||
		*live.Bars[0].ExactValues.OpenEvidence.SourceFractionDigits != exactScale ||
		live.Bars[0].ExactValues.AmountCNYEvidence == nil ||
		live.Bars[0].ExactValues.AmountCNYEvidence.SourceToken != exactRawAmount ||
		live.Bars[0].ExactValues.ChangePercent == nil ||
		*live.Bars[0].ExactValues.ChangePercent != exactChangePercent ||
		live.Bars[0].ExactValues.ChangePercentSourceField != "pct_chg" ||
		live.Bars[0].ExactValues.ChangePercentEvidence == nil ||
		live.Bars[0].ExactValues.ChangePercentEvidence.SourceToken != exactChangePercent {
		t.Fatalf("exact decimal sidecar was not preserved through Application Port: %+v", live.Bars[0].ExactValues)
	}
	if live.Bars[0].ProviderTime == nil || !live.Bars[0].ProviderTime.Equal(providerTime) ||
		live.Bars[0].SystemObservedAt == nil || !live.Bars[0].SystemObservedAt.Equal(systemObservedAt) {
		t.Fatalf("provider/system timestamps were not preserved: %+v", live.Bars[0])
	}
	*live.Bars[0].Volume = 1
	*live.Bars[0].ExactValues.RawAmount = "mutated"
	*live.Bars[0].ExactValues.ChangePercent = "mutated"
	live.Bars[0].ExactValues.OpenEvidence.SourceToken = "mutated"
	*live.Bars[0].ExactValues.OpenEvidence.SourceFractionDigits = 1
	live.Bars[0].QualityReasons[0] = "mutated"
	live.Warnings[0] = "mutated"
	if *current.Volume != volume || current.ExactValues == nil || *current.ExactValues.RawAmount != exactRawAmount ||
		current.QualityReasons[0] != "source_contract_verified" ||
		current.ExactValues.OpenEvidence.SourceToken != "10.500400" ||
		*current.ExactValues.ChangePercent != exactChangePercent ||
		current.ExactValues.ChangePercentSourceField != "pct_chg" ||
		*current.ExactValues.OpenEvidence.SourceFractionDigits != exactScale ||
		backend.currentWarnings[0] != "bars_truncated_to_limit" {
		t.Fatal("Application DTO must not alias DataService daily-bar pointers or slices")
	}

	cutoff := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	pit, err := reader.GetDailyBars(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", Limit: 120, AdjustmentBasis: app.AdjustmentForward,
		SystemObservedAtCutoff: &cutoff,
	})
	if err != nil {
		t.Fatalf("GetDailyBars(PIT) error = %v", err)
	}
	if backend.currentCalls != 1 || backend.pitCalls != 1 || !backend.lastAsOf.Equal(cutoff) {
		t.Fatalf("PIT cutoff was not forwarded exactly: current=%d PIT=%d as_of=%s", backend.currentCalls, backend.pitCalls, backend.lastAsOf)
	}
	if pit.SystemObservedAtCutoff == nil || !pit.SystemObservedAtCutoff.Equal(cutoff) {
		t.Fatalf("PIT cutoff was not echoed in the Application DTO: %+v", pit)
	}
	if len(pit.Bars) != 1 || pit.Bars[0].ExactValues == nil || pit.Bars[0].ExactValues.Close != "10.5004" ||
		pit.Bars[0].ExactValues.RawAmount == nil || *pit.Bars[0].ExactValues.RawAmount != exactRawAmount {
		t.Fatalf("PIT Application DTO lost exact decimal sidecar: %+v", pit.Bars)
	}
	if pit.QualityStatus != app.QualityDegraded {
		t.Fatalf("PIT quality = %q, want degraded due to revision warning", pit.QualityStatus)
	}
}

func TestReaderRejectsMissingContextAndPreservesBackendErrors(t *testing.T) {
	if _, err := (*Reader)(nil).GetQuote(context.Background(), "600000.SH"); err == nil {
		t.Fatal("nil Reader must be rejected")
	}
	if _, err := (&Reader{backend: &fakeDataService{}}).GetQuote(nil, "600000.SH"); err == nil {
		t.Fatal("nil context must be rejected")
	}
	wantErr := errors.New("provider unavailable")
	reader := &Reader{backend: &fakeDataService{quoteErr: wantErr}}
	if _, err := reader.GetQuote(context.Background(), "600000.SH"); !errors.Is(err, wantErr) {
		t.Fatalf("GetQuote() error = %v, want wrapped/preserved %v", err, wantErr)
	}
}

func TestReaderGetDailyBarsRejectsInvalidContextAndPreservesBackendErrors(t *testing.T) {
	query := app.DailyBarsQuery{Symbol: "600000.SH", Limit: 10, AdjustmentBasis: app.AdjustmentNone}
	if _, err := (*Reader)(nil).GetDailyBars(context.Background(), query); err == nil {
		t.Fatal("nil Reader must be rejected")
	}
	reader := &Reader{backend: &fakeDataService{}}
	if _, err := reader.GetDailyBars(nil, query); err == nil {
		t.Fatal("nil context must be rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.GetDailyBars(ctx, query); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v, want context.Canceled", err)
	}
	wantErr := errors.New("daily bars unavailable")
	reader = &Reader{backend: &fakeDataService{currentErr: wantErr}}
	if _, err := reader.GetDailyBars(context.Background(), query); !errors.Is(err, wantErr) {
		t.Fatalf("GetDailyBars() error = %v, want preserved %v", err, wantErr)
	}
}

func TestReaderGetDailyBarsEmptyResultRemainsUnknown(t *testing.T) {
	reader := &Reader{backend: &fakeDataService{}}
	got, err := reader.GetDailyBars(context.Background(), app.DailyBarsQuery{
		Symbol: "600000.SH", Limit: 10, AdjustmentBasis: app.AdjustmentNone,
	})
	if err != nil {
		t.Fatalf("GetDailyBars() error = %v", err)
	}
	if got.QualityStatus != app.QualityUnknown || len(got.Bars) != 0 {
		t.Fatalf("empty data must remain unknown, got %+v", got)
	}
}

func TestReaderGetFundamentalsSelectsCurrentAndPITUseCases(t *testing.T) {
	availableAt := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	observedAt := availableAt.Add(time.Minute)
	snapshot := &model.FundamentalsSnapshot{
		Symbol:  "600519.SH",
		Year:    2025,
		Quarter: 4,
		HasData: true,
		Sections: []model.FundamentalSection{{
			Category:        "profitability",
			ReportDate:      "2025-12-31",
			PublicationDate: "2026-03-20",
			AvailableAt:     &availableAt,
			HasData:         true,
			Metrics:         []model.FundamentalMetric{{Key: "roe", Name: "净资产收益率", Value: 0.12, Unit: "ratio", SourceField: "roe"}},
			QualityStatus:   model.QualityDegraded,
			QualityReasons:  []string{"source_time_precision_date_only"},
		}},
		Source:           "tushare",
		SourceVersion:    "fund-v2",
		FetchedAt:        availableAt.Add(2 * time.Minute),
		SystemObservedAt: &observedAt,
		QualityStatus:    model.QualityDegraded,
		QualityReasons:   []string{"some_sections_unavailable"},
	}
	backend := &fakeDataService{fundamentals: snapshot}
	reader := &Reader{backend: backend}
	query := app.FundamentalsQuery{Symbol: "600519.SH", Year: 2025, Quarter: 4}
	current, err := reader.GetFundamentals(context.Background(), query)
	if err != nil {
		t.Fatalf("GetFundamentals(current) error = %v", err)
	}
	if backend.currentFundamentalsCalls != 1 || backend.pitFundamentalsCalls != 0 ||
		current.Symbol != snapshot.Symbol || current.Year != snapshot.Year || current.Quarter != snapshot.Quarter ||
		current.Source != snapshot.Source || current.SourceVersion != snapshot.SourceVersion ||
		current.QualityStatus != app.QualityDegraded || len(current.Sections) != 1 ||
		current.Sections[0].AvailableAt == nil || !current.Sections[0].AvailableAt.Equal(availableAt) ||
		current.Sections[0].Metrics[0].SourceField != "roe" || current.Sections[0].Metrics[0].Unit != "ratio" {
		t.Fatalf("fundamentals current mapping mismatch: calls=%d/%d result=%+v", backend.currentFundamentalsCalls, backend.pitFundamentalsCalls, current)
	}
	*current.Sections[0].AvailableAt = availableAt.Add(time.Hour)
	current.Sections[0].Metrics[0].Key = "mutated"
	current.Sections[0].QualityReasons[0] = "mutated"
	if !snapshot.Sections[0].AvailableAt.Equal(availableAt) ||
		snapshot.Sections[0].Metrics[0].Key != "roe" ||
		snapshot.Sections[0].QualityReasons[0] != "source_time_precision_date_only" {
		t.Fatal("fundamentals Application DTO aliases backend data")
	}
	cutoff := availableAt.Add(30 * time.Minute)
	query.SystemObservedAtCutoff = &cutoff
	historical, err := reader.GetFundamentals(context.Background(), query)
	if err != nil {
		t.Fatalf("GetFundamentals(PIT) error = %v", err)
	}
	if backend.currentFundamentalsCalls != 1 || backend.pitFundamentalsCalls != 1 ||
		!backend.lastFundamentalsAsOf.Equal(cutoff) ||
		historical.SystemObservedAt == nil || !historical.SystemObservedAt.Equal(observedAt) {
		t.Fatalf("fundamentals PIT mapping mismatch: %+v", historical)
	}
}

func TestReaderGetValuationsSelectsCurrentAndPITUseCases(t *testing.T) {
	providerTime := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	observedAt := providerTime.Add(time.Minute)
	pe, pb := 25.5, 3.2
	snapshot := &model.ValuationSnapshot{
		RequestedSymbols: []string{"600519.SH"},
		Total:            1,
		Items: []model.ValuationItem{{
			Symbol:         "600519.SH",
			Code:           "600519",
			Name:           "贵州茅台",
			PETTM:          &pe,
			PBMRQ:          &pb,
			Source:         "eastmoney",
			SourceVersion:  "valuation-v3",
			FetchedAt:      providerTime,
			QualityStatus:  model.QualityDegraded,
			QualityReasons: []string{"some_metric_missing"},
		}},
		ProviderReportDate: "2026-11-05",
		ProviderTimestamp:  &providerTime,
		Source:             "eastmoney",
		SourceVersion:      "valuation-v3",
		FetchedAt:          providerTime,
		SystemObservedAt:   &observedAt,
		QualityStatus:      model.QualityDegraded,
		QualityReasons:     []string{"provider_timestamp_is_aggregate"},
	}
	backend := &fakeDataService{valuations: snapshot}
	reader := &Reader{backend: backend}
	query := app.ValuationsQuery{Symbols: []string{"600519.SH"}}
	current, err := reader.GetValuations(context.Background(), query)
	if err != nil {
		t.Fatalf("GetValuations(current) error = %v", err)
	}
	if backend.currentValuationsCalls != 1 || backend.pitValuationsCalls != 0 ||
		current.Total != 1 || len(current.Items) != 1 || current.Items[0].PETTM == nil ||
		*current.Items[0].PETTM != pe || current.Items[0].PBMRQ == nil || *current.Items[0].PBMRQ != pb ||
		current.ProviderReportDate != "2026-11-05" || current.ProviderTimestamp == nil ||
		current.SourceVersion != "valuation-v3" || current.QualityStatus != app.QualityDegraded {
		t.Fatalf("valuation current mapping mismatch: calls=%d/%d result=%+v", backend.currentValuationsCalls, backend.pitValuationsCalls, current)
	}
	*current.Items[0].PETTM = 1
	current.Items[0].QualityReasons[0] = "mutated"
	current.RequestedSymbols[0] = "mutated"
	if *snapshot.Items[0].PETTM != pe || snapshot.Items[0].QualityReasons[0] != "some_metric_missing" ||
		snapshot.RequestedSymbols[0] != "600519.SH" {
		t.Fatal("valuations Application DTO aliases backend data")
	}
	cutoff := observedAt.Add(time.Minute)
	query.SystemObservedAtCutoff = &cutoff
	historical, err := reader.GetValuations(context.Background(), query)
	if err != nil {
		t.Fatalf("GetValuations(PIT) error = %v", err)
	}
	if backend.currentValuationsCalls != 1 || backend.pitValuationsCalls != 1 ||
		!backend.lastValuationsAsOf.Equal(cutoff) ||
		historical.SystemObservedAt == nil || !historical.SystemObservedAt.Equal(observedAt) {
		t.Fatalf("valuation PIT mapping mismatch: %+v", historical)
	}
}

func TestReaderFundamentalsAndValuationsPreserveBackendErrors(t *testing.T) {
	fundamentalsErr := errors.New("fundamentals storage unavailable")
	valuationsErr := errors.New("valuations revision not found")
	reader := &Reader{backend: &fakeDataService{
		fundamentalsErr: fundamentalsErr,
		valuationsErr:   valuationsErr,
	}}
	if _, err := reader.GetFundamentals(context.Background(), app.FundamentalsQuery{
		Symbol: "600519.SH", Year: 2025, Quarter: 4,
	}); !errors.Is(err, fundamentalsErr) {
		t.Fatalf("GetFundamentals() error = %v, want %v", err, fundamentalsErr)
	}
	if _, err := reader.GetValuations(context.Background(), app.ValuationsQuery{
		Symbols: []string{"600519.SH"},
	}); !errors.Is(err, valuationsErr) {
		t.Fatalf("GetValuations() error = %v, want %v", err, valuationsErr)
	}
	if _, err := reader.GetFundamentals(nil, app.FundamentalsQuery{}); err == nil {
		t.Fatal("GetFundamentals must reject nil context")
	}
	if _, err := reader.GetValuations(nil, app.ValuationsQuery{}); err == nil {
		t.Fatal("GetValuations must reject nil context")
	}
}
