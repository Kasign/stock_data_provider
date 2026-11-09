package bootstrap

import (
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

type egressHTTPDoerStub struct{}

func (egressHTTPDoerStub) Do(*http.Request) (*http.Response, error) { return nil, nil }

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
		ProviderSettings: map[string]config.ProviderSettings{"tushare": {Enabled: true, DailyBars: true}},
	}
	err := validateEgressDependencies(cfg, Dependencies{})
	if err == nil || !strings.Contains(err.Error(), "tushare HTTP") {
		t.Fatalf("expected missing Tushare HTTP dependency, got %v", err)
	}
}

func TestValidateEgressDependenciesChecksEastmoneyOrigins(t *testing.T) {
	cfg := config.Config{
		ProviderPriority:          []string{"eastmoney"},
		ProviderSettings:          map[string]config.ProviderSettings{"eastmoney": {Enabled: true, Quote: true}},
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
			"baostock": {Enabled: true, DailyBars: true},
			"pytdx":    {Enabled: true, DailyBars: true},
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
		ProviderSettings: map[string]config.ProviderSettings{"eastmoney": {Enabled: true, Quote: true}},
	}
	err := validateEgressDependencies(cfg, Dependencies{EastmoneyHTTPDoers: map[string]provider.HTTPDoer{}})
	if err == nil || !strings.Contains(err.Error(), "eastmoney.com") {
		t.Fatalf("expected missing default Eastmoney origin, got %v", err)
	}
	if strings.Contains(err.Error(), "invalid Eastmoney endpoint origin") {
		t.Fatalf("default endpoint was not resolved before validation: %v", err)
	}
}

func TestValidateEgressDependenciesSkipsProviderWithoutCapabilities(t *testing.T) {
	cfg := config.Config{
		ProviderPriority: []string{"tushare", "eastmoney"},
		ProviderSettings: map[string]config.ProviderSettings{
			"tushare": {Enabled: true}, "eastmoney": {Enabled: true},
		},
	}
	if err := validateEgressDependencies(cfg, Dependencies{}); err != nil {
		t.Fatalf("providers without enabled capabilities must not require egress dependencies: %v", err)
	}
}

func TestHasConfiguredProviderCapability(t *testing.T) {
	if hasConfiguredProviderCapability(config.ProviderSettings{}) {
		t.Fatal("empty provider settings must not report an enabled capability")
	}
	if !hasConfiguredProviderCapability(config.ProviderSettings{DailyBars: true}) {
		t.Fatal("DailyBars must be recognized as an enabled capability")
	}
	if !hasConfiguredProviderCapability(config.ProviderSettings{Announcements: true}) {
		t.Fatal("Announcements must be recognized as an enabled capability")
	}
}

func TestValidateEgressDependenciesChecksOnlyEnabledTencentEndpoints(t *testing.T) {
	cfg := config.Config{
		ProviderPriority: []string{"tencent"},
		ProviderSettings: map[string]config.ProviderSettings{
			"tencent": {Enabled: true, Quote: true},
		},
	}
	if err := validateEgressDependencies(cfg, Dependencies{}); err == nil ||
		!strings.Contains(err.Error(), "tencent quote HTTP") {
		t.Fatalf("expected missing quote executor, got %v", err)
	}
	deps := Dependencies{TencentQuoteHTTPDoer: egressHTTPDoerStub{}}
	if err := validateEgressDependencies(cfg, deps); err != nil {
		t.Fatalf("quote-only Tencent must not require kline/search executors: %v", err)
	}
}

func TestValidateEgressDependenciesChecksOnlyEnabledSinaAndNeteaseEndpoints(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		deps Dependencies
	}{
		{
			name: "sina quote only",
			cfg:  config.Config{ProviderPriority: []string{"sina"}, ProviderSettings: map[string]config.ProviderSettings{"sina": {Enabled: true, Quote: true}}},
			deps: Dependencies{SinaQuoteHTTPDoer: egressHTTPDoerStub{}},
		},
		{
			name: "netease daily bars only",
			cfg:  config.Config{ProviderPriority: []string{"netease"}, ProviderSettings: map[string]config.ProviderSettings{"netease": {Enabled: true, DailyBars: true}}},
			deps: Dependencies{NeteaseHistoryHTTPDoer: egressHTTPDoerStub{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateEgressDependencies(tt.cfg, tt.deps); err != nil {
				t.Fatalf("disabled endpoints must not require executors: %v", err)
			}
		})
	}
}

func TestValidateEgressDependenciesChecksTickFlowRequestModes(t *testing.T) {
	tests := []struct {
		name     string
		settings config.ProviderSettings
		apiKey   string
		deps     Dependencies
		wantErr  string
	}{
		{name: "quote requires authenticated executor only", settings: config.ProviderSettings{Enabled: true, Quote: true}, deps: Dependencies{TickFlowAuthenticatedHTTPDoer: egressHTTPDoerStub{}}},
		{name: "daily bars without key requires public executor only", settings: config.ProviderSettings{Enabled: true, DailyBars: true}, deps: Dependencies{TickFlowPublicHTTPDoer: egressHTTPDoerStub{}}},
		{name: "daily bars with key requires authenticated executor only", settings: config.ProviderSettings{Enabled: true, DailyBars: true}, apiKey: "configured-key", deps: Dependencies{TickFlowAuthenticatedHTTPDoer: egressHTTPDoerStub{}}},
		{name: "daily bars without public executor fails", settings: config.ProviderSettings{Enabled: true, DailyBars: true}, wantErr: "tickflow public HTTP"},
		{name: "quote without authenticated executor fails", settings: config.ProviderSettings{Enabled: true, Quote: true}, deps: Dependencies{TickFlowPublicHTTPDoer: egressHTTPDoerStub{}}, wantErr: "tickflow authenticated HTTP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{ProviderPriority: []string{"tickflow"}, ProviderSettings: map[string]config.ProviderSettings{"tickflow": tt.settings}, TickFlowAPIKey: tt.apiKey}
			err := validateEgressDependencies(cfg, tt.deps)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected egress validation error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("expected %q error, got %v", tt.wantErr, err)
			}
		})
	}
}
