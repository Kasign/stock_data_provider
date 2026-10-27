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

var tickflowBreadthFixtureSymbols = []string{"600000.SH", "000001.SZ", "300001.SZ"}

func newTickFlowBreadthFixture(t *testing.T, quoteSymbols []string, omitAmountFor string) *httptest.Server {
	t.Helper()
	timestamp := time.Now().UTC().Add(-time.Minute).UnixMilli()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Errorf("x-api-key header was not forwarded")
		}
		switch r.URL.Path {
		case "/v1/universes/CN_Equity_A":
			if r.Method != http.MethodGet {
				t.Errorf("universe method = %s, want GET", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": "CN_Equity_A", "symbol_count": len(tickflowBreadthFixtureSymbols),
					"symbols": tickflowBreadthFixtureSymbols,
				},
			})
		case "/v1/quotes":
			if r.Method != http.MethodGet || r.URL.Query().Get("universes") != "CN_Equity_A" {
				t.Errorf("unexpected universe quote request: %s %s", r.Method, r.URL.String())
			}
			rows := make([]map[string]any, 0, len(quoteSymbols))
			for _, symbol := range quoteSymbols {
				current, previous := 10.0, 10.0
				switch symbol {
				case "600000.SH":
					current, previous = 11, 10
				case "000001.SZ":
					current, previous = 9, 10
				}
				row := map[string]any{
					"symbol": symbol, "last_price": current, "prev_close": previous,
					"open": current, "high": current, "low": current, "timestamp": timestamp,
					"ext": map[string]any{"limit_up": 11.0, "limit_down": 9.0},
				}
				if symbol != omitAmountFor {
					switch symbol {
					case "600000.SH":
						row["amount"] = 100.0
					case "000001.SZ":
						row["amount"] = 200.0
					default:
						row["amount"] = 300.0
					}
				}
				rows = append(rows, row)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTickFlowFetchMarketBreadthCalculatesCompleteSnapshot(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols, "")
	defer server.Close()

	client := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := client.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalSecurities != 3 || got.CoveredSecurities != 3 {
		t.Fatalf("unexpected market coverage: %+v", got)
	}
	if got.AdvancingCount != 1 || got.DecliningCount != 1 || got.UnchangedCount != 1 || got.UnclassifiedCount != 0 {
		t.Fatalf("unexpected breadth counts: %+v", got)
	}
	if got.TotalAmountCNY == nil || *got.TotalAmountCNY != 600 || got.AmountCoverageCount != 3 {
		t.Fatalf("unexpected total amount: %+v", got)
	}
	if got.LimitUpCount == nil || *got.LimitUpCount != 1 || got.LimitDownCount == nil || *got.LimitDownCount != 1 {
		t.Fatalf("unexpected limit counts: %+v", got)
	}
	if got.ProviderReportDate == "" || got.QualityStatus != model.QualityValid {
		t.Fatalf("expected a valid report date and quality status: %+v", got)
	}
}

func TestTickFlowFetchMarketBreadthRejectsIncompleteQuoteCoverage(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols[:2], "")
	defer server.Close()

	client := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	if _, err := client.FetchMarketBreadth(context.Background()); err == nil || !strings.Contains(err.Error(), "coverage mismatch") {
		t.Fatalf("incomplete universe quote coverage must fail for fallback, got %v", err)
	}
}

func TestTickFlowFetchMarketBreadthDegradesMissingAmount(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols, "000001.SZ")
	defer server.Close()

	client := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := client.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityDegraded || got.TotalAmountCNY != nil || got.AmountCoverageCount != 2 {
		t.Fatalf("partial amount coverage must remain degraded and have no total: %+v", got)
	}
	if !containsTickFlowReason(got.QualityReasons, "provider_amount_missing_or_invalid") {
		t.Fatalf("missing amount reason is not present: %+v", got.QualityReasons)
	}
}
