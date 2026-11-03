package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

func TestDailyBarsAsOfRequiresRevisionStorage(t *testing.T) {
	svc := &DataService{defaultBars: 200, maxBars: 2000}
	asOf := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	_, _, err := svc.DailyBarsAsOf(context.Background(), "600519.SH", "2026-10-01", "2026-10-28", 100, model.AdjustmentNone, asOf)
	if !errors.Is(err, ErrDailyBarsAsOfUnavailable) {
		t.Fatalf("expected explicit PIT-storage-unavailable error, got %v", err)
	}
}

func TestDailyBarsAsOfRejectsFutureTimestampWithoutProviderFallback(t *testing.T) {
	svc := &DataService{defaultBars: 200, maxBars: 2000}
	_, _, err := svc.DailyBarsAsOf(context.Background(), "600519.SH", "", "", 100, model.AdjustmentNone, time.Now().UTC().Add(time.Hour))
	if err == nil || errors.Is(err, ErrDailyBarsAsOfUnavailable) {
		t.Fatalf("future as_of should fail validation before storage lookup, got %v", err)
	}
}

func TestRebuildAdjustedDailyBarsUsesAsOfFactorReferenceAndProvenance(t *testing.T) {
	observed := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	bars := []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-09-29", Open: 100, High: 110, Low: 90, Close: 100, AdjustmentBasis: model.AdjustmentNone}, {Symbol: "600519.SH", TradeDate: "2026-09-30", Open: 110, High: 120, Low: 100, Close: 110, AdjustmentBasis: model.AdjustmentNone}}
	factors := []model.AdjustmentFactor{{Symbol: "600519.SH", TradeDate: "2026-09-29", Factor: 2, Source: "factor-provider", SourceVersion: "v1", SystemObservedAt: &observed, AvailableAt: &observed, QualityStatus: model.QualityValid}, {Symbol: "600519.SH", TradeDate: "2026-09-30", Factor: 4, Source: "factor-provider", SourceVersion: "v1", SystemObservedAt: &observed, AvailableAt: &observed, QualityStatus: model.QualityValid}}
	got, err := rebuildAdjustedDailyBars(bars, factors, model.AdjustmentForward, "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Close != 50 || got[1].Close != 110 || got[0].AdjustmentRefDate != "2026-09-30" {
		t.Fatalf("unexpected qfq values: %+v", got)
	}
	if got[0].AdjustmentFactorSource != "factor-provider" || got[0].AdjustmentFactorSourceVersion != "v1" || got[0].AdjustmentFactorSystemObservedAt == nil {
		t.Fatalf("factor provenance missing: %+v", got[0])
	}
	back, err := rebuildAdjustedDailyBars(bars, factors, model.AdjustmentBackward, "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if back[0].Close != 200 || back[1].Close != 440 {
		t.Fatalf("unexpected hfq values: %+v", back)
	}
	if back[0].AdjustmentRefDate != "" || back[1].AdjustmentRefDate != "" {
		t.Fatalf("cumulative-factor backward adjustment must not claim a request-end reference date: %+v", back)
	}
}

func TestRebuildAdjustedDailyBarsRejectsUnprovenFactor(t *testing.T) {
	observed := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	bars := []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-09-29", Open: 10, High: 11, Low: 9, Close: 10, AdjustmentBasis: model.AdjustmentNone}}
	factors := []model.AdjustmentFactor{{Symbol: "600519.SH", TradeDate: "2026-09-29", Factor: 2, Source: "p", SourceVersion: "v1", SystemObservedAt: &observed, QualityStatus: model.QualityValid}}
	if _, err := rebuildAdjustedDailyBars(bars, factors, model.AdjustmentForward, "2026-09-29"); err == nil {
		t.Fatal("factor without available_at must be rejected")
	}
}

func TestRebuildAdjustedDailyBarsPropagatesDegradedFactorQuality(t *testing.T) {
	observed := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	bars := []model.DailyBar{
		{Symbol: "600519.SH", TradeDate: "2026-09-29", Open: 100, High: 110, Low: 90, Close: 100, AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid},
		{Symbol: "600519.SH", TradeDate: "2026-09-30", Open: 110, High: 120, Low: 100, Close: 110, AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid},
	}
	factors := []model.AdjustmentFactor{
		{Symbol: "600519.SH", TradeDate: "2026-09-29", Factor: 2, Source: "factor-provider", SourceVersion: "v1", SystemObservedAt: &observed, AvailableAt: &observed, QualityStatus: model.QualityValid},
		{Symbol: "600519.SH", TradeDate: "2026-09-30", Factor: 4, Source: "factor-provider", SourceVersion: "v1", SystemObservedAt: &observed, AvailableAt: &observed, QualityStatus: model.QualityDegraded, QualityReasons: []string{"factor_revision_incomplete"}},
	}
	got, err := rebuildAdjustedDailyBars(bars, factors, model.AdjustmentForward, "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].QualityStatus != model.QualityDegraded || !dailyBarsPITContainsQualityReason(got[0].QualityReasons, "adjustment_factor_reference_quality_degraded") || !dailyBarsPITContainsQualityReason(got[0].QualityReasons, "adjustment_factor_reference:factor_revision_incomplete") {
		t.Fatalf("reference-factor quality was not propagated to every qfq bar: %+v", got[0])
	}
	if got[1].QualityStatus != model.QualityDegraded || !dailyBarsPITContainsQualityReason(got[1].QualityReasons, "adjustment_factor_bar_quality_degraded") {
		t.Fatalf("bar-factor quality was not propagated: %+v", got[1])
	}
}

func dailyBarsPITContainsQualityReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
