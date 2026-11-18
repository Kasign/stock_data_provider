package bootstrap

import (
	"reflect"
	"testing"

	marketdataconfig "example.com/stock-analysis-flow/internal/marketdata/config"
)

func TestParseMarketDataIndexSymbolsPreservesDefaultsAndDeduplicates(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "默认指数", raw: "", want: []string{"000001.SH"}},
		{name: "空白配置回退默认指数", raw: " , , ", want: []string{"000001.SH"}},
		{name: "配置去重", raw: "000001.SH, 399001.SZ,000001.sh", want: []string{"000001.SH", "399001.SZ"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMarketDataIndexSymbols(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseMarketDataIndexSymbols(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMarketDataProviderActiveRequiresEnabledCapability(t *testing.T) {
	cfg := marketdataconfig.Config{
		ProviderPriority: []string{"eastmoney", "sina", "tencent"},
		ProviderSettings: map[string]marketdataconfig.ProviderSettings{
			"eastmoney": {Enabled: true},
			"sina":      {Enabled: true, Quote: true},
			"tencent":   {Enabled: false, Quote: true},
		},
	}
	if marketDataProviderActive(cfg, "eastmoney") {
		t.Fatal("enabled provider without a capability must not create an egress binding")
	}
	if !marketDataProviderActive(cfg, "sina") {
		t.Fatal("enabled provider with a quote capability must create an egress binding")
	}
	if marketDataProviderActive(cfg, "tencent") {
		t.Fatal("disabled provider must not create an egress binding")
	}
}
