package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/marketdata"
	"example.com/stock-analysis-flow/internal/adapters/pythonapi"
	"example.com/stock-analysis-flow/internal/analysis/domain"
)

type memoryStore struct {
	requests  []domain.RunRequest
	result    domain.RunResult
	events    []domain.StageEvent
	raw       int
	market    int
	portfolio int
}

func (m *memoryStore) Begin(_ context.Context, r domain.RunRequest, _ time.Time) error {
	m.requests = append(m.requests, r)
	return nil
}
func (m *memoryStore) AppendEvent(_ context.Context, e domain.StageEvent) error {
	m.events = append(m.events, e)
	return nil
}
func (m *memoryStore) Finish(_ context.Context, r domain.RunResult) error { m.result = r; return nil }
func (m *memoryStore) SaveRaw(_ context.Context, runID string, s domain.RawSnapshot) (string, error) {
	m.raw++
	return runID + "-raw", nil
}
func (m *memoryStore) SaveMarket(_ context.Context, runID string, s domain.MarketSnapshot) (string, error) {
	m.market++
	return runID + "-market", nil
}
func (m *memoryStore) SavePortfolio(_ context.Context, runID string, s domain.PortfolioSnapshot) (string, error) {
	m.portfolio++
	return runID + "-portfolio", nil
}

func TestRunnerWithStoresRunsRealAdapterFlowAndFailsClosed(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 15, 5, 0, 0, loc)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/stocks/600519/quote", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"stock_code": "600519", "stock_name": "测试股票", "current_price": 121.0, "update_time": "2026-10-20 14:52:00"})
	})
	mux.HandleFunc("/api/v1/stocks/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/history") {
			http.NotFound(w, r)
			return
		}
		code := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/stocks/"), "/history")
		data := make([]map[string]any, 0, 21)
		for i := 20; i >= 0; i-- {
			d := asOf.AddDate(0, 0, -i).Format("2006-01-02")
			closePrice := float64(100 + 20 - i)
			data = append(data, map[string]any{"date": d, "open": closePrice * 0.995, "high": closePrice * 1.01, "low": closePrice * 0.99, "close": closePrice, "volume": float64(1000 + i), "amount": float64(100000 + i)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"stock_code": code, "stock_name": "测试", "period": "daily", "data": data})
	})
	upstream := httptest.NewServer(mux)
	defer upstream.Close()
	cfg := pythonTestConfig(upstream.URL)
	store := &memoryStore{}
	runner, err := NewRunnerWithStores(cfg, store, store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), domain.RunRequest{RunID: "integration-test-run", TaskType: domain.TaskPostMarketScan, AsOf: asOf, UniverseID: "test-universe", Symbols: []string{"600519"}, PortfolioID: "unconfigured", MarketID: "CN-A", StrategyIDs: []string{StrategyTrendFollowing}, StrategyConfigVersion: "strategy-v1", RiskConfigVersion: "risk-v1", TradingRulesVersion: "rules-v1", DataPolicyVersion: "quality-v1"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Quality.CorePass != true {
		t.Fatalf("expected core quality gate pass, issues=%+v", result.Quality.Issues)
	}
	if result.Status != domain.RunStatusPartial {
		t.Fatalf("expected partial due incomplete upstream metadata, got %s", result.Status)
	}
	if len(result.Signals) != 1 {
		t.Fatalf("expected one trend candidate, got %d", len(result.Signals))
	}
	if len(result.Tradability) != 1 || result.Tradability[0].Allowed {
		t.Fatalf("missing fail-closed tradability decision: %+v", result.Tradability)
	}
	if len(result.Plans) != 0 {
		t.Fatalf("unsafe MVP must not produce a trade plan: %+v", result.Plans)
	}
	if store.raw != 1 || store.market != 1 || store.portfolio != 1 {
		t.Fatalf("expected raw/market/portfolio persistence, got %d/%d/%d", store.raw, store.market, store.portfolio)
	}
	if len(store.events) == 0 || store.result.RunID != result.RunID {
		t.Fatalf("run audit/result not persisted")
	}
	stages := make([]string, 0, len(store.events))
	for _, e := range store.events {
		stages = append(stages, e.Stage)
	}
	sort.Strings(stages)
	if !containsStage(stages, "quality_gate") || !containsStage(stages, "strategy_evaluation") || !containsStage(stages, "portfolio_planning") {
		t.Fatalf("expected key stages, got %v", stages)
	}
}
func pythonTestConfig(base string) pythonapi.Config {
	c := pythonapi.DefaultConfig()
	c.BaseURL = base
	c.IndexSymbols = []string{"sh000001"}
	c.HistoryDays = 200
	return c
}
func containsStage(stages []string, want string) bool {
	for _, s := range stages {
		if s == want {
			return true
		}
	}
	return false
}

