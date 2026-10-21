package indicators

import (
	"math"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

func TestCalculateMovingAveragesAndTrend(t *testing.T) {
	bars := make([]domain.DailyBar, 25)
	for i := range bars {
		close := float64(i + 1)
		bars[i] = domain.DailyBar{Date: "2026-01-01", Close: close}
	}
	got, err := Calculate(bars)
	if err != nil {
		t.Fatal(err)
	}
	if got.MA5 == nil || math.Abs(*got.MA5-23) > 1e-9 {
		t.Fatalf("MA5 want 23, got %#v", got.MA5)
	}
	if got.MA10 == nil || math.Abs(*got.MA10-20.5) > 1e-9 {
		t.Fatalf("MA10 want 20.5, got %#v", got.MA10)
	}
	if got.MA20 == nil || math.Abs(*got.MA20-15.5) > 1e-9 {
		t.Fatalf("MA20 want 15.5, got %#v", got.MA20)
	}
	if got.MAAlignment != "bullish" || got.TrendStatus != "uptrend" {
		t.Fatalf("expected uptrend, got alignment=%q trend=%q", got.MAAlignment, got.TrendStatus)
	}
}

func TestCalculateRequiresValidClose(t *testing.T) {
	if _, err := Calculate([]domain.DailyBar{{Close: 0}}); err == nil {
		t.Fatal("expected error for empty/invalid close series")
	}
}

func TestRSIFlatSeriesDoesNotReturnNaN(t *testing.T) {
	bars := make([]domain.DailyBar, 30)
	for i := range bars {
		bars[i].Close = 10
	}
	got, err := Calculate(bars)
	if err != nil {
		t.Fatal(err)
	}
	if got.RSI14 == nil || math.IsNaN(*got.RSI14) || math.IsInf(*got.RSI14, 0) || *got.RSI14 != 50 {
		t.Fatalf("expected neutral RSI 50, got %#v", got.RSI14)
	}
}

func TestRSIUsesWilderSmoothingCompatibleWithLegacyEWM(t *testing.T) {
	closes := []float64{44.34, 44.09, 44.15, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84, 46.08, 45.89, 46.03, 45.61, 46.28, 46.28}
	bars := make([]domain.DailyBar, len(closes))
	for i, close := range closes {
		bars[i] = domain.DailyBar{Date: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Close: close}
	}
	got, err := Calculate(bars)
	if err != nil {
		t.Fatal(err)
	}
	if got.RSI14 == nil || math.Abs(*got.RSI14-71.80241065373282) > 1e-8 {
		t.Fatalf("RSI drifted from the legacy EWM result, got %#v", got.RSI14)
	}
}
