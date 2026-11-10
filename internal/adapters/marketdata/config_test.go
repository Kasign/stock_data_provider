package marketdata

import (
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestLoadConfigUsesExplicitSource(t *testing.T) {
	values := map[string]string{
		"MARKET_DATA_BASE_URL": "https://market.internal",
		"MARKET_DATA_API_KEY": "0123456789abcdef0123456789abcdef",
		"MARKET_DATA_HISTORY_DAYS": "120",
		"MARKET_DATA_MAX_CONCURRENCY": "8",
		"MARKET_DATA_INDEX_SYMBOLS": "000001.SH,399001.SZ,000001.SH",
		"MARKET_DATA_ADJUSTMENT": "none",
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.BaseURL != "https://market.internal" || cfg.HistoryDays != 120 ||
		cfg.MaxConcurrency != 8 || cfg.Adjustment != "none" ||
		len(cfg.IndexSymbols) != 2 || cfg.APIKey != values["MARKET_DATA_API_KEY"] {
		t.Fatalf("explicit source was not applied correctly: %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidExplicitSource(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == "MARKET_DATA_API_KEY" {
			return "0123456789abcdef0123456789abcdef", true
		}
		if key == "MARKET_DATA_HISTORY_DAYS" {
			return "366", true
		}
		return "", false
	})
	if _, err := LoadConfig(env); err == nil {
		t.Fatal("LoadConfig() error = nil, want history days validation error")
	}
}