func TestUniverseResolverRejectsNonSixDigitSymbols(t *testing.T) {
	r := universeResolver{}
	_, err := r.Resolve(context.Background(), domain.RunRequest{AsOf: time.Now(), UniverseID: "u", Symbols: []string{"600519.SH"}})
	if err == nil {
		t.Fatal("expected suffixed symbol to be rejected; this adapter expects six digit A-share codes")
	}
}

func TestRunnerWithMarketDataStoresUsesNativeGoAPI(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 15, 5, 0, 0, loc)
	const apiKey = "01234567890123456789012345678901"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/quotes/600519.SH", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != apiKey {
			t.Errorf("native market-data API key missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"symbol": "600519.SH", "name": "贵州茅台", "current_price": 120.0,
			"change": 1.0, "change_percent": 0.84, "open": 119.0, "high": 121.0,
			"low": 118.0, "prev_close": 119.0, "volume": 10000, "amount": 1200000,
			"quote_time": "2026-10-20T07:00:00Z", "fetched_at": "2026-10-20T07:00:01Z",
			"source": "eastmoney", "source_version": "v1", "quality_status": "valid",
			"quality_reasons": []string{}, "volume_unit": "shares", "amount_unit": "CNY",
			"adjustment_basis": "none",
		})
	})
	mux.HandleFunc("/api/v1/bars/600519.SH/daily", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != apiKey {
			t.Errorf("native market-data API key missing")
		}
		if r.URL.Query().Get("adjustment") != "forward_adjusted" {
			t.Errorf("unexpected adjustment query %q", r.URL.RawQuery)
		}
		rows := make([]map[string]any, 0, 21)
		for i := 20; i >= 0; i-- {
			closePrice := float64(100 + 20 - i)
			rows = append(rows, map[string]any{
				"symbol": "600519.SH", "trade_date": asOf.AddDate(0, 0, -i).Format("2006-01-02"),
				"period": "1d", "open": closePrice * 0.995, "high": closePrice * 1.01,
				"low": closePrice * 0.99, "close": closePrice, "volume": 10000 + i,
				"amount": 1000000 + i, "change_percent": 0.1,
				"adjustment_basis": "forward_adjusted", "source": "eastmoney",
				"source_version": "v1", "provider_timestamp": "2026-10-20T07:00:00Z",
				"fetched_at": "2026-10-20T07:00:01Z", "normalized_at": "2026-10-20T07:00:01Z",
				"volume_unit": "shares", "amount_unit": "CNY", "quality_status": "valid",
				"quality_reasons": []string{},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"symbol": "600519.SH", "period": "1d", "adjustment_basis": "forward_adjusted",
			"volume_unit": "shares", "amount_unit": "CNY", "quality_status": "valid",
			"warnings": []string{}, "bars": rows,
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := marketdata.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.APIKey = apiKey
	cfg.IndexSymbols = nil
	store := &memoryStore{}
	runner, err := NewRunnerWithMarketDataStores(cfg, store, store)
	if err != nil {
		t.Fatalf("wire native Go market-data runner: %v", err)
	}
	result, err := runner.Run(context.Background(), domain.RunRequest{
		RunID: "native-market-data-run", TaskType: domain.TaskPostMarketScan, AsOf: asOf,
		UniverseID: "test-universe", Symbols: []string{"600519"}, PortfolioID: "unconfigured",
		MarketID: "CN-A", StrategyIDs: []string{StrategyTrendFollowing},
		StrategyConfigVersion: "strategy-v1", RiskConfigVersion: "risk-v1",
		TradingRulesVersion: "rules-v1", DataPolicyVersion: "quality-v1",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Quality.CorePass {
		t.Fatalf("expected core quality gate pass, issues=%+v", result.Quality.Issues)
	}
	if result.Status != domain.RunStatusPartial {
		t.Fatalf("expected partial status due missing precise available_at metadata, got %s", result.Status)
	}
	if len(result.Signals) != 1 {
		t.Fatalf("expected one trend candidate from Go market-data API, got %d", len(result.Signals))
	}
	if store.raw != 1 || store.market != 1 || store.portfolio != 1 {
		t.Fatalf("expected raw/market/portfolio persistence, got %d/%d/%d", store.raw, store.market, store.portfolio)
	}
}
