package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
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
	t.Setenv("MARKET_DATA_ADJUSTMENT", "")
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

func TestLoadDefaultTimeoutCoversBaoStockFailoverBudget(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,tencent,sina")
	t.Setenv("MARKET_DATA_BAOSTOCK_ENABLED", "true")
	t.Setenv("MARKET_DATA_BAOSTOCK_QUOTE_ENABLED", "false")
	t.Setenv("MARKET_DATA_BAOSTOCK_DAILY_BARS_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("default request timeout should cover BaoStock failover budget: %v", err)
	}
	if cfg.RequestTimeout < 3*time.Minute {
		t.Fatalf("default request timeout=%s, want at least 3m for BaoStock-enabled failover", cfg.RequestTimeout)
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

func TestLoadSinaQuoteOnlyProvider(t *testing.T) {
	file := filepath.Join(t.TempDir(), "providers.json")
	data := `{"provider_priority":["eastmoney","tencent","sina"],"adjustment_basis":"forward_adjusted","providers":{"eastmoney":{"enabled":true,"quote":true,"daily_bars":true},"tencent":{"enabled":true,"quote":true,"daily_bars":true},"sina":{"enabled":true,"quote":true,"daily_bars":false}}}`
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKET_DATA_PROVIDERS_CONFIG", file)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "130s")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_SINA_ENABLED", "")
	t.Setenv("MARKET_DATA_SINA_QUOTE_ENABLED", "")
	t.Setenv("MARKET_DATA_SINA_DAILY_BARS_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProviderPriority) != 3 || cfg.ProviderPriority[2] != "sina" {
		t.Fatalf("provider priority=%v", cfg.ProviderPriority)
	}
	sina := cfg.ProviderSettings["sina"]
	if !sina.Enabled || !sina.Quote || sina.DailyBars {
		t.Fatalf("Sina must be enabled for quotes only: %+v", sina)
	}
	if cfg.SinaQuoteURL != "https://hq.sinajs.cn/list=" {
		t.Fatalf("unexpected Sina quote URL %q", cfg.SinaQuoteURL)
	}
}

func TestLoadMiaoXiangSupplementIsOptInAndKeyGuarded(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_MX_ENABLED", "")
	t.Setenv("MARKET_DATA_MX_API_KEY", "")
	t.Setenv("MX_APIKEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("default config should load with MX disabled: %v", err)
	}
	if cfg.MXEnabled || cfg.MXAPIKey != "" {
		t.Fatalf("MX should be disabled by default: enabled=%t keyPresent=%t", cfg.MXEnabled, cfg.MXAPIKey != "")
	}

	t.Setenv("MARKET_DATA_MX_ENABLED", "true")
	if _, err := Load(); err == nil {
		t.Fatal("enabling MX without an API key must fail fast")
	}
	t.Setenv("MARKET_DATA_MX_API_KEY", "mx-test-key-12345678")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("MX with explicit key should load: %v", err)
	}
	if !cfg.MXEnabled || cfg.MXAPIKey != "mx-test-key-12345678" {
		t.Fatalf("MX opt-in config not applied: enabled=%t key=%q", cfg.MXEnabled, cfg.MXAPIKey)
	}
	t.Setenv("MARKET_DATA_MX_API_KEY", "replace-with-a-placeholder-key")
	if _, err := Load(); err == nil {
		t.Fatal("MX placeholder key must be rejected")
	}
}

func TestLoadBuildsMySQLDSNFromSeparateServiceSettings(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "false")
	t.Setenv("MARKET_DATA_MYSQL_DSN", "")
	t.Setenv("MARKET_DATA_DB_HOST", "Mariadb")
	t.Setenv("MARKET_DATA_DB_PORT", "3306")
	t.Setenv("MARKET_DATA_DB_USER", "stock_analysis_server")
	t.Setenv("MARKET_DATA_DB_PASSWORD", "test-db-password")
	t.Setenv("MARKET_DATA_DB_NAME", "stock_analysis_server")
	t.Setenv("MARKET_DATA_DB_CHARSET", "utf8mb4")
	t.Setenv("MARKET_DATA_DB_TIMEZONE", "UTC")
	t.Setenv("MARKET_DATA_REDIS_ADDR", "Redis:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mysqlDriver.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		t.Fatalf("generated DSN is invalid: %v", err)
	}
	if parsed.User != "stock_analysis_server" || parsed.Passwd != "test-db-password" {
		t.Fatalf("unexpected generated DSN user/password: user=%q passwordPresent=%t", parsed.User, parsed.Passwd != "")
	}
	if !parsed.AllowNativePasswords {
		t.Fatal("generated DSN must preserve the MySQL driver's default native-password authentication support")
	}
	if parsed.Addr != "Mariadb:3306" || parsed.DBName != "stock_analysis_server" {
		t.Fatalf("unexpected generated DSN target: address=%q database=%q", parsed.Addr, parsed.DBName)
	}
	if !parsed.ParseTime || parsed.Loc.String() != "UTC" || parsed.Params["charset"] != "utf8mb4" {
		t.Fatalf("unexpected generated DSN options: parseTime=%t location=%q params=%v", parsed.ParseTime, parsed.Loc, parsed.Params)
	}
}

