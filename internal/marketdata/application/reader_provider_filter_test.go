package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/service"
)

type providerFilteredReaderBackend struct {
	allowed []string
	bars    []model.DailyBar
}

func (backend *providerFilteredReaderBackend) Quote(context.Context, string) (*model.Quote, bool, error) {
	return nil, false, errors.New("unexpected Quote call")
}
func (backend *providerFilteredReaderBackend) DailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, bool, []string, error) {
	return nil, false, nil, errors.New("unexpected DailyBars call")
}
func (backend *providerFilteredReaderBackend) DailyBarsAsOf(context.Context, string, string, string, int, model.AdjustmentBasis, time.Time) ([]model.DailyBar, []string, error) {
	return nil, nil, errors.New("unexpected DailyBarsAsOf call")
}
func (backend *providerFilteredReaderBackend) Fundamentals(context.Context, string, int, int) (*model.FundamentalsSnapshot, bool, error) {
	return nil, false, errors.New("unexpected Fundamentals call")
}
func (backend *providerFilteredReaderBackend) FundamentalsAsOf(context.Context, string, int, int, time.Time) (*model.FundamentalsSnapshot, error) {
	return nil, errors.New("unexpected FundamentalsAsOf call")
}
func (backend *providerFilteredReaderBackend) Valuations(context.Context, []string) (*model.ValuationSnapshot, bool, error) {
	return nil, false, errors.New("unexpected Valuations call")
}
func (backend *providerFilteredReaderBackend) ValuationsAsOf(context.Context, []string, time.Time) (*model.ValuationSnapshot, error) {
	return nil, errors.New("unexpected ValuationsAsOf call")
}
func (backend *providerFilteredReaderBackend) DailyBarsWithCoverageEvidence(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, bool, []string, service.DailyBarCoverageEvidence, error) {
	return nil, false, nil, service.DailyBarCoverageEvidence{}, errors.New("unexpected unfiltered DailyBars call")
}
func (backend *providerFilteredReaderBackend) DailyBarsAsOfWithCoverageEvidence(context.Context, string, string, string, int, model.AdjustmentBasis, time.Time) ([]model.DailyBar, []string, service.DailyBarCoverageEvidence, error) {
	return nil, nil, service.DailyBarCoverageEvidence{}, errors.New("unexpected unfiltered PIT DailyBars call")
}
func (backend *providerFilteredReaderBackend) DailyBarsAsOfWithCoverageEvidenceFromSources(_ context.Context, _ string, _ string, _ string, _ int, _ model.AdjustmentBasis, _ time.Time, sources []string) ([]model.DailyBar, []string, service.DailyBarCoverageEvidence, error) {
	backend.allowed = append([]string(nil), sources...)
	return append([]model.DailyBar(nil), backend.bars...), nil, service.DailyBarCoverageEvidence{
		CalendarQualityStatus: model.QualityValid, ExpectedTradingDays: 20,
		MinimumRequiredBars: 20, CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		CalendarRef: "data_calendar:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, nil
}

func TestReaderPassesCanonicalProviderAllowlistToPITBackend(t *testing.T) {
	backend := &providerFilteredReaderBackend{}
	reader := &Reader{backend: backend}
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	_, err := reader.GetDailyBarsV2(context.Background(), DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-10-01", EndDate: "2026-10-30",
		Limit: 20, AdjustmentBasis: AdjustmentNone, SystemObservedAtCutoff: &cutoff,
		AllowedProviderCodes: []string{"eastmoney"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.allowed, []string{"eastmoney"}) {
		t.Fatalf("Data Reader did not preserve the ResourcePolicy provider allowlist: %#v", backend.allowed)
	}
}

func TestReaderRejectsProviderAllowlistWithoutPITOrCanonicalOrder(t *testing.T) {
	backend := &providerFilteredReaderBackend{}
	reader := &Reader{backend: backend}
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	tests := []DailyBarsQuery{
		{Symbol: "600036.SH", AllowedProviderCodes: []string{"eastmoney"}},
		{Symbol: "600036.SH", SystemObservedAtCutoff: &cutoff, AllowedProviderCodes: []string{"tavily", "eastmoney"}},
		{Symbol: "600036.SH", SystemObservedAtCutoff: &cutoff, AllowedProviderCodes: []string{"Eastmoney"}},
		{Symbol: "600036.SH", SystemObservedAtCutoff: &cutoff, AllowedProviderCodes: []string{"eastmoney", "eastmoney"}},
	}
	for index, query := range tests {
		if _, err := reader.GetDailyBarsV2(context.Background(), query); err == nil {
			t.Errorf("query %d with invalid provider constraints was accepted", index)
		}
	}
	if len(backend.allowed) != 0 {
		t.Fatalf("invalid query reached the Data backend: %#v", backend.allowed)
	}
}

func TestReaderRejectsProviderOutsidePITAllowlistEvenIfBackendReturnsIt(t *testing.T) {
	backend := &providerFilteredReaderBackend{
		bars: []model.DailyBar{{Symbol: "600036.SH", Source: "baostock"}},
	}
	reader := &Reader{backend: backend}
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	_, err := reader.GetDailyBarsV2(context.Background(), DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-10-01", EndDate: "2026-10-30",
		Limit: 20, AdjustmentBasis: AdjustmentNone, SystemObservedAtCutoff: &cutoff,
		AllowedProviderCodes: []string{"eastmoney"},
	})
	if err == nil {
		t.Fatal("Data Reader accepted a backend source outside the immutable allowlist")
	}
}
