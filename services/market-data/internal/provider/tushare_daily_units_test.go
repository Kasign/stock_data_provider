package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTushareDailyBarsNormalizesHandsAndThousandCNY(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %s, want POST", r.Method)
		}
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if !strings.Contains(string(rawBody), "\"api_name\":\"daily\"") {
			t.Errorf("unexpected Tushare request body: %s", string(rawBody))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","open","high","low","close","pct_chg","vol","amount"],"items":[["600519.SH","20261028",10,11,9,10,1.0,100,100]]}}`)
	}))
	defer server.Close()

	client := NewTushareConfigured(0, server.URL, "test-token")
	bars, err := client.FetchDailyBars(context.Background(), "600519.SH", "2026-10-28", "2026-10-28", 10, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("FetchDailyBars() error = %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("bars count = %d, want 1", len(bars))
	}
	bar := bars[0]
	if bar.Volume == nil || *bar.Volume != 10000 || bar.VolumeUnit != "shares" {
		t.Fatalf("Tushare vol is in hands and must normalize to shares: %+v", bar)
	}
	if bar.RawVolume == nil || *bar.RawVolume != 100 || bar.RawVolumeUnit != "hands" {
		t.Fatalf("raw volume provenance must be retained: %+v", bar)
	}
	if bar.Amount == nil || *bar.Amount != 100000 || bar.AmountUnit != "CNY" {
		t.Fatalf("Tushare amount is in thousand CNY and must normalize to CNY: %+v", bar)
	}
	if bar.RawAmount == nil || *bar.RawAmount != 100 || bar.RawAmountUnit != "thousand_CNY" || bar.RawAmountField != "amount" {
		t.Fatalf("raw amount provenance must be retained: %+v", bar)
	}
	if bar.QualityStatus != model.QualityValid {
		t.Fatalf("quality status = %s, want valid", bar.QualityStatus)
	}
}

func TestTushareDailyBarsMissingAmountStaysNullAndUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"","data":{"fields":["ts_code","trade_date","open","high","low","close","pct_chg","vol","amount"],"items":[["600519.SH","20261028",10,11,9,10,1.0,100,null]]}}`)
	}))
	defer server.Close()

	client := NewTushareConfigured(0, server.URL, "test-token")
	bars, err := client.FetchDailyBars(context.Background(), "600519.SH", "2026-10-28", "2026-10-28", 10, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("FetchDailyBars() error = %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("bars count = %d, want 1", len(bars))
	}
	bar := bars[0]
	if bar.Amount != nil || bar.AmountUnit != "unknown" {
		t.Fatalf("missing amount must remain null with unknown unit: %+v", bar)
	}
	if bar.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing amount must degrade quality, got %s", bar.QualityStatus)
	}
	if !containsProviderReason(bar.QualityReasons, "amount_missing_from_provider") {
		t.Fatalf("missing amount reason not recorded: %v", bar.QualityReasons)
	}
}

func containsProviderReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}