func TestLoadKeepsLegacyMySQLDSNAsTemporaryFallback(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "false")
	t.Setenv("MARKET_DATA_DB_HOST", "")
	t.Setenv("MARKET_DATA_DB_PORT", "")
	t.Setenv("MARKET_DATA_DB_USER", "")
	t.Setenv("MARKET_DATA_DB_PASSWORD", "")
	t.Setenv("MARKET_DATA_DB_NAME", "")
	t.Setenv("MARKET_DATA_DB_CHARSET", "")
	t.Setenv("MARKET_DATA_DB_TIMEZONE", "")
	legacy := "legacy_user:legacy_password@tcp(Mariadb:3306)/legacy_db?charset=utf8mb4&parseTime=true&loc=UTC"
	t.Setenv("MARKET_DATA_MYSQL_DSN", legacy)
	t.Setenv("MARKET_DATA_REDIS_ADDR", "Redis:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MySQLDSN != legacy {
		t.Fatalf("legacy DSN fallback mismatch: got %q want legacy DSN", cfg.MySQLDSN)
	}
}

func TestLoadRejectsInvalidMarketDataDBPort(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_DB_HOST", "Mariadb")
	t.Setenv("MARKET_DATA_DB_PORT", "not-a-port")
	t.Setenv("MARKET_DATA_DB_USER", "stock_analysis_server")
	t.Setenv("MARKET_DATA_DB_NAME", "stock_analysis_server")
	t.Setenv("MARKET_DATA_MYSQL_DSN", "")
	if _, err := Load(); err == nil {
		t.Fatal("invalid MARKET_DATA_DB_PORT must be rejected")
	}
}

func TestLoadUsesSharedMarketDataAdjustmentWhenServiceOverrideIsUnset(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_DEFAULT_ADJUSTMENT", "")
	t.Setenv("MARKET_DATA_ADJUSTMENT", "none")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Adjustment != "none" {
		t.Fatalf("adjustment=%q, want shared MARKET_DATA_ADJUSTMENT value none", cfg.Adjustment)
	}
}

func TestLoadBaoStockEnabledByDefaultAndDailyBarsOnly(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,tencent")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "3s")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "30s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "0")
	t.Setenv("MARKET_DATA_BAOSTOCK_ENABLED", "")
	t.Setenv("MARKET_DATA_BAOSTOCK_DAILY_BARS_ENABLED", "")
	t.Setenv("MARKET_DATA_BAOSTOCK_QUOTE_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["baostock"].Enabled || !cfg.ProviderSettings["baostock"].DailyBars || cfg.ProviderSettings["baostock"].Quote {
		t.Fatalf("BaoStock should be enabled as a daily-bar fallback only by default: %+v", cfg.ProviderSettings["baostock"])
	}
	if cfg.BaostockHost != "public-api.baostock.com" || cfg.BaostockPort != 10030 {
		t.Fatalf("unexpected BaoStock endpoint: %s:%d", cfg.BaostockHost, cfg.BaostockPort)
	}

	t.Setenv("MARKET_DATA_BAOSTOCK_ENABLED", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("explicit BaoStock disable should load: %v", err)
	}
	if cfg.ProviderSettings["baostock"].Enabled {
		t.Fatal("explicit environment disable must override default configuration")
	}

	t.Setenv("MARKET_DATA_BAOSTOCK_ENABLED", "true")
	t.Setenv("MARKET_DATA_BAOSTOCK_DAILY_BARS_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("opt-in BaoStock config should load: %v", err)
	}
	if !cfg.ProviderSettings["baostock"].Enabled || !cfg.ProviderSettings["baostock"].DailyBars || cfg.ProviderSettings["baostock"].Quote {
		t.Fatalf("unexpected BaoStock capabilities: %+v", cfg.ProviderSettings["baostock"])
	}

	t.Setenv("MARKET_DATA_BAOSTOCK_QUOTE_ENABLED", "true")
	if _, err := Load(); err == nil {
		t.Fatal("BaoStock quote capability must fail fast until implemented")
	}
}

