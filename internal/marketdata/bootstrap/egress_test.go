package bootstrap

import (
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

func TestValidateEgressDependenciesOnlyRequiresEnabledProviders(t *testing.T) {
	cfg := config.Config{
		ProviderPriority: []string{"tushare", "pytdx"},
		ProviderSettings: map[string]config.ProviderSettings{
			"tushare": {Enabled: false},
			"pytdx":   {Enabled: false},
		},
	}
	if err := validateEgressDependencies(cfg, Dependencies{}); err != nil {
		t.Fatalf("disabled providers must not require egress dependencies: %v", err)
	}
}

func TestValidateEgressDependenciesRequiresEnabledProviderInjection(t *testing.T) {
	cfg := config.Config{
		ProviderPriority: []string{"tushare"},
		ProviderSettings: map[string]config.ProviderSettings{"tushare": {Enabled: true}},
	}
	err := validateEgressDependencies(cfg, Dependencies{})
	if err == nil || !strings.Contains(err.Error(), "tushare HTTP") {
		t.Fatalf("expected missing Tushare HTTP dependency, got %v", err)
	}
}

func TestValidateEgressDependenciesChecksEastmoneyOrigins(t *testing.T) {
	cfg := config.Config{
		ProviderPriority:          []string{"eastmoney"},
		ProviderSettings:          map[string]config.ProviderSettings{"eastmoney": {Enabled: true}},
		EastmoneyQuoteURL:         "https://quote.example.test/api",
		EastmoneyKlineURL:         "https://quote.example.test/kline",
		EastmoneyStockBasicURL:    "https://quote.example.test/basic",
		EastmoneySectorRankingURL: "https://quote.example.test/sector",
		EastmoneyIndexURL:         "https://quote.example.test/index",
		EastmoneyMarketStocksURL:  "https://quote.example.test/market",
		EastmoneyLimitPoolBaseURL: "https://quote.example.test",
		EastmoneyStockBoardsURL:   "https://quote.example.test/boards",
		EastmoneyDragonTigerURL:   "https://quote.example.test/dragon",
		EastmoneyAnnouncementsURL: "https://quote.example.test/announcements",
		EastmoneyHotRankingsURL:   "https://quote.example.test/hot",
		EastmoneyHotSurgeURL:      "https://quote.example.test/surge",
		EastmoneyCapitalFlowURL:   "https://quote.example.test/flow",
	}
	if err := validateEgressDependencies(cfg, Dependencies{EastmoneyHTTPDoers: map[string]provider.HTTPDoer{}}); err == nil ||
		!strings.Contains(err.Error(), "quote.example.test") {
		t.Fatalf("expected missing Eastmoney origin, got %v", err)
	}
}

func TestValidateEgressDependenciesRequiresTCPDialers(t *testing.T) {
	cfg := config.Config{
		ProviderPriority: []string{"baostock", "pytdx"},
		ProviderSettings: map[string]config.ProviderSettings{
			"baostock": {Enabled: true},
			"pytdx":    {Enabled: true},
		},
	}
	if err := validateEgressDependencies(cfg, Dependencies{}); err == nil ||
		!strings.Contains(err.Error(), "baostock TCP") {
		t.Fatalf("expected missing BaoStock dialer, got %v", err)
	}
}

func TestValidateEgressDependenciesUsesEffectiveEastmoneyDefaults(t *testing.T) {
	cfg := config.Config{
		ProviderPriority: []string{"eastmoney"},
		ProviderSettings: map[string]config.ProviderSettings{"eastmoney": {Enabled: true}},
	}
	err := validateEgressDependencies(cfg, Dependencies{EastmoneyHTTPDoers: map[string]provider.HTTPDoer{}})
	if err == nil || !strings.Contains(err.Error(), "push2.eastmoney.com") {
		t.Fatalf("expected missing default Eastmoney origin, got %v", err)
	}
	if strings.Contains(err.Error(), "invalid Eastmoney endpoint origin") {
		t.Fatalf("default endpoint was not resolved before validation: %v", err)
	}
}
