package provider

import (
	"math"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestApplyTushareAdjustmentUsesExplicitReferenceDate(t *testing.T) {
	bars := []model.DailyBar{
		{TradeDate: "2026-10-27", Open: 10, High: 11, Low: 9, Close: 10},
		{TradeDate: "2026-10-28", Open: 20, High: 22, Low: 18, Close: 20},
	}
	factors := map[string]float64{
		"2026-10-27": 1,
		"2026-10-28": 2,
	}

	referenceDate, err := applyTushareAdjustment(bars, factors, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if referenceDate != "2026-10-28" {
		t.Fatalf("reference date = %q, want 2026-10-28", referenceDate)
	}
	if bars[0].AdjustmentRefDate != referenceDate || bars[1].AdjustmentRefDate != referenceDate {
		t.Fatalf("forward-adjusted bars must expose the same reference date: %+v", bars)
	}
	if bars[0].AdjustmentMethod != "tushare_adj_factor_qfq_v1" || bars[1].AdjustmentMethod != "tushare_adj_factor_qfq_v1" {
		t.Fatalf("unexpected forward-adjustment method: %+v", bars)
	}
	if bars[0].Close != 5 || bars[1].Close != 20 {
		t.Fatalf("unexpected forward-adjusted close values: got %v and %v", bars[0].Close, bars[1].Close)
	}
}

func TestApplyTushareBackwardAdjustmentKeepsRawVolumeAndAmount(t *testing.T) {
	volume := int64(100)
	amount := 1000.0
	bars := []model.DailyBar{{
		TradeDate: "2026-10-28", Open: 10, High: 12, Low: 9, Close: 11,
		Volume: &volume, Amount: &amount, VolumeUnit: "shares", AmountUnit: "CNY",
	}}
	_, err := applyTushareAdjustment(bars, map[string]float64{"2026-10-28": 2}, model.AdjustmentBackward)
	if err != nil {
		t.Fatal(err)
	}
	if bars[0].Close != 22 || bars[0].Open != 20 || bars[0].High != 24 || bars[0].Low != 18 {
		t.Fatalf("unexpected backward-adjusted OHLC: %+v", bars[0])
	}
	if bars[0].AdjustmentRefDate != "" {
		t.Fatalf("backward adjustment should not claim a forward reference date: %+v", bars[0])
	}
	if bars[0].Volume == nil || *bars[0].Volume != volume || bars[0].Amount == nil || *bars[0].Amount != amount {
		t.Fatalf("adjustment must not rescale raw volume or amount: %+v", bars[0])
	}
	if bars[0].AdjustmentMethod != "tushare_adj_factor_hfq_v1" {
		t.Fatalf("unexpected adjustment method %q", bars[0].AdjustmentMethod)
	}
}

func TestApplyTushareAdjustmentRejectsMissingOrInvalidFactors(t *testing.T) {
	newBars := func() []model.DailyBar {
		return []model.DailyBar{{TradeDate: "2026-10-28", Open: 10, High: 12, Low: 9, Close: 11}}
	}
	cases := []struct {
		name    string
		factors map[string]float64
		basis   model.AdjustmentBasis
	}{
		{name: "missing bar factor", factors: map[string]float64{"2026-10-27": 1}, basis: model.AdjustmentForward},
		{name: "zero factor", factors: map[string]float64{"2026-10-28": 0}, basis: model.AdjustmentForward},
		{name: "NaN factor", factors: map[string]float64{"2026-10-28": math.NaN()}, basis: model.AdjustmentBackward},
		{name: "unsupported basis", factors: map[string]float64{"2026-10-28": 1}, basis: model.AdjustmentNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := applyTushareAdjustment(newBars(), tc.factors, tc.basis); err == nil {
				t.Fatal("expected invalid adjustment input to return an error")
			}
		})
	}
}
