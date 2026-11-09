package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/config"
)

func testConfig() config.Config {
	return config.Config{
		APIKey:                  "test-market-data-api-key-at-least-32",
		HTTPAddr:                "127.0.0.1:0",
		RequestTimeout:          10 * time.Second,
		ProviderTimeout:         5 * time.Second,
		CircuitFailureThreshold: 3,
		CircuitCooldown:         30 * time.Second,
		QuoteCacheTTL:           30 * time.Second,
		BarsCacheTTL:            5 * time.Minute,
		DefaultDailyBars:        120,
		MaxDailyBars:            2000,
		Adjustment:              "forward_adjusted",
		ProviderPriority:        []string{"eastmoney"},
		ProviderSettings: map[string]config.ProviderSettings{
			"eastmoney": {Enabled: true, Quote: true, DailyBars: true},
		},
	}
}

func testDependencies() Dependencies {
	return Dependencies{
		Cache:  cache.NewDisabled(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestNewExposesApplicationHTTPAndDiagnostics(t *testing.T) {
	module, err := New(testConfig(), testDependencies())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if module.InProcessHandler() == nil {
		t.Fatal("InProcessHandler() returned nil")
	}
	if module.Handler() == nil {
		t.Fatal("Handler() returned nil")
	}
	if module.Application() == nil {
		t.Fatal("Application() returned nil")
	}
	if module.Diagnostics() == nil {
		t.Fatal("Diagnostics() returned nil")
	}
	if err := module.Diagnostics().Ready(context.Background()); err != nil {
		t.Fatalf("Diagnostics.Ready() error = %v", err)
	}
	if got := module.Diagnostics().ProviderDiagnostics(); len(got) == 0 {
		t.Fatal("ProviderDiagnostics() returned no configured provider")
	}
	summary := module.ProviderSummary()
	if len(summary.All) != 1 || summary.All[0] != "eastmoney" ||
		len(summary.Quotes) != 1 || len(summary.DailyBars) != 1 {
		t.Fatalf("ProviderSummary() = %+v", summary)
	}
	summary.All[0] = "mutated"
	if module.ProviderSummary().All[0] != "eastmoney" {
		t.Fatal("ProviderSummary() must return a defensive copy")
	}
	if err := module.Close(context.Background()); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := module.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestModuleMigrationRunnerIsNilWhenStorageDisabled(t *testing.T) {
	module, err := New(testConfig(), testDependencies())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer module.Close(context.Background())

	if runner := module.MigrationRunner(); runner != nil {
		t.Fatalf("MigrationRunner() = %T, want nil when storage is disabled", runner)
	}
}

func TestNewRejectsMissingConfiguredInfrastructure(t *testing.T) {
	cfg := testConfig()
	cfg.MySQLDSN = "market-data-test-dsn"
	if _, err := New(cfg, testDependencies()); err == nil {
		t.Fatal("New() must reject configured MariaDB without an injected Store")
	}

	cfg = testConfig()
	cfg.RedisAddr = "127.0.0.1:6379"
	if _, err := New(cfg, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}); err == nil {
		t.Fatal("New() must reject configured Redis without an injected Cache")
	}
}

func TestNewRejectsUnknownProvider(t *testing.T) {
	cfg := testConfig()
	cfg.ProviderPriority = []string{"unknown-provider"}
	if _, err := New(cfg, testDependencies()); err == nil {
		t.Fatal("New() must reject an unknown configured provider")
	}
}