func TestLoadRejectsInvalidBaoStockPort(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_BAOSTOCK_PORT", "70000")
	if _, err := Load(); err == nil {
		t.Fatal("invalid BaoStock port must fail configuration loading")
	}
}

func TestLoadYahooFinanceDailyBarsCanBeOptedIn(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,yahoo,tencent,sina")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "4m")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_YAHOO_ENABLED", "true")
	t.Setenv("MARKET_DATA_YAHOO_QUOTE_ENABLED", "")
	t.Setenv("MARKET_DATA_YAHOO_DAILY_BARS_ENABLED", "")
	t.Setenv("MARKET_DATA_YAHOO_CHART_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	yahoo := cfg.ProviderSettings["yahoo"]
	if !yahoo.Enabled || yahoo.Quote || !yahoo.DailyBars {
		t.Fatalf("Yahoo must be enabled for daily bars only: %+v", yahoo)
	}
	if cfg.YahooChartURL != "https://query1.finance.yahoo.com/v8/finance/chart" {
		t.Fatalf("unexpected Yahoo chart endpoint %q", cfg.YahooChartURL)
	}
}

func TestLoadYahooQuoteCanBeOptedInAndDailyBarsCanBeDisabled(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,yahoo,tencent,sina")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "5m")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_YAHOO_ENABLED", "true")
	t.Setenv("MARKET_DATA_YAHOO_QUOTE_ENABLED", "true")
	t.Setenv("MARKET_DATA_YAHOO_DAILY_BARS_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	yahoo := cfg.ProviderSettings["yahoo"]
	if !yahoo.Enabled || !yahoo.Quote || yahoo.DailyBars {
		t.Fatalf("Yahoo quote-only configuration should be accepted: %+v", yahoo)
	}
}

func TestLoadSinaDailyBarsCanBeEnabled(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,yahoo,tencent,sina")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "4m")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_SINA_ENABLED", "true")
	t.Setenv("MARKET_DATA_SINA_QUOTE_ENABLED", "")
	t.Setenv("MARKET_DATA_SINA_DAILY_BARS_ENABLED", "true")
	t.Setenv("MARKET_DATA_SINA_KLINE_URL", "https://example.test/kline")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	sina := cfg.ProviderSettings["sina"]
	if !sina.Enabled || !sina.Quote || !sina.DailyBars {
		t.Fatalf("Sina should expose quote and daily bars when opted in: %+v", sina)
	}
	if cfg.SinaKlineURL != "https://example.test/kline" {
		t.Fatalf("Sina K-line endpoint was not loaded: %q", cfg.SinaKlineURL)
	}
}

func TestLoadNeteaseDailyBarsIsOptInAndEndpointConfigurable(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,yahoo,tencent,sina,netease")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "5m")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_NETEASE_ENABLED", "true")
	t.Setenv("MARKET_DATA_NETEASE_QUOTE_ENABLED", "")
	t.Setenv("MARKET_DATA_NETEASE_DAILY_BARS_ENABLED", "")
	t.Setenv("MARKET_DATA_NETEASE_HISTORY_URL", "https://example.test/history")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	netease := cfg.ProviderSettings["netease"]
	if !netease.Enabled || netease.Quote || !netease.DailyBars {
		t.Fatalf("Netease must expose daily bars only when enabled: %+v", netease)
	}
	if cfg.NeteaseHistoryURL != "https://example.test/history" {
		t.Fatalf("unexpected Netease endpoint %q", cfg.NeteaseHistoryURL)
	}
}

func TestLoadNeteaseQuoteCanBeEnabledAndEndpointConfigured(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_PROVIDER_PRIORITY", "eastmoney,baostock,yahoo,tencent,sina,netease")
	t.Setenv("MARKET_DATA_REQUEST_TIMEOUT", "5m")
	t.Setenv("MARKET_DATA_PROVIDER_TIMEOUT", "12s")
	t.Setenv("MARKET_DATA_PROVIDER_RETRIES", "1")
	t.Setenv("MARKET_DATA_NETEASE_ENABLED", "true")
	t.Setenv("MARKET_DATA_NETEASE_QUOTE_ENABLED", "true")
	t.Setenv("MARKET_DATA_NETEASE_DAILY_BARS_ENABLED", "false")
	t.Setenv("MARKET_DATA_NETEASE_QUOTE_URL", "https://example.test/quote")
	t.Setenv("MARKET_DATA_NETEASE_HISTORY_URL", "https://example.test/history")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	netease := cfg.ProviderSettings["netease"]
	if !netease.Enabled || !netease.Quote || netease.DailyBars {
		t.Fatalf("Netease capabilities not configured as expected: %+v", netease)
	}
	if cfg.NeteaseQuoteURL != "https://example.test/quote" {
		t.Fatalf("unexpected Netease quote endpoint %q", cfg.NeteaseQuoteURL)
	}
}

