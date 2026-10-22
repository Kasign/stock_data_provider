package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type fakeBaostockClient struct {
	rows      []baostock.DailyKLine
	err       error
	gotCode   string
	gotStart  string
	gotEnd    string
	gotAdjust string
}

func (f *fakeBaostockClient) QueryDailyKLines(_ context.Context, code, startDate, endDate, adjustFlag string) ([]baostock.DailyKLine, error) {
	f.gotCode, f.gotStart, f.gotEnd, f.gotAdjust = code, startDate, endDate, adjustFlag
	return f.rows, f.err
}

func TestBaostockDailyBarsNormalizesAmountsAndAdjustmentMetadata(t *testing.T) {
	client := &fakeBaostockClient{rows: []baostock.DailyKLine{{
		Date: "2026-10-21", Code: "sh.600519", Open: "1331.8800", High: "1331.9800",
		Low: "1302.3600", Close: "1309.6900", PreClose: "1331.8800",
		Volume: "3351400.000", Amount: "4403823070.0000", AdjustFlag: "2",
		Turn: "0.18", TradeStatus: "1", PctChg: "-1.66", IsST: "0",
	}}}
	p := NewBaostock(client)
	bars, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-09-02", "2026-10-21", 31, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("bars=%+v", bars)
	}
	bar := bars[0]
	if client.gotCode != "sh.600519" || client.gotAdjust != "2" {
		t.Fatalf("query code/adjust flag=%q/%q", client.gotCode, client.gotAdjust)
	}
	if client.gotStart != "2026-09-02" || client.gotEnd != "2026-10-21" {
		t.Fatalf("query date range=%s..%s", client.gotStart, client.gotEnd)
	}
	if *bar.Volume != 3351400 || bar.RawVolumeUnit != "shares" || bar.VolumeUnit != "shares" {
		t.Fatalf("volume unit/normalization mismatch: %+v", bar)
	}
	if bar.Amount == nil || *bar.Amount != 4403823070 || bar.AmountUnit != "CNY" || bar.RawAmountUnit != "CNY" {
		t.Fatalf("amount unit/normalization mismatch: %+v", bar)
	}
	if bar.AdjustmentBasis != model.AdjustmentForward || bar.AdjustmentMethod != "baostock_pct_change" {
		t.Fatalf("adjustment metadata=%s/%s", bar.AdjustmentBasis, bar.AdjustmentMethod)
	}
	if bar.ChangePercent == nil || *bar.ChangePercent != -1.66 || bar.QualityStatus != model.QualityValid {
		t.Fatalf("invalid normalized bar: %+v", bar)
	}
}

func TestBaostockMissingAmountIsDegradedAndNeverEstimated(t *testing.T) {
	client := &fakeBaostockClient{rows: []baostock.DailyKLine{{
		Date: "2026-10-21", Code: "sz.000001", Open: "10", High: "10.2",
		Low: "9.8", Close: "10.1", Volume: "100000", Amount: "",
		AdjustFlag: "3", TradeStatus: "1",
	}}}
	p := NewBaostock(client)
	bars, err := p.FetchDailyBars(context.Background(), "000001.SZ", "2026-10-21", "2026-10-21", 20, model.AdjustmentNone)
	if err != nil {
		t.Fatal(err)
	}
	bar := bars[0]
	if bar.Amount != nil || bar.RawAmount != nil || bar.AmountUnit != "unknown" || bar.RawAmountUnit != "" {
		t.Fatalf("missing amount must remain explicit: %+v", bar)
	}
	if bar.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing amount must degrade quality, got %s", bar.QualityStatus)
	}
	if client.gotAdjust != "3" {
		t.Fatalf("unadjusted query must use flag 3, got %q", client.gotAdjust)
	}
}

func TestBaostockRejectsAdjustmentMismatch(t *testing.T) {
	client := &fakeBaostockClient{rows: []baostock.DailyKLine{{
		Date: "2026-10-21", Code: "sh.600519", Open: "10", High: "10.2",
		Low: "9.8", Close: "10.1", Volume: "1000", Amount: "10000", AdjustFlag: "3",
	}}}
	p := NewBaostock(client)
	_, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 20, model.AdjustmentForward)
	if err == nil {
		t.Fatal("BaoStock must not relabel unadjusted results as forward adjusted")
	}
}

func TestBaostockRejectsBeijingExchangeUntilVerified(t *testing.T) {
	p := NewBaostock(&fakeBaostockClient{})
	if _, err := p.FetchDailyBars(context.Background(), "430047.BJ", "2026-10-01", "2026-10-21", 20, model.AdjustmentForward); err == nil {
		t.Fatal("BaoStock BJ support must not be assumed")
	}
}

func TestBaostockReturnsUpstreamFailure(t *testing.T) {
	client := &fakeBaostockClient{err: errors.New("fake upstream unavailable")}
	p := NewBaostock(client)
	_, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-01", "2026-10-21", 20, model.AdjustmentForward)
	if err == nil || err.Error() != "fake upstream unavailable" {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

func TestBaostockPreservesExplicitDateRange(t *testing.T) {
	now := time.Date(2026, 10, 22, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	start, end, err := baostockDateRange("2026-09-02", "2026-10-21", 31, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2026-09-02" || end != "2026-10-21" {
		t.Fatalf("date range=%s..%s", start, end)
	}
	start, end, err = baostockDateRange("", "2026-10-21", 30, now)
	if err != nil {
		t.Fatal(err)
	}
	if start != "2026-05-24" || end != "2026-10-21" {
		t.Fatalf("implicit bounded date range=%s..%s", start, end)
	}
}

func TestBaostockRejectsInvalidOHLC(t *testing.T) {
	tests := []struct {
		name  string
		open  string
		high  string
		low   string
		close string
	}{
		{name: "open above high", open: "10.3", high: "10.2", low: "9.8", close: "10.1"},
		{name: "low above high", open: "10.0", high: "9.9", low: "10.1", close: "10.0"},
		{name: "close below low", open: "10.0", high: "10.2", low: "9.8", close: "9.7"},
		{name: "close above high", open: "10.0", high: "10.2", low: "9.8", close: "10.3"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeBaostockClient{rows: []baostock.DailyKLine{{
				Date: "2026-10-21", Code: "sh.600519",
				Open: tc.open, High: tc.high, Low: tc.low, Close: tc.close,
				Volume: "1000", Amount: "10000", AdjustFlag: "2",
			}}}
			p := NewBaostock(client)
			_, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-01", "2026-10-21", 20, model.AdjustmentForward)
			if err == nil || !strings.Contains(err.Error(), "OHLC invariant") {
				t.Fatalf("异常 OHLC 必须被拒绝，实际错误：%v", err)
			}
		})
	}
}

func TestBaostockRejectsDuplicateTradeDates(t *testing.T) {
	row := baostock.DailyKLine{
		Date: "2026-10-21", Code: "sh.600519",
		Open: "10.0", High: "10.2", Low: "9.8", Close: "10.1",
		Volume: "1000", Amount: "10000", AdjustFlag: "2",
	}
	client := &fakeBaostockClient{rows: []baostock.DailyKLine{row, row}}
	p := NewBaostock(client)
	_, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-01", "2026-10-21", 20, model.AdjustmentForward)
	if err == nil || !strings.Contains(err.Error(), "duplicate trade date") {
		t.Fatalf("重复交易日必须被拒绝，实际错误：%v", err)
	}
}
