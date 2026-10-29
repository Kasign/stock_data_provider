package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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
