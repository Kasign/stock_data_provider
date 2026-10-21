package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setDefaultProviderConfig(t *testing.T) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "providers.json")
	data := `{"provider_priority":["eastmoney","tencent"],"adjustment_basis":"forward_adjusted","providers":{"eastmoney":{"enabled":true,"quote":true,"daily_bars":true},"tencent":{"enabled":true,"quote":true,"daily_bars":true}}}`
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKET_DATA_PROVIDERS_CONFIG", file)
}

func TestLoadProviderConfigAndEnvironmentOverrides(t *testing.T) {
	file := filepath.Join(t.TempDir(), "providers.json")
	data := `{"provider_priority":["eastmoney","tencent"],"adjustment_basis":"backward_adjusted","providers":{"eastmoney":{"enabled":true,"quote":false,"daily_bars":true},"tencent":{"enabled":true,"quote":true,"daily_bars":false}}}`
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKET_DATA_PROVIDERS_CONFIG", file)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_DEFAULT_ADJUSTMENT", "")
	t.Setenv("MARKET_DATA_EASTMONEY_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Adjustment != "backward_adjusted" {
		t.Fatalf("adjustment=%s", cfg.Adjustment)
	}
	if cfg.ProviderSettings["eastmoney"].Quote || !cfg.ProviderSettings["eastmoney"].DailyBars {
		t.Fatalf("Eastmoney capabilities not loaded: %+v", cfg.ProviderSettings["eastmoney"])
	}
	if !cfg.ProviderSettings["tencent"].Quote || cfg.ProviderSettings["tencent"].DailyBars {
		t.Fatalf("Tencent capabilities not loaded: %+v", cfg.ProviderSettings["tencent"])
	}
	t.Setenv("MARKET_DATA_EASTMONEY_QUOTE_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["eastmoney"].Quote {
		t.Fatal("environment override should take precedence over provider config")
	}
}

func TestLoadRejectsInvalidDurationInsteadOfSilentlyDefaulting(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "fast")
	if _, err := Load(); err == nil {
		t.Fatal("invalid duration must fail configuration loading")
	}
}

func TestLoadRejectsPlaceholderSecrets(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "replace-with-a-long-random-secret")
	if _, err := Load(); err == nil {
		t.Fatal("placeholder API key must not be accepted")
	}
}

func TestLoadRequiresStorageUnlessExplicitlyDisabledForSmokeTest(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "false")
	t.Setenv("MARKET_DATA_MYSQL_DSN", "")
	t.Setenv("MARKET_DATA_REDIS_ADDR", "")
	if _, err := Load(); err == nil {
		t.Fatal("storage must be required by default")
	}
}

func TestLoadRejectsUnknownProviderConfigField(t *testing.T) {
	file := filepath.Join(t.TempDir(), "providers.json")
	if err := os.WriteFile(file, []byte(`{"provider_priority":["eastmoney"],"typo":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKET_DATA_PROVIDERS_CONFIG", file)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("unknown provider config fields must fail instead of being silently ignored")
	}
}

func TestLoadRejectsRetryValuesOutsideSupportedRange(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "6")
	if _, err := Load(); err == nil {
		t.Fatal("provider retries above 5 must fail configuration loading")
	}
}

func TestLoadRejectsNonIntegerRetryConfiguration(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "twice")
	if _, err := Load(); err == nil {
		t.Fatal("non-integer retry configuration must fail instead of silently using a default")
	}
}

func TestLoadRejectsRequestTimeoutTooShortForProviderRetriesAndFailover(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "25s")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	if _, err := Load(); err == nil {
		t.Fatal("request timeout must cover provider retries and failover rather than starving the fallback provider")
	}
}

func TestLoadDefaultTimeoutCoversDefaultTwoProviderRetryBudget(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	if _, err := Load(); err != nil {
		t.Fatalf("default request timeout should allow both configured providers to be tried: %v", err)
	}
}

func TestLoadRejectsExplicitMissingProviderConfig(t *testing.T) {
	t.Setenv("MARKET_DATA_PROVIDERS_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("explicitly configured missing provider file must fail closed")
	}
}

func TestLoadDefaultTimeoutAccountsForSequentialBundleBudget(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "60s")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	if _, err := Load(); err == nil {
		t.Fatal("60s is insufficient for sequential quote + daily-bar failover budgets")
	}
}

func TestLoadRejectsMalformedProviderEndpointURL(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_TENCENT_QUOTE_URL", "file:///tmp/quote")
	if _, err := Load(); err == nil {
		t.Fatal("non-http provider endpoint must fail during configuration validation")
	}
}

func TestLoadRejectsProviderEndpointWithInlineQuery(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_QUOTE_URL", "https://example.test/quote?token=secret")
	if _, err := Load(); err == nil {
		t.Fatal("query parameters embedded in configured endpoint must not be silently replaced")
	}
}

func TestSplitListDoesNotDuplicateProviderNames(t *testing.T) {
	got := splitList("eastmoney, tencent,, EASTMONEY ")
	want := []string{"eastmoney", "tencent", "eastmoney"}
	if len(got) != len(want) {
		t.Fatalf("splitList returned duplicate entries unexpectedly: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitList[%d]=%q want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}
