package pythonapi

import (
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("PYTHON_API_BASE_URL", "http://python-service:8000/prefix")
	t.Setenv("PYTHON_API_TIMEOUT", "7s")
	t.Setenv("PYTHON_API_HISTORY_DAYS", "120")
	t.Setenv("PYTHON_API_MAX_CONCURRENCY", "3")
	t.Setenv("PYTHON_API_MAX_RESPONSE_BYTES", "4096")
	t.Setenv("PYTHON_API_INDEX_SYMBOLS", "sh000001, SH000001, 930606.CSI")
	t.Setenv("PYTHON_API_MARKET_TIMEZONE", "Asia/Shanghai")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://python-service:8000/prefix" || cfg.Timeout.String() != "7s" || cfg.HistoryDays != 120 || cfg.MaxConcurrency != 3 || cfg.MaxResponseBytes != 4096 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if len(cfg.IndexSymbols) != 2 || cfg.IndexSymbols[0] != "sh000001" || cfg.IndexSymbols[1] != "930606.CSI" {
		t.Fatalf("index symbols were not normalized/deduplicated: %#v", cfg.IndexSymbols)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("PYTHON_API_HISTORY_DAYS", "500")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected out-of-range history days error")
	}
}
