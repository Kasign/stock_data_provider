package marketdata

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
	dataapp "example.com/stock-analysis-flow/internal/marketdata/app"
)

type fakeMarketDataReader struct {
	quote   dataapp.QuoteSnapshot
	bars    map[string]dataapp.DailyBarsSnapshot
	queries []dataapp.DailyBarsQuery
}

func (f *fakeMarketDataReader) GetQuote(context.Context, string) (dataapp.QuoteSnapshot, error) {
	return f.quote, nil
}

func (f *fakeMarketDataReader) GetDailyBars(_ context.Context, query dataapp.DailyBarsQuery) (dataapp.DailyBarsSnapshot, error) {
	f.queries = append(f.queries, query)
	return f.bars[query.Symbol], nil
}

func (f *fakeMarketDataReader) GetFundamentals(context.Context, dataapp.FundamentalsQuery) (dataapp.FundamentalsSnapshot, error) {
	return dataapp.FundamentalsSnapshot{}, nil
}

func (f *fakeMarketDataReader) GetValuations(context.Context, dataapp.ValuationsQuery) (dataapp.ValuationsSnapshot, error) {
	return dataapp.ValuationsSnapshot{}, nil
}

func TestAnalysisCollectorMapsDailyBarsFromDataApplication(t *testing.T) {
	reader := &fakeMarketDataReader{
		bars: map[string]dataapp.DailyBarsSnapshot{
			"600519.SH": {
				Symbol: "600519.SH", AdjustmentBasis: dataapp.AdjustmentForward,
				QualityStatus:   dataapp.QualityValid,
				Bars: []dataapp.DailyBar{{
					Symbol: "600519.SH", TradeDate: "2026-11-17",
					Open: 100, High: 110, Low: 99, Close: 108,
					Source: "eastmoney", FetchedAt: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC),
					AdjustmentBasis: dataapp.AdjustmentForward,
					QualityStatus: dataapp.QualityValid,
				}},
			},
		},
	}
	collector, err := NewAnalysisCollector(reader, 120, dataapp.AdjustmentForward, []string{"000001.SH"})
	if err != nil {
		t.Fatalf("NewAnalysisCollector() error = %v", err)
	}
	asOf := time.Date(2026, 11, 18, 10, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	result, err := collector.Collect(context.Background(), domain.RunRequest{AsOf: asOf},
		domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600519.SH", AssetType: "equity"}}},
		domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: "daily_bars"}}})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(result.Bars) != 1 {
		t.Fatalf("daily bars count = %d, want 1", len(result.Bars))
	}
	if got := result.Bars[0].TradeDate.Format("2006-01-02"); got != "2026-11-17" {
		t.Fatalf("trade date = %s", got)
	}
	if result.Bars[0].AdjustBasis != string(dataapp.AdjustmentForward) {
		t.Fatalf("adjustment basis = %q", result.Bars[0].AdjustBasis)
	}
	if len(reader.queries) != 1 || reader.queries[0].EndDate != "2026-11-18" || reader.queries[0].Limit != 120 {
		t.Fatalf("Data query not aligned to Analysis request: %+v", reader.queries)
	}
}

func TestAnalysisCollectorAddsConfiguredIndexBars(t *testing.T) {
	reader := &fakeMarketDataReader{
		bars: map[string]dataapp.DailyBarsSnapshot{
			"000001.SH": {
				Symbol: "000001.SH", AdjustmentBasis: dataapp.AdjustmentNone,
				QualityStatus: dataapp.QualityValid,
				Bars: []dataapp.DailyBar{{
					Symbol: "000001.SH", TradeDate: "2026-11-17",
					Open: 3000, High: 3010, Low: 2990, Close: 3005,
					Source: "eastmoney", AdjustmentBasis: dataapp.AdjustmentNone,
					QualityStatus: dataapp.QualityValid,
				}},
			},
		},
	}
	collector, err := NewAnalysisCollector(reader, 60, dataapp.AdjustmentNone, []string{"000001.SH"})
	if err != nil {
		t.Fatalf("NewAnalysisCollector() error = %v", err)
	}
	asOf := time.Date(2026, 11, 18, 10, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	result, err := collector.Collect(context.Background(), domain.RunRequest{AsOf: asOf},
		domain.UniverseSnapshot{}, domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: "index_bars"}}})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(result.IndexBars) != 1 || result.IndexBars[0].Symbol != "000001.SH" {
		t.Fatalf("index bars = %+v", result.IndexBars)
	}
}

func TestAnalysisCollectorRejectsQuoteAfterAsOf(t *testing.T) {
	price := 100.0
	quoteTime := time.Date(2026, 11, 18, 11, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	reader := &fakeMarketDataReader{quote: dataapp.QuoteSnapshot{
		Symbol: "600519.SH", CurrentPrice: &price, QuoteTime: &quoteTime,
		Source: "eastmoney", QualityStatus: dataapp.QualityValid,
	}}
	collector, err := NewAnalysisCollector(reader, 60, dataapp.AdjustmentNone, nil)
	if err != nil {
		t.Fatalf("NewAnalysisCollector() error = %v", err)
	}
	asOf := time.Date(2026, 11, 18, 10, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	result, err := collector.Collect(context.Background(), domain.RunRequest{AsOf: asOf},
		domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600519.SH", AssetType: "equity"}}},
		domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: "realtime_quote"}}})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(result.Quotes) != 0 {
		t.Fatalf("future quote was included: %+v", result.Quotes)
	}
	if len(result.SourceNotes) != 1 || result.SourceNotes[0].QualityStatus != "unavailable" {
		t.Fatalf("future quote diagnostic missing: %+v", result.SourceNotes)
	}
}
