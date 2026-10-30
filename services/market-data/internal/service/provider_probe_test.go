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

func TestRecordProviderProbeCoverageAcceptsOnlyValidRatios(t *testing.T) {
	evidence := &providerProbeEvidence{}
	ctx := context.WithValue(context.Background(), providerProbeEvidenceContextKey{}, evidence)
	valid := 0.75
	recordProviderProbeCoverage(ctx, valid)
	if evidence.coverageRatio == nil || *evidence.coverageRatio != valid {
		t.Fatalf("expected coverage ratio %v, got %+v", valid, evidence.coverageRatio)
	}
	recordProviderProbeCoverage(ctx, 1.01)
	if evidence.coverageRatio == nil || *evidence.coverageRatio != valid {
		t.Fatalf("invalid coverage ratio must not overwrite valid evidence, got %+v", evidence.coverageRatio)
	}
}

func TestRecordProviderProbeEvidenceOnlyClassifiesAshareMarkets(t *testing.T) {
	for _, test := range []struct {
		symbol string
		want   string
	}{
		{symbol: "600519.SH", want: "SH"},
		{symbol: "000001.SZ", want: "SZ"},
		{symbol: "920001.BJ", want: "BJ"},
		{symbol: "885001.TI", want: ""},
		{symbol: "000001.IND", want: ""},
	} {
		evidence := &providerProbeEvidence{}
		ctx := context.WithValue(context.Background(), providerProbeEvidenceContextKey{}, evidence)
		recordProviderProbeEvidence(ctx, test.symbol, "", 1, 1, nil)
		if evidence.sampleMarket != test.want {
			t.Errorf("symbol %q market = %q, want %q", test.symbol, evidence.sampleMarket, test.want)
		}
	}
}

func TestProviderProbeOptionsApplyMarketAndSymbolSample(t *testing.T) {
	p := providerProbeQuoteFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"quote"}, Markets: []string{"CHINEXT"}, Symbols: []string{"300750.SZ"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("filtered probe returned error: %v", err)
	}
	if len(run.Items) != 1 || run.Items[0].SampleSymbol != "300750.SZ" || run.Items[0].SampleMarket != "SZ" {
		t.Fatalf("unexpected selected sample evidence: %+v", run)
	}
	if len(run.RequestedMarkets) != 1 || run.RequestedMarkets[0] != "CHINEXT" || len(run.RequestedSymbols) != 1 || run.RequestedSymbols[0] != "300750.SZ" {
		t.Fatalf("unexpected requested filters: %+v", run)
	}
}

func TestProviderProbeOptionsValidateMarketAndSymbolFilters(t *testing.T) {
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	cases := []ProviderProbeOptions{
		{Scope: "all", Capabilities: []string{"quote"}, Markets: []string{"UNKNOWN"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"quote"}, Markets: []string{"STAR"}, Symbols: []string{"300750.SZ"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"quote"}, Symbols: []string{"600519"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"quote"}, Markets: []string{"SH_MAIN", "STAR"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"market_breadth"}, Markets: []string{"SH_MAIN"}, TimeoutSeconds: 5},
	}
	for i, options := range cases {
		if _, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), options); err == nil {
			t.Errorf("case %d should reject invalid filters: %+v", i, options)
		}
	}
}

type providerProbeUnsupportedBeijingFake struct{}

func (providerProbeUnsupportedBeijingFake) Name() string { return "probe-sina" }
func (providerProbeUnsupportedBeijingFake) FetchQuote(context.Context, string) (*model.Quote, error) {
	return nil, errors.New(`Sina does not support exchange suffix "BJ"`)
}

func TestProviderProbeMarksUnsupportedBeijingProviderWithoutFailure(t *testing.T) {
	p := providerProbeUnsupportedBeijingFake{}
	ds := NewWithCapabilities(
		[]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil,
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
	)
	filters := []struct {
		name    string
		markets []string
		symbols []string
	}{
		{name: "market filter", markets: []string{"BSE"}},
		{name: "symbol filter", symbols: []string{"920001.BJ"}},
	}
	for _, filter := range filters {
		t.Run(filter.name, func(t *testing.T) {
			run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
				Scope: "all", Capabilities: []string{"quote"}, Markets: filter.markets,
				Symbols: filter.symbols, TimeoutSeconds: 5,
			})
			if err != nil {
				t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
			}
			item, found := findProbeItem(run.Items, p.Name(), "quote")
			if !found || item.Status != "not_supported" || item.ErrorCode != "market_not_supported" || item.ErrorStage != "configuration" {
				t.Fatalf("unsupported BSE capability must not be counted as provider failure: %+v", run)
			}
			if run.NotSupported != 1 || run.Failed != 0 || run.Status != "degraded" {
				t.Fatalf("unsupported provider summary is incorrect: %+v", run)
			}
		})
	}
}

func TestRecordProviderProbeCoverageCountsPreservesDenominator(t *testing.T) {
	evidence := &providerProbeEvidence{}
	ctx := context.WithValue(context.Background(), providerProbeEvidenceContextKey{}, evidence)
	recordProviderProbeCoverageCounts(ctx, 100, 90, 88, 10, 2)
	if evidence.coverageTotal == nil || *evidence.coverageTotal != 100 ||
		evidence.coverageReturned == nil || *evidence.coverageReturned != 90 ||
		evidence.coverageValidated == nil || *evidence.coverageValidated != 88 ||
		evidence.coverageMissing == nil || *evidence.coverageMissing != 10 ||
		evidence.coverageExcluded == nil || *evidence.coverageExcluded != 2 ||
		evidence.coverageRatio == nil || *evidence.coverageRatio != 0.88 {
		t.Fatalf("coverage denominator evidence is incomplete: %+v", evidence)
	}

	recordProviderProbeCoverageCounts(ctx, 100, 90, 89, 10, 2)
	if *evidence.coverageValidated != 88 || *evidence.coverageRatio != 0.88 {
		t.Fatalf("inconsistent coverage counts must not overwrite valid evidence: %+v", evidence)
	}
}
