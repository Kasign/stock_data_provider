package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
)

type fakeDataService struct {
	quote                    *model.Quote
	quoteErr                 error
	currentBars              []model.DailyBar
	currentWarnings          []string
	currentErr               error
	pitBars                  []model.DailyBar
	pitWarnings              []string
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
	current := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-11-06", IsPartial: true, Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, MA5: &ma5, MA20: &ma20, MA120: &ma120, MA250: &ma250,
		Volume: &volume, Amount: &amount, ChangePercent: &changePercent, AdjustmentBasis: model.AdjustmentForward,
		AdjustmentMethod: "provider_forward", AdjustmentRefDate: "2026-11-06",
		AdjustmentFactorSource: "eastmoney", AdjustmentFactorSourceVersion: "factor-v2",
		AdjustmentFactorRevisionHash: "factor-revision-v2", AdjustmentFactorSetHash: "factor-set-v2",
		AdjustmentFactorReferenceRevisionHash: "reference-revision-v2", AdjustmentAlgorithmVersion: "cumulative_adj_factor_v1",
		AdjustmentFactorSystemObservedAt: &factorObservedAt,
		Source:                           "eastmoney", SourceVersion: "bars-v3", ProviderTime: &providerTime,
		FetchedAt: providerTime.Add(3 * time.Minute), NormalizedAt: providerTime.Add(4 * time.Minute),
		SystemObservedAt: &systemObservedAt, RawVolume: &rawVolume, RawVolumeUnit: "手",
		RawAmount: &rawAmount, RawAmountUnit: "CNY", RawAmountField: "amount",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
		QualityReasons: []string{"source_contract_verified"}, FallbackFrom: "tushare",
		FallbackReason: "primary_timeout", RequestID: "req-bars", NormalizationVer: "norm-v1",
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
	if live.Bars[0].ProviderTime == nil || !live.Bars[0].ProviderTime.Equal(providerTime) ||
		live.Bars[0].SystemObservedAt == nil || !live.Bars[0].SystemObservedAt.Equal(systemObservedAt) {
		t.Fatalf("provider/system timestamps were not preserved: %+v", live.Bars[0])
	}
	*live.Bars[0].Volume = 1
	live.Bars[0].QualityReasons[0] = "mutated"
	live.Warnings[0] = "mutated"
	if *current.Volume != volume || current.QualityReasons[0] != "source_contract_verified" || backend.currentWarnings[0] != "bars_truncated_to_limit" {
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
		Symbol: "600519.SH", Year: 2025, Quarter: 4, HasData: true,
		Sections: []model.FundamentalSection{{
			Category: "profitability", ReportDate: "2025-12-31", PublicationDate: "2026-03-20",
			AvailableAt: &availableAt, HasData: true, QualityStatus: model.QualityDegraded,
			Metrics:        []model.FundamentalMetric{{Key: "roe", Name: "净资产收益率", Value: 0.12, Unit: "ratio", SourceField: "roe"}},
			QualityReasons: []string{"source_time_precision_date_only"},
		}},
		Source: "tushare", SourceVersion: "fund-v2", FetchedAt: availableAt.Add(2 * time.Minute),
		SystemObservedAt: &observedAt, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"some_sections_unavailable"},
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
		RequestedSymbols: []string{"600519.SH"}, Total: 1,
		Items: []model.ValuationItem{{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", PETTM: &pe, PBMRQ: &pb,
			Source: "eastmoney", SourceVersion: "valuation-v3", FetchedAt: providerTime,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"some_metric_missing"},
		}},
		ProviderReportDate: "2026-11-05", ProviderTimestamp: &providerTime,
		Source: "eastmoney", SourceVersion: "valuation-v3", FetchedAt: providerTime,
		SystemObservedAt: &observedAt, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_timestamp_is_aggregate"},
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
		fundamentalsErr: fundamentalsErr, valuationsErr: valuationsErr,
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
