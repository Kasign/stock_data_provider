package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/baostock"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type fakeBaoStockFactorClient struct {
	rows     []baostock.AdjustmentFactor
	err      error
	gotCode  string
	gotStart string
	gotEnd   string
}

func (f *fakeBaoStockFactorClient) QueryDailyKLines(context.Context, string, string, string, string) ([]baostock.DailyKLine, error) {
	return nil, nil
}

func (f *fakeBaoStockFactorClient) QueryAdjustmentFactors(_ context.Context, code, startDate, endDate string) ([]baostock.AdjustmentFactor, error) {
	f.gotCode, f.gotStart, f.gotEnd = code, startDate, endDate
	return f.rows, f.err
}

func TestBaoStockFetchAdjustmentFactorsSplitsBasisAndKeepsAvailabilityUnknown(t *testing.T) {
	client := &fakeBaoStockFactorClient{rows: []baostock.AdjustmentFactor{
		{Code: "sh.600036", OperateDate: "2025-07-11", ForwardFactor: 0.948859, BackwardFactor: 5.448589, AdjustmentFactor: 5.448589},
		{Code: "sh.600036", OperateDate: "2026-07-10", ForwardFactor: 1, BackwardFactor: 5.742256, AdjustmentFactor: 5.742256},
	}}
	p := NewBaostock(client)
	values, err := p.FetchAdjustmentFactors(context.Background(), "600036.SH", "2025-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if client.gotCode != "sh.600036" || client.gotStart != "2025-01-01" || client.gotEnd != "2026-12-31" {
		t.Fatalf("BaoStock query scope=%s %s..%s", client.gotCode, client.gotStart, client.gotEnd)
	}
	if len(values) != 4 {
		t.Fatalf("got %d factor records, want 4: %+v", len(values), values)
	}
	want := []struct {
		date  string
		basis model.AdjustmentFactorBasis
		value float64
	}{
		{"2025-07-11", model.AdjustmentFactorForward, 0.948859},
		{"2025-07-11", model.AdjustmentFactorBackward, 5.448589},
		{"2026-07-10", model.AdjustmentFactorForward, 1},
		{"2026-07-10", model.AdjustmentFactorBackward, 5.742256},
	}
	for i, item := range want {
		got := values[i]
		if got.TradeDate != item.date || got.Basis != item.basis || got.Factor != item.value {
			t.Fatalf("factor[%d]=%+v, want %+v", i, got, item)
		}
		if got.Symbol != "600036.SH" || got.Source != "baostock" || got.SourceVersion != baostockVersion {
			t.Fatalf("factor[%d] source identity is incomplete: %+v", i, got)
		}
		if got.FetchedAt.IsZero() || got.QualityStatus != model.QualityDegraded || got.AvailableAt != nil || got.SystemObservedAt != nil {
			t.Fatalf("factor[%d] must be fetched/degraded with unknown availability and no provider-supplied observation time: %+v", i, got)
		}
		if len(got.QualityReasons) != 1 || got.QualityReasons[0] != "provider_available_at_not_exposed_by_query_adjust_factor" {
			t.Fatalf("factor[%d] missing availability quality reason: %+v", i, got.QualityReasons)
		}
	}
}

func TestBaoStockFetchAdjustmentFactorsRejectsInvalidScopeAndRows(t *testing.T) {
	client := &fakeBaoStockFactorClient{rows: []baostock.AdjustmentFactor{
		{Code: "sz.002982", OperateDate: "2025-07-11", ForwardFactor: 1, BackwardFactor: 1},
	}}
	p := NewBaostock(client)
	if _, err := p.FetchAdjustmentFactors(context.Background(), "002982.SZ", "2025-08-01", "2025-07-01"); err == nil {
		t.Fatal("reversed date range must be rejected")
	}
	if _, err := p.FetchAdjustmentFactors(context.Background(), "002982.SZ", "2025-08-01", "2025-12-31"); err == nil {
		t.Fatal("out-of-range source rows must be rejected")
	}
	client.rows = []baostock.AdjustmentFactor{{Code: "sz.002982", OperateDate: "2025-07-11", ForwardFactor: 0, BackwardFactor: 1}}
	if _, err := p.FetchAdjustmentFactors(context.Background(), "002982.SZ", "2025-01-01", "2025-12-31"); err == nil {
		t.Fatal("non-positive factors must be rejected")
	}
	client.err = errors.New("upstream unavailable")
	if _, err := p.FetchAdjustmentFactors(context.Background(), "002982.SZ", "2025-01-01", "2025-12-31"); err == nil || err.Error() != "upstream unavailable" {
		t.Fatalf("upstream error must be preserved, got %v", err)
	}
}

func TestBaoStockFetchAdjustmentFactorsRequiresFactorCapability(t *testing.T) {
	p := NewBaostock(&fakeBaostockClient{})
	if _, err := p.FetchAdjustmentFactors(context.Background(), "600036.SH", "2025-01-01", "2025-12-31"); err == nil {
		t.Fatal("client without adjustment-factor capability must be rejected")
	}
}

func TestParseBaoStockFactorDateRejectsNonCanonicalDate(t *testing.T) {
	if _, err := parseBaoStockFactorDate("20250711"); err == nil {
		t.Fatal("compact dates must not be accepted as ISO dates")
	}
	if _, err := parseBaoStockFactorDate("2025-02-30"); err == nil {
		t.Fatal("invalid calendar date must be rejected")
	}
	if _, err := parseBaoStockFactorDate("2025-07-11"); err != nil {
		t.Fatalf("valid date rejected: %v", err)
	}
}
