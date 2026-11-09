package bootstrap

import (
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

type egressTestDoer struct{}

func (egressTestDoer) Do(*http.Request) (*http.Response, error) { return nil, nil }

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
		ProviderPriority:  []string{"eastmoney"},
		ProviderSettings:  map[string]config.ProviderSettings{"eastmoney": {Enabled: true}},
		EastmoneyQuoteURL: "https://quote.example.test/api",
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
