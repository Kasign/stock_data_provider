package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEastmoneyCapitalFlowSortsAndAggregatesLatestTenObservations(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc)
	rows := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		rows = append(rows, fmt.Sprintf("%s,%d", date, 10-i))
	}
	encodedRows, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("encode fixture rows: %v", err)
	}
	body := fmt.Sprintf(`{"rc":0,"data":{"code":"600519","klines":%s}}`, encodedRows)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fields2") != "f51,f52" {
			t.Errorf("unexpected fields2: %s", r.URL.Query().Get("fields2"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	p := NewEastmoney(time.Second, "", "").WithCapitalFlowURL(server.URL)
	got, err := p.FetchCapitalFlow(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchCapitalFlow returned error: %v", err)
	}
	if got.ProviderDate != today.Format("2006-01-02") || got.Observations != 10 {
		t.Fatalf("unexpected date/observations: date=%s observations=%d", got.ProviderDate, got.Observations)
	}
	if got.MainNetInflow == nil || *got.MainNetInflow != 10 {
		t.Fatalf("unexpected latest main net inflow: %#v", got.MainNetInflow)
	}
	if got.Inflow5D == nil || *got.Inflow5D != 40 {
		t.Fatalf("unexpected 5-day sum: %#v", got.Inflow5D)
	}
	if got.Inflow10D == nil || *got.Inflow10D != 55 {
		t.Fatalf("unexpected 10-day sum: %#v", got.Inflow10D)
	}
	if got.QualityStatus != "degraded" {
		t.Fatalf("capital-flow quality must remain degraded, got %q", got.QualityStatus)
	}
}

func TestEastmoneyCapitalFlowRejectsSymbolMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rc":0,"data":{"code":"000001","klines":["2026-10-26,100"]}}`))
	}))
	defer server.Close()

	p := NewEastmoney(time.Second, "", "").WithCapitalFlowURL(server.URL)
	_, err := p.FetchCapitalFlow(context.Background(), "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("expected symbol mismatch, got %v", err)
	}
}

func TestEastmoneyCapitalFlowRejectsMissingSecurityIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rc":0,"data":{"klines":["2026-10-26,100"]}}`))
	}))
	defer server.Close()

	p := NewEastmoney(time.Second, "", "").WithCapitalFlowURL(server.URL)
	_, err := p.FetchCapitalFlow(context.Background(), "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "missing security code") {
		t.Fatalf("missing returned security identity must be rejected, got %v", err)
	}
}

func TestEastmoneyCapitalFlowRejectsMalformedSecurityIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rc":0,"data":{"code":"60051","klines":["2026-10-26,100"]}}`))
	}))
	defer server.Close()

	p := NewEastmoney(time.Second, "", "").WithCapitalFlowURL(server.URL)
	_, err := p.FetchCapitalFlow(context.Background(), "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "invalid security code") {
		t.Fatalf("malformed returned security identity must be rejected, got %v", err)
	}
}
