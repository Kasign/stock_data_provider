package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTickFlowFetchIndexQuotesParsesAllowlistedCoreIndexes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/quotes" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("symbols"); got != strings.Join(tickflowTestIndexSymbols(), ",") {
			t.Errorf("symbols query = %q", got)
		}
		now := time.Now().UTC().Add(-time.Minute).UnixMilli()
		rows := make([]map[string]any, 0, len(tickflowCoreIndexDefinitions))
		for _, def := range tickflowCoreIndexDefinitions {
			rows = append(rows, map[string]any{
				"symbol": def.Symbol, "name": def.Name, "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "amount": 123456.0, "timestamp": now,
				"ext": map[string]any{"change_amount": 1.0, "change_pct": 0.01},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
	}))
	defer server.Close()
	p := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := p.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != len(tickflowCoreIndexDefinitions) || got.QualityStatus != model.QualityValid {
		t.Fatalf("unexpected index quote snapshot: %+v", got)
	}
	if got.Items[0].Symbol != "000001.SH" || got.Items[0].Name != "上证指数" {
		t.Fatalf("unexpected first index row: %+v", got.Items[0])
	}
	if got.Items[0].Change == nil || *got.Items[0].Change != 1 {
		t.Fatalf("change amount should be derived from last price and previous close: %+v", got.Items[0].Change)
	}
	if got.Items[0].ChangePercent == nil || *got.Items[0].ChangePercent != 1 {
		t.Fatalf("change percent should be percentage points: %+v", got.Items[0].ChangePercent)
	}
	if got.Items[0].QuoteTime == nil || got.Items[0].QuoteTimeSemantics != "provider_snapshot_time" {
		t.Fatalf("provider timestamp semantics missing: %+v", got.Items[0])
	}
}

func TestTickFlowFetchIndexQuotesDegradesMissingOptionalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC().Add(-time.Minute).UnixMilli()
		rows := make([]map[string]any, 0, len(tickflowCoreIndexDefinitions))
		for _, def := range tickflowCoreIndexDefinitions {
			rows = append(rows, map[string]any{"symbol": def.Symbol, "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "timestamp": now})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
	}))
	defer server.Close()
	p := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := p.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing amount should degrade index snapshot: %+v", got)
	}
	if got.Items[0].Amount != nil || got.Items[0].AmountUnit != "unknown" {
		t.Fatalf("missing amount must remain nil/unknown, got %+v", got.Items[0])
	}
}

func TestTickFlowFetchIndexQuotesRequiresAPIKey(t *testing.T) {
	p := NewTickFlowConfigured(2*time.Second, "http://127.0.0.1", "", "")
	if _, err := p.FetchIndexQuotes(context.Background()); err == nil || !strings.Contains(err.Error(), "TICKFLOW_API_KEY") {
		t.Fatalf("index quotes without an API key must be rejected, got %v", err)
	}
}

func tickflowTestIndexSymbols() []string {
	symbols := make([]string, 0, len(tickflowCoreIndexDefinitions))
	for _, item := range tickflowCoreIndexDefinitions {
		symbols = append(symbols, item.Symbol)
	}
	return symbols
}

func TestTickFlowFetchIndexQuotesDegradesImpossibleProviderTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]any, 0, len(tickflowCoreIndexDefinitions))
		for _, def := range tickflowCoreIndexDefinitions {
			rows = append(rows, map[string]any{
				"symbol": def.Symbol, "name": def.Name, "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "amount": 123456.0, "timestamp": int64(1),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
	}))
	defer server.Close()

	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := provider.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("impossible provider timestamps must degrade the snapshot: %+v", got)
	}
	for _, item := range got.Items {
		if item.QuoteTime != nil || item.QuoteTimeSemantics != "" {
			t.Fatalf("impossible timestamp must not be exposed as provider time: %+v", item)
		}
		if !containsTickFlowReason(item.QualityReasons, "provider_quote_timestamp_missing_or_invalid") {
			t.Fatalf("missing timestamp degradation reason: %+v", item.QualityReasons)
		}
	}
}

func containsTickFlowReason(reasons []string, wanted string) bool {
	for _, reason := range reasons {
		if reason == wanted {
			return true
		}
	}
	return false
}

func TestTickFlowFetchIndexQuotesDropsInconsistentChangeExtensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC().Add(-time.Minute).UnixMilli()
		rows := make([]map[string]any, 0, len(tickflowCoreIndexDefinitions))
		for _, def := range tickflowCoreIndexDefinitions {
			changeAmount := 1.0
			changePercent := 0.01
			if def.Symbol == "000001.SH" {
				changeAmount = 9.0
			}
			rows = append(rows, map[string]any{
				"symbol": def.Symbol, "name": def.Name, "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "amount": 123456.0, "timestamp": now,
				"ext": map[string]any{"change_amount": changeAmount, "change_pct": changePercent},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
	}))
	defer server.Close()

	p := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := p.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("inconsistent extension fields must degrade the snapshot: %+v", got)
	}
	if len(got.Items) != len(tickflowCoreIndexDefinitions)-1 {
		t.Fatalf("inconsistent index row should be dropped; got %d items, want %d", len(got.Items), len(tickflowCoreIndexDefinitions)-1)
	}
	for _, item := range got.Items {
		if item.Symbol == "000001.SH" {
			t.Fatalf("index with contradictory extension fields must not be returned: %+v", item)
		}
	}
	if !containsTickFlowReason(got.QualityReasons, "provider_index_change_fields_inconsistent") {
		t.Fatalf("snapshot must disclose contradictory index change fields: %+v", got.QualityReasons)
	}
}
