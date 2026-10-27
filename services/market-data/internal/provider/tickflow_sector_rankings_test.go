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

var tickflowSectorTestSymbols = []string{"600000.SH", "000001.SZ", "300001.SZ"}

func newTickFlowSectorFixture(t *testing.T, quoteSymbols []string) *httptest.Server {
	t.Helper()
	timestamp := time.Now().UTC().Add(-time.Minute).UnixMilli()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Errorf("x-api-key header was not forwarded")
		}
		switch r.URL.Path {
		case "/v1/universes":
			if r.Method != http.MethodGet {
				t.Errorf("universe-list method = %s, want GET", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []map[string]any{
				{"id": "CN_Equity_A", "name": "A股", "symbol_count": len(tickflowSectorTestSymbols)},
				{"id": "CN_Equity_SW1_BANK", "name": "SW1银行", "symbol_count": 2},
				{"id": "CN_Equity_SW1_FOOD", "name": "SW1食品饮料", "symbol_count": 1},
			}})
		case "/v1/universes/CN_Equity_A":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"id": "CN_Equity_A", "symbol_count": len(tickflowSectorTestSymbols),
				"symbols": tickflowSectorTestSymbols,
			}})
		case "/v1/universes/batch":
			if r.Method != http.MethodPost {
				t.Errorf("universe batch method = %s, want POST", r.Method)
			}
			var request struct {
				IDs []string `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode batch body: %v", err)
			}
			if len(request.IDs) != 2 || request.IDs[0] != "CN_Equity_SW1_BANK" || request.IDs[1] != "CN_Equity_SW1_FOOD" {
				t.Errorf("unexpected SW1 batch IDs: %+v", request.IDs)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
				"CN_Equity_SW1_BANK": map[string]any{
					"id": "CN_Equity_SW1_BANK", "symbol_count": 2,
					"symbols": []string{"600000.SH", "000001.SZ"},
				},
				"CN_Equity_SW1_FOOD": map[string]any{
					"id": "CN_Equity_SW1_FOOD", "symbol_count": 1,
					"symbols": []string{"300001.SZ"},
				},
			}})
		case "/v1/quotes":
			if r.Method != http.MethodGet || r.URL.Query().Get("universes") != "CN_Equity_A" {
				t.Errorf("unexpected A-share quote request: %s %s", r.Method, r.URL.String())
			}
			quotes := make([]map[string]any, 0, len(quoteSymbols))
			for _, symbol := range quoteSymbols {
				last, prev, pct := 10.0, 10.0, 0.0
				switch symbol {
				case "600000.SH":
					last, prev, pct = 10.2, 10.0, 0.02
				case "000001.SZ":
					last, prev, pct = 9.9, 10.0, -0.01
				case "300001.SZ":
					last, prev, pct = 20.6, 20.0, 0.03
				}
				quotes = append(quotes, map[string]any{
					"symbol": symbol, "last_price": last, "prev_close": prev,
					"open": prev, "high": last, "low": prev, "timestamp": timestamp,
					"ext": map[string]any{"change_pct": pct},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": quotes})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTickFlowFetchSectorRankingsBuildsSW1IndustryRanking(t *testing.T) {
	server := newTickFlowSectorFixture(t, tickflowSectorTestSymbols)
	defer server.Close()

	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := provider.FetchSectorRankings(context.Background(), "industry", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "industry" || got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("unexpected SW1 ranking snapshot: %+v", got)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("constituent-average rankings should be degraded: %+v", got.QualityStatus)
	}
	if got.Items[0].Name != "食品饮料" || got.Items[0].Rank != 1 ||
		got.Items[0].ChangePercent == nil || *got.Items[0].ChangePercent != 3 {
		t.Fatalf("unexpected leading industry: %+v", got.Items[0])
	}
	if got.Items[1].Name != "银行" || got.Items[1].Rank != 2 ||
		got.Items[1].ChangePercent == nil || *got.Items[1].ChangePercent < 0.49 || *got.Items[1].ChangePercent > 0.51 {
		t.Fatalf("unexpected second industry: %+v", got.Items[1])
	}
	if got.Items[1].AdvancingCount == nil || *got.Items[1].AdvancingCount != 1 ||
		got.Items[1].DecliningCount == nil || *got.Items[1].DecliningCount != 1 {
		t.Fatalf("unexpected bank constituents' advancing/declining counts: %+v", got.Items[1])
	}
	if got.Items[0].AmountUnit != "unknown" || got.Items[0].ProviderReportDate == "" {
		t.Fatalf("unsupported amount must stay unknown and report date must come from quotes: %+v", got.Items[0])
	}
	if !containsTickFlowReason(got.QualityReasons, "sector_change_percent_is_average_of_constituent_changes") {
		t.Fatalf("average-based ranking quality reason is missing: %+v", got.QualityReasons)
	}
}

func TestTickFlowFetchSectorRankingsRejectsIncompleteQuoteCoverage(t *testing.T) {
	server := newTickFlowSectorFixture(t, tickflowSectorTestSymbols[:2])
	defer server.Close()

	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	if _, err := provider.FetchSectorRankings(context.Background(), "industry", 10); err == nil ||
		!strings.Contains(err.Error(), "quotes coverage mismatch") {
		t.Fatalf("incomplete A-share quote coverage must fail, got %v", err)
	}
}

func TestTickFlowFetchSectorRankingsDoesNotMislabelConceptsAsSW1(t *testing.T) {
	provider := NewTickFlowConfigured(2*time.Second, "http://127.0.0.1", "", "test-api-key")
	if _, err := provider.FetchSectorRankings(context.Background(), "concept", 10); err == nil ||
		!strings.Contains(err.Error(), "only SW1 industry rankings") {
		t.Fatalf("TickFlow SW1 ranking must not claim concept coverage, got %v", err)
	}
}
