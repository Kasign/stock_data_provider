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

func newTickFlowBreadthFixture(t *testing.T, quoteSymbols []string, omitAmountFor, omitNameFor string, timestampSkew ...time.Duration) *httptest.Server {
	t.Helper()
	now := time.Now().UTC()
	timestampTime := now.Add(-time.Minute)
	var skewMillis int64
	if len(timestampSkew) > 0 {
		skewMillis = timestampSkew[0].Milliseconds()
		// Near China midnight, move the fixture's anchor to yesterday 23:59 so
		// an intentional timestamp skew cannot accidentally span two report dates.
		if skewMillis > 0 {
			location, err := time.LoadLocation("Asia/Shanghai")
			if err != nil {
				location = time.FixedZone("CST", 8*60*60)
			}
			localNow := now.In(location)
			midnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
			if localNow.Sub(midnight) <= time.Duration(skewMillis)*time.Millisecond+time.Minute {
				timestampTime = midnight.Add(-time.Minute)
			}
		}
	}
	timestamp := timestampTime.UnixMilli()
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
				name := "测试A股"
				switch symbol {
				case "600000.SH":
					name = "浦发银行"
				case "000001.SZ":
					name = "平安银行"
				case "300001.SZ":
					name = "创业板样本"
				}
				rowTimestamp := timestamp
				if symbol == "000001.SZ" && skewMillis > 0 {
					rowTimestamp -= skewMillis
				}
				row := map[string]any{
					"symbol": symbol, "last_price": current, "prev_close": previous,
					"open": current, "high": current, "low": current, "timestamp": rowTimestamp,
				}
				if symbol != omitNameFor {
					row["name"] = name
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
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols, "", "")
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
	if got.QuoteTimestampCoverageCount != 3 || got.OldestQuoteTime == nil || got.NewestQuoteTime == nil ||
		got.NewestQuoteTime.Sub(*got.OldestQuoteTime) != 0 {
		t.Fatalf("complete fixture must expose a complete, aligned timestamp range: %+v", got)
	}
}


func TestTickFlowFetchMarketBreadthDegradesWideQuoteTimestampSpread(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols, "", "", 11*time.Minute)
	defer server.Close()

	client := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := client.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityDegraded ||
		got.OldestQuoteTime == nil || got.NewestQuoteTime == nil ||
		got.NewestQuoteTime.Sub(*got.OldestQuoteTime) != 11*time.Minute {
		t.Fatalf("wide timestamp spread must be exposed and degrade quality: %+v", got)
	}
	if !containsTickFlowReason(got.QualityReasons, "provider_quote_timestamp_spread_exceeds_10_minutes") {
		t.Fatalf("missing timestamp spread quality reason: %v", got.QualityReasons)
	}
}

func TestTickFlowFetchMarketBreadthRejectsIncompleteQuoteCoverage(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols[:2], "", "")
	defer server.Close()

	client := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	if _, err := client.FetchMarketBreadth(context.Background()); err == nil || !strings.Contains(err.Error(), "coverage mismatch") {
		t.Fatalf("incomplete universe quote coverage must fail for fallback, got %v", err)
	}
}

func TestTickFlowFetchMarketBreadthDegradesMissingAmount(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols, "000001.SZ", "")
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


func TestTickFlowLimitRatioMatchesAShareRules(t *testing.T) {
	cases := []struct {
		name      string
		symbol    string
		stockName string
		wantRatio float64
		wantKnown bool
	}{
		{name: "Beijing new-format code", symbol: "920000.BJ", stockName: "普通股票", wantRatio: 0.30, wantKnown: true},
		{name: "historical Beijing code", symbol: "830001.BJ", stockName: "普通股票", wantRatio: 0.30, wantKnown: true},
		{name: "ChiNext", symbol: "300750.SZ", stockName: "普通股票", wantRatio: 0.20, wantKnown: true},
		{name: "STAR Market", symbol: "688001.SH", stockName: "普通股票", wantRatio: 0.20, wantKnown: true},
		{name: "ST main board", symbol: "600000.SH", stockName: "*ST示例", wantRatio: 0.05, wantKnown: true},
		{name: "ordinary main board", symbol: "600000.SH", stockName: "浦发银行", wantRatio: 0.10, wantKnown: true},
		{name: "main-board name missing", symbol: "600000.SH", stockName: "", wantKnown: false},
		{name: "B share outside A-share contract", symbol: "900001.SH", stockName: "B股", wantKnown: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, known := tickflowLimitRatio(tc.symbol, tc.stockName)
			if known != tc.wantKnown {
				t.Fatalf("tickflowLimitRatio(%q, %q) known=%v, want %v", tc.symbol, tc.stockName, known, tc.wantKnown)
			}
			if known && got != tc.wantRatio {
				t.Fatalf("tickflowLimitRatio(%q, %q)=%v, want %v", tc.symbol, tc.stockName, got, tc.wantRatio)
			}
		})
	}
}

func TestTickFlowLimitPriceMatchesCentRoundingAndTolerance(t *testing.T) {
	upper, upperTolerance, upperOK := tickflowLimitPrice(10.05, 0.10, true)
	if !upperOK || upper != 11.06 || upperTolerance < 0.0049 || upperTolerance > 0.0051 {
		t.Fatalf("unexpected upper limit calculation: price=%v tolerance=%v valid=%v", upper, upperTolerance, upperOK)
	}
	lower, lowerTolerance, lowerOK := tickflowLimitPrice(10.05, 0.10, false)
	if !lowerOK || lower != 9.05 || lowerTolerance < 0.0049 || lowerTolerance > 0.0051 {
		t.Fatalf("unexpected lower limit calculation: price=%v tolerance=%v valid=%v", lower, lowerTolerance, lowerOK)
	}
	if _, _, ok := tickflowLimitPrice(0, 0.10, true); ok {
		t.Fatal("non-positive previous close must be rejected")
	}
}


func TestTickFlowFetchMarketBreadthDegradesUnavailableLimitCounts(t *testing.T) {
	server := newTickFlowBreadthFixture(t, tickflowBreadthFixtureSymbols, "", "000001.SZ")
	defer server.Close()

	client := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-api-key")
	got, err := client.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityDegraded || got.LimitUpCount != nil || got.LimitDownCount != nil {
		t.Fatalf("missing ST-classification metadata must leave limit counts unknown and degrade the snapshot: %+v", got)
	}
	if !containsTickFlowReason(got.QualityReasons, "provider_limit_counts_unavailable_or_incomplete") {
		t.Fatalf("missing limit-count degradation reason: %+v", got.QualityReasons)
	}
}
