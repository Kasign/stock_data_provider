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
	quote          *model.Quote
	quoteErr       error
	currentBars    []model.DailyBar
	currentWarnings []string
	currentErr     error
	pitBars        []model.DailyBar
	pitWarnings    []string
	pitErr         error
	currentCalls   int
	pitCalls       int
	lastAsOf       time.Time
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
	ma5 := 10.5
	current := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-11-06", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, MA5: &ma5,
		Volume: &volume, Amount: &amount, AdjustmentBasis: model.AdjustmentForward,
		AdjustmentMethod: "provider_forward", AdjustmentRefDate: "2026-11-06",
		AdjustmentFactorSource: "eastmoney", AdjustmentFactorSourceVersion: "factor-v2",
		AdjustmentFactorSystemObservedAt: &factorObservedAt,
		Source: "eastmoney", SourceVersion: "bars-v3", ProviderTime: &providerTime,
		FetchedAt: providerTime.Add(3 * time.Minute), NormalizedAt: providerTime.Add(4 * time.Minute),
		SystemObservedAt: &systemObservedAt, VolumeUnit: "shares", AmountUnit: "CNY",
		QualityStatus: model.QualityValid, RequestID: "req-bars", NormalizationVer: "norm-v1",
	}
	backend := &fakeDataService{
		currentBars: []model.DailyBar{current},
		currentWarnings: []string{"bars_truncated_to_limit"},
		pitBars: []model.DailyBar{current},
		pitWarnings: []string{"daily_bars_reconstructed_from_recorded_revisions"},
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
	if live.QualityStatus != app.QualityValid || live.VolumeUnit != "shares" || live.AmountUnit != "CNY" {
		t.Fatalf("current snapshot metadata mismatch: %+v", live)
	}
	if len(live.Bars) != 1 || live.Bars[0].AdjustmentReferenceDate != "2026-11-06" ||
		live.Bars[0].AdjustmentFactorSourceVersion != "factor-v2" || live.Bars[0].NormalizationVersion != "norm-v1" {
		t.Fatalf("daily-bar provenance metadata was not preserved: %+v", live.Bars)
	}
	if live.Bars[0].ProviderTime == nil || !live.Bars[0].ProviderTime.Equal(providerTime) ||
		live.Bars[0].SystemObservedAt == nil || !live.Bars[0].SystemObservedAt.Equal(systemObservedAt) {
		t.Fatalf("provider/system timestamps were not preserved: %+v", live.Bars[0])
	}
	*live.Bars[0].Volume = 1
	if *current.Volume != volume {
		t.Fatal("Application DTO must not alias DataService daily-bar pointers")
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
