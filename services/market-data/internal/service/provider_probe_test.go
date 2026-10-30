package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type providerProbeQuoteFake struct{}

func (providerProbeQuoteFake) Name() string { return "probe-test" }
func (providerProbeQuoteFake) FetchQuote(_ context.Context, symbol string) (*model.Quote, error) {
	now := time.Now().UTC()
	price, change, pct, open, high, low, prev, amount := 10.0, 1.0, 11.111111, 9.0, 11.0, 8.0, 9.0, 100000.0
	volume := int64(10000)
	return &model.Quote{Symbol: symbol, Name: "测试", CurrentPrice: &price, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &volume, Amount: &amount, QuoteTime: &now, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: now, NormalizedAt: now, Source: "probe-test", SourceVersion: "probe-quote-v1", VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}, nil
}

func TestProviderActiveProbeRunsOutsideBusinessCircuitBreaker(t *testing.T) {
	p := providerProbeQuoteFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilities(context.Background())
	if err != nil {
		t.Fatalf("ProbeProviderCapabilities() error = %v", err)
	}
	if run.Returned != 1 || run.Failed != 0 || run.NotConfigured == 0 || run.Persisted {
		t.Fatalf("unexpected probe summary: %+v", run)
	}
	item, found := findProbeItem(run.Items, "probe-test", "quote")
	if !found || item.Status != "returned" {
		t.Fatalf("expected a successful quote probe, got %+v", run.Items)
	}
	if item.SampleSymbol != "600519.SH" || item.SampleMarket != "SH" || item.SampleDate == "" || item.RowsReturned != 1 || item.RowsValidated != 1 || item.ProviderTime == nil {
		t.Fatalf("quote probe is missing sample and freshness evidence: %+v", item)
	}
}

type providerProbeFailQuoteFake struct {
	message string
}

func (p providerProbeFailQuoteFake) Name() string { return "probe-fail-test" }
func (p providerProbeFailQuoteFake) FetchQuote(context.Context, string) (*model.Quote, error) {
	return nil, errors.New(p.message)
}

func TestProviderProbeFailureIncludesSanitizedHTTPDetails(t *testing.T) {
	p := providerProbeFailQuoteFake{message: "upstream HTTP 502 Bad Gateway https://example.test/quote?api_key=secret-value"}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilities(context.Background())
	if err != nil {
		t.Fatalf("ProbeProviderCapabilities() error = %v", err)
	}
	if run.Failed != 1 || run.NotConfigured == 0 {
		t.Fatalf("expected one failed probe and explicit unconfigured capabilities, got %+v", run)
	}
	item, found := findProbeItem(run.Items, "probe-fail-test", "quote")
	if !found {
		t.Fatalf("failed quote probe item not found: %+v", run.Items)
	}
	if item.ErrorCode != "http_502" || item.ErrorStage != "http" || item.HTTPStatus != 502 {
		t.Fatalf("HTTP failure details were not classified: %+v", item)
	}
	if item.ErrorMessage == "" || !strings.Contains(item.ErrorMessage, "HTTP 502") {
		t.Fatalf("sanitized failure message should retain actionable HTTP detail: %+v", item)
	}
	if strings.Contains(item.ErrorMessage, "secret-value") || strings.Contains(item.ErrorMessage, "api_key=") {
		t.Fatalf("probe error message leaked a credential: %q", item.ErrorMessage)
	}
}

func findProbeItem(items []model.ProviderProbeItem, providerName, capability string) (model.ProviderProbeItem, bool) {
	for _, item := range items {
		if item.Provider == providerName && item.Capability == capability {
			return item, true
		}
	}
	return model.ProviderProbeItem{}, false
}

type providerProbeMissingIdentityFake struct{}

func (providerProbeMissingIdentityFake) Name() string { return "probe-missing-identity" }
func (providerProbeMissingIdentityFake) FetchQuote(ctx context.Context, symbol string) (*model.Quote, error) {
	quote, err := (providerProbeQuoteFake{}).FetchQuote(ctx, symbol)
	if err != nil {
		return nil, err
	}
	quote.Symbol = ""
	quote.Source = ""
	return quote, nil
}

func TestProviderProbeRejectsQuoteWithoutIdentity(t *testing.T) {
	p := providerProbeMissingIdentityFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilities(context.Background())
	if err != nil {
		t.Fatalf("ProbeProviderCapabilities() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "quote")
	if !found || item.Status != "failed" || item.ErrorCode != "contract_invalid" {
		t.Fatalf("expected missing quote identity to fail contract validation, got %+v", run)
	}
}

func TestProviderProbeFailureClassification(t *testing.T) {
	cases := []struct {
		message string
		code    string
		stage   string
	}{
		{"HTTP 502 Bad Gateway", "http_502", "http"},
		{"HTTP 429 Too Many Requests", "rate_limited", "rate_limit"},
		{"HTTP 403 Forbidden", "permission_denied", "provider_permission"},
		{"dial tcp: lookup example.test: no such host", "network_error", "connect"},
		{"quote contract missing source version", "contract_invalid", "contract"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			code := classifyProbeError(errors.New(tc.message))
			if code != tc.code || probeErrorStage(code) != tc.stage {
				t.Fatalf("classification = %q/%q, want %q/%q", code, probeErrorStage(code), tc.code, tc.stage)
			}
		})
	}
}

func TestProviderProbeReportsUnconfiguredCoreCapabilities(t *testing.T) {
	p := providerProbeQuoteFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilities(context.Background())
	if err != nil {
		t.Fatalf("ProbeProviderCapabilities() error = %v", err)
	}
	if run.NotConfigured == 0 {
		t.Fatalf("expected missing core capabilities to be reported, got %+v", run)
	}
	found := false
	for _, item := range run.Items {
		if item.Capability == "trading_calendar" {
			found = true
			if item.Status != "not_configured" || item.ErrorStage != "configuration" {
				t.Fatalf("unexpected result: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("trading_calendar missing from diagnostic coverage report")
	}
}

func TestProviderProbeOptionsLimitSelectedCapabilities(t *testing.T) {
	p := providerProbeQuoteFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"quote"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	if run.Total != 1 || run.Scope != "all" || run.TimeoutSeconds != 5 || len(run.RequestedCapabilities) != 1 || run.RequestedCapabilities[0] != "quote" {
		t.Fatalf("unexpected filtered probe metadata: %+v", run)
	}
	if run.Items[0].Capability != "quote" || run.Items[0].Status != "returned" {
		t.Fatalf("unexpected filtered probe items: %+v", run.Items)
	}
}

func TestProviderProbeOptionsValidateScopeCapabilityAndTimeout(t *testing.T) {
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	cases := []ProviderProbeOptions{
		{Scope: "unknown", TimeoutSeconds: 5},
		{Scope: "core", Capabilities: []string{"stock_universe"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"unknown"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"quote", "quote"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"quote"}, TimeoutSeconds: 31},
	}
	for i, options := range cases {
		if _, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), options); err == nil {
			t.Errorf("case %d should reject invalid options: %+v", i, options)
		}
	}
}
