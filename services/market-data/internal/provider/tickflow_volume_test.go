package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

const tickflowVolumeFixture = 1234567.0

func TestTickFlowFetchQuoteKeepsVolumeInShares(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/quotes" || r.URL.Query().Get("symbols") != "600519.SH" {
			t.Errorf("unexpected quote request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Error("TickFlow API key header missing")
		}
		row := map[string]any{
			"symbol": "600519.SH", "name": "贵州茅台",
			"last_price": 101.0, "prev_close": 100.0,
			"open": 100.0, "high": 102.0, "low": 99.0,
			"volume": tickflowVolumeFixture, "amount": 123456789.0,
			"timestamp": time.Now().UTC().Add(-time.Minute).UnixMilli(),
			"ext": map[string]any{"change_amount": 1.0, "change_pct": 0.01},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []map[string]any{row}})
	}))
	defer server.Close()

	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := provider.FetchQuote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume == nil || *got.Volume != int64(tickflowVolumeFixture) {
		t.Fatalf("TickFlow quote volume should stay in shares: %+v", got.Volume)
	}
	if got.RawVolume == nil || *got.RawVolume != tickflowVolumeFixture ||
		got.RawVolumeUnit != "shares" || got.VolumeUnit != "shares" {
		t.Fatalf("TickFlow quote volume metadata is inconsistent: raw=%v raw_unit=%q unit=%q", got.RawVolume, got.RawVolumeUnit, got.VolumeUnit)
	}
	if got.QualityStatus != model.QualityValid {
		t.Fatalf("complete quote fixture should have valid quality: %+v", got.QualityReasons)
	}
}

func TestTickFlowFetchDailyBarsKeepsVolumeInShares(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, -1)
	dateISO := date.Format("2006-01-02")
	timestamp := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/klines" || r.URL.Query().Get("symbol") != "600519.SH" {
			t.Errorf("unexpected K-line request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Error("TickFlow API key header missing")
		}
		if r.URL.Query().Get("start_time") == "" || r.URL.Query().Get("end_time") == "" {
			t.Error("K-line date interval query is missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"timestamp": []int64{timestamp},
			"open": []float64{100}, "high": []float64{102}, "low": []float64{99}, "close": []float64{101},
			"volume": []float64{tickflowVolumeFixture}, "amount": []float64{123456789}, "prev_close": []float64{100},
		}})
	}))
	defer server.Close()

	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	bars, err := provider.FetchDailyBars(context.Background(), "600519.SH", dateISO, dateISO, 10, model.AdjustmentNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one daily bar, got %+v", bars)
	}
	if bars[0].Volume == nil || *bars[0].Volume != int64(tickflowVolumeFixture) {
		t.Fatalf("TickFlow daily volume should stay in shares: %+v", bars[0].Volume)
	}
	if bars[0].RawVolume == nil || *bars[0].RawVolume != tickflowVolumeFixture ||
		bars[0].RawVolumeUnit != "shares" || bars[0].VolumeUnit != "shares" {
		t.Fatalf("TickFlow daily volume metadata is inconsistent: raw=%v raw_unit=%q unit=%q", bars[0].RawVolume, bars[0].RawVolumeUnit, bars[0].VolumeUnit)
	}
	if bars[0].QualityStatus != model.QualityValid {
		t.Fatalf("complete daily fixture should have valid quality: %+v", bars[0].QualityReasons)
	}
}