func TestEastmoneyStockBasicURLAndCapabilityCanBeConfigured(t *testing.T) {
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_STOCK_BASIC_URL", "https://example.test/f10")
	t.Setenv("MARKET_DATA_EASTMONEY_STOCK_BASIC_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EastmoneyStockBasicURL != "https://example.test/f10" {
		t.Fatalf("stock-basic URL=%q", cfg.EastmoneyStockBasicURL)
	}
	if cfg.ProviderSettings["eastmoney"].StockBasic {
		t.Fatalf("stock-basic override must disable capability: %+v", cfg.ProviderSettings["eastmoney"])
	}
}

func TestEastmoneyStockBoardsURLAndCapabilityAreConfigurable(t *testing.T) {
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_STOCK_BOARDS_URL", "https://example.test/slist")
	t.Setenv("MARKET_DATA_EASTMONEY_STOCK_BOARDS_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EastmoneyStockBoardsURL != "https://example.test/slist" {
		t.Fatalf("unexpected stock-boards endpoint %q", cfg.EastmoneyStockBoardsURL)
	}
	if !cfg.ProviderSettings["eastmoney"].StockBoards {
		t.Fatal("stock-boards environment override should enable capability")
	}
}

func TestEastmoneySectorRankingURLAndCapabilityAreConfigurable(t *testing.T) {
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_SECTOR_RANKING_URL", "https://example.test/clist")
	t.Setenv("MARKET_DATA_EASTMONEY_SECTOR_RANKINGS_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EastmoneySectorRankingURL != "https://example.test/clist" {
		t.Fatalf("unexpected sector-ranking endpoint %q", cfg.EastmoneySectorRankingURL)
	}
	if !cfg.ProviderSettings["eastmoney"].SectorRankings {
		t.Fatal("sector-ranking environment override should enable the capability")
	}
}

func TestEastmoneyIndexEndpointAndCapabilityAreConfigurable(t *testing.T) {
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_INDEX_URL", "https://example.test/indices")
	t.Setenv("MARKET_DATA_EASTMONEY_INDEX_QUOTES_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EastmoneyIndexURL != "https://example.test/indices" {
		t.Fatalf("unexpected index endpoint %q", cfg.EastmoneyIndexURL)
	}
	if !cfg.ProviderSettings["eastmoney"].IndexQuotes {
		t.Fatal("index-quote environment override should enable capability")
	}
}

func TestEastmoneyDragonTigerURLAndCapabilityAreConfigurable(t *testing.T) {
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_DRAGON_TIGER_URL", "https://example.test/api/data/v1/get")
	t.Setenv("MARKET_DATA_EASTMONEY_DRAGON_TIGER_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EastmoneyDragonTigerURL != "https://example.test/api/data/v1/get" {
		t.Fatalf("unexpected dragon-tiger endpoint %q", cfg.EastmoneyDragonTigerURL)
	}
	if !cfg.ProviderSettings["eastmoney"].DragonTiger {
		t.Fatal("dragon-tiger override should enable capability")
	}
	t.Setenv("MARKET_DATA_EASTMONEY_DRAGON_TIGER_ENABLED", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderSettings["eastmoney"].DragonTiger {
		t.Fatal("dragon-tiger should be disabled explicitly")
	}
}

func TestEastmoneyAnnouncementsDefaultOffAndEndpointConfigurable(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_ENABLED", "")
	t.Setenv("MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_URL", "https://example.test/api/security/ann")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderSettings["eastmoney"].Announcements {
		t.Fatal("announcements must remain disabled until target deployment acceptance")
	}
	if cfg.EastmoneyAnnouncementsURL != "https://example.test/api/security/ann" {
		t.Fatalf("unexpected announcement endpoint %q", cfg.EastmoneyAnnouncementsURL)
	}

	t.Setenv("MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["eastmoney"].Announcements {
		t.Fatal("explicit opt-in should enable the announcements capability")
	}
	t.Setenv("MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_ENABLED", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderSettings["eastmoney"].Announcements {
		t.Fatal("explicit false must disable announcements")
	}
}

func TestBaoStockFundamentalsDefaultsOffAndCanBeEnabledExplicitly(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_BAOSTOCK_FUNDAMENTALS_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderSettings["baostock"].Fundamentals {
		t.Fatal("fundamentals must stay disabled until target deployment acceptance")
	}
	t.Setenv("MARKET_DATA_BAOSTOCK_FUNDAMENTALS_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["baostock"].Fundamentals {
		t.Fatal("explicit opt-in must enable fundamentals")
	}
}
