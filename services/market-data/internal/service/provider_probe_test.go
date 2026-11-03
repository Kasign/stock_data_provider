package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/validation"
)

type providerProbeQuoteFake struct{}

func (providerProbeQuoteFake) Name() string { return "probe-test" }

type providerProbePartialMarketBreadthFake struct {
	snapshot *model.MarketBreadthSnapshot
}

func (p providerProbePartialMarketBreadthFake) Name() string { return "probe-partial-market-breadth" }

func (p providerProbePartialMarketBreadthFake) FetchMarketBreadth(context.Context) (*model.MarketBreadthSnapshot, error) {
	return p.snapshot, nil
}

func TestProviderProbePreservesPartialMarketBreadthCoverageWithoutBusinessAcceptance(t *testing.T) {
	now := time.Now().UTC()
	reportDate := now.In(marketLocation()).Format("2006-01-02")
	snapshot := &model.MarketBreadthSnapshot{
		Market: "cn_a_share", TotalSecurities: 3, CoveredSecurities: 2,
		AdvancingCount: 1, UnclassifiedCount: 1,
		AmountCoverageCount: 2, QuoteTimestampCoverageCount: 2,
		OldestQuoteTime: &now, NewestQuoteTime: &now,
		Source: "probe-test", SourceVersion: "probe-market-breadth-v1",
		ProviderReportDate: reportDate, FetchedAt: now,
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_quote_coverage_incomplete"},
	}
	if err := validateMarketBreadthProbeSnapshot(snapshot); err != nil {
		t.Fatalf("diagnostic validator should accept structurally valid partial evidence: %v", err)
	}
	if err := validateMarketBreadth(snapshot); err == nil {
		t.Fatal("business validator must reject partial population coverage")
	}

	p := providerProbePartialMarketBreadthFake{snapshot: snapshot}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	ds.SetMarketBreadthProviders([]provider.MarketBreadthProvider{p})
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"market_breadth"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "market_breadth")
	if !found || item.Status != "returned_degraded" {
		t.Fatalf("partial market-breadth probe should be degraded, got %+v", run.Items)
	}
	if item.CoverageTotal == nil || *item.CoverageTotal != 3 ||
		item.CoverageReturned == nil || *item.CoverageReturned != 2 ||
		item.CoverageValidated == nil || *item.CoverageValidated != 1 ||
		item.CoverageMissing == nil || *item.CoverageMissing != 1 ||
		item.CoverageExcluded == nil || *item.CoverageExcluded != 1 ||
		item.CoverageRatio == nil || *item.CoverageRatio < 0.333333 || *item.CoverageRatio > 0.333334 {
		t.Fatalf("partial market-breadth coverage evidence is incomplete: %+v", item)
	}
}

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
	recordProviderProbeCoverageCounts(ctx, 100, 90, 88, 10, 2, "provider_universe", "fixture.total")
	if evidence.coverageTotal == nil || *evidence.coverageTotal != 100 ||
		evidence.coveragePopulation != "provider_universe" || evidence.coverageSource != "fixture.total" ||
		evidence.coverageReturned == nil || *evidence.coverageReturned != 90 ||
		evidence.coverageValidated == nil || *evidence.coverageValidated != 88 ||
		evidence.coverageMissing == nil || *evidence.coverageMissing != 10 ||
		evidence.coverageExcluded == nil || *evidence.coverageExcluded != 2 ||
		evidence.coverageRatio == nil || *evidence.coverageRatio != 0.88 {
		t.Fatalf("coverage denominator evidence is incomplete: %+v", evidence)
	}

	recordProviderProbeCoverageCounts(ctx, 100, 90, 89, 10, 2, "provider_universe", "fixture.total")
	if *evidence.coverageValidated != 88 || *evidence.coverageRatio != 0.88 {
		t.Fatalf("inconsistent coverage counts must not overwrite valid evidence: %+v", evidence)
	}
}

func TestProviderProbeChecksFilterAndPreserveUnimplementedEvidence(t *testing.T) {
	p := providerProbeQuoteFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"quote"}, Checks: []string{"quality", "connectivity", "freshness", "cross_provider"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	if strings.Join(run.RequestedChecks, ",") != "connectivity,cross_provider,freshness,quality" {
		t.Fatalf("requested checks were not normalized: %+v", run.RequestedChecks)
	}
	statuses := map[string]string{}
	for _, result := range run.CheckResults {
		statuses[result.Check] = result.Status
		if result.Check == "cross_provider" && result.Capability != "" {
			t.Fatalf("cross_provider must not report an unrequested daily_bars capability: %+v", result)
		}
	}
	if statuses["connectivity"] != "passed" || statuses["quality"] != "passed" {
		t.Fatalf("connectivity/quality check result is incorrect: %+v", run.CheckResults)
	}
	if statuses["freshness"] != "skipped" || statuses["cross_provider"] != "skipped" {
		t.Fatalf("checks without configured SLA/snapshot must remain explicitly skipped: %+v", run.CheckResults)
	}
	if len(run.CheckResults) != 4 {
		t.Fatalf("checks filter should return only four requested dimensions, got %+v", run.CheckResults)
	}
}

func TestProviderProbeRejectsUnknownOrDuplicateChecks(t *testing.T) {
	for _, checks := range [][]string{{"unknown"}, {"quality", "quality"}} {
		_, err := (&DataService{}).ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
			Scope: "all", Capabilities: []string{"quote"}, Checks: checks, TimeoutSeconds: 5,
		})
		if err == nil {
			t.Fatalf("invalid checks %v should be rejected", checks)
		}
	}
}

func TestBuildProviderProbeCheckResultsClassifiesNetworkAndContractFailures(t *testing.T) {
	now := time.Now().UTC()
	items := []model.ProviderProbeItem{
		{Provider: "network", Capability: "quote", Status: "failed", ErrorCode: "network_error", ErrorStage: "connect", CheckedAt: now},
		{Provider: "contract", Capability: "quote", Status: "failed", ErrorCode: "contract_invalid", ErrorStage: "contract", CheckedAt: now},
	}
	results := buildProviderProbeCheckResults(items, []string{"connectivity", "contract"})
	if len(results) != 4 {
		t.Fatalf("unexpected check result count: %+v", results)
	}
	got := map[string]string{}
	for _, result := range results {
		got[result.Provider+"/"+result.Check] = result.Status
	}
	if got["network/connectivity"] != "failed" || got["network/contract"] != "skipped" ||
		got["contract/connectivity"] != "passed" || got["contract/contract"] != "failed" {
		t.Fatalf("unexpected check classification: %+v", results)
	}
}

type providerProbeDailyBarsFake struct {
	name    string
	close   float64
	quality model.QualityStatus
}

func (p providerProbeDailyBarsFake) Name() string { return p.name }
func (p providerProbeDailyBarsFake) FetchDailyBars(_ context.Context, symbol, _, _ string, _ int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	volume := int64(10000)
	amount := 100000.0
	now := time.Now().UTC()
	return []model.DailyBar{{
		Symbol: symbol, TradeDate: "2026-10-29", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: p.close, Volume: &volume, Amount: &amount,
		AdjustmentBasis: adjustment, Source: p.name, SourceVersion: p.name + "-v1",
		ProviderTime: &now, FetchedAt: now, NormalizedAt: now,
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: p.quality,
	}}, nil
}

type providerProbeDailyBarsReportedFake struct {
	providerProbeDailyBarsFake
}

func (p providerProbeDailyBarsReportedFake) FetchDailyBars(ctx context.Context, symbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	bars, err := p.providerProbeDailyBarsFake.FetchDailyBars(ctx, symbol, startDate, endDate, limit, adjustment)
	if err != nil {
		return nil, err
	}
	if len(bars) > 0 {
		bars[0].Amount = nil
		bars[0].AmountUnit = "unknown"
		bars[0].QualityStatus = model.QualityDegraded
		bars[0].QualityReasons = []string{"amount_missing"}
	}
	report := validation.NewReport(10)
	report.Add(validation.FieldIssue{
		Stage: validation.StageValidate, Field: "amount", ProviderField: "amount",
		Code: "missing_optional", Expected: "optional CNY amount",
		Severity: validation.SeverityWarning, Message: "amount is unavailable",
	})
	validation.AddContextReport(ctx, report)
	return bars, nil
}

func TestProviderProbeDoesNotDuplicateAdapterDailyBarIssues(t *testing.T) {
	p := providerProbeDailyBarsReportedFake{providerProbeDailyBarsFake{name: "probe-reported-bars", close: 10.5, quality: model.QualityValid}}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Checks: []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
	if !found || item.ValidationTotalCount != 1 || len(item.ValidationIssues) != 1 {
		t.Fatalf("adapter field issue was duplicated by Probe validation: %+v", run.Items)
	}
}

func TestProviderProbeRunsCrossProviderDailyBarComparison(t *testing.T) {
	left := providerProbeDailyBarsFake{name: "probe-left", close: 10.5, quality: model.QualityValid}
	right := providerProbeDailyBarsFake{name: "probe-right", close: 10.5, quality: model.QualityValid}
	ds := NewWithCapabilities(
		[]provider.NamedProvider{left, right}, nil, []provider.DailyBarProvider{left, right},
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
	)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Checks: []string{"cross_provider"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	if len(run.CheckResults) != 1 {
		t.Fatalf("expected one cross-provider result, got %+v", run.CheckResults)
	}
	result := run.CheckResults[0]
	if result.Status != "passed" || result.Provider != "probe-left+probe-right" || result.FieldCount != 6 || len(result.ComparedDates) != 1 {
		t.Fatalf("unexpected cross-provider comparison result: %+v", result)
	}
}

func TestBuildCrossProviderProbeCheckResultsRejectsDegradedSnapshots(t *testing.T) {
	left := &provider.CrossProviderDailySnapshot{
		Provider: "left", SourceVersion: "left-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "degraded",
		Bars: []provider.CrossProviderDailyBar{{TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, VolumeUnit: "shares"}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, VolumeUnit: "shares"}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, time.Now().UTC())
	if len(results) != 1 || results[0].Status != "skipped" || results[0].ErrorCode != "comparison_not_comparable" {
		t.Fatalf("degraded snapshots must not be reported as cross-provider pass: %+v", results)
	}
}

func TestProviderProbeFieldIssueJSONContainsSafeDiagnosticFields(t *testing.T) {
	rowIndex := 3
	item := model.ProviderProbeItem{
		Provider: "test", Capability: "daily_bars", Status: "degraded",
		ValidationTotalCount: 2, ValidationErrorCount: 1, ValidationWarningCount: 1,
		ValidationIssues: []model.ProviderProbeFieldIssue{{
			Stage: "validate", Field: "open", ProviderField: "f2", Code: "out_of_range",
			Expected: "positive finite number", ActualType: "number", ActualPreview: "-1",
			Severity: "error", Message: "daily bar open must be positive", RowIndex: &rowIndex,
			Symbol: "600519.SH", TradeDate: "2026-10-30",
		}},
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal probe item: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal probe item: %v", err)
	}
	if decoded["validation_total_count"] != float64(2) || decoded["validation_error_count"] != float64(1) {
		t.Fatalf("validation counters missing from probe response: %s", data)
	}
	issues, ok := decoded["validation_issues"].([]any)
	if !ok || len(issues) != 1 {
		t.Fatalf("validation issue details missing from probe response: %s", data)
	}
}

// TestBuildCrossProviderProbeCheckResultsIncludesFieldLevelEvidence 确保对账结果可追溯到具体字段。
func TestBuildCrossProviderProbeCheckResultsIncludesFieldLevelEvidence(t *testing.T) {
	left := &provider.CrossProviderDailySnapshot{
		Provider: "left", SourceVersion: "left-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: 100, VolumeUnit: "shares",
		}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: 100, VolumeUnit: "shares",
		}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected one comparison result, got %+v", results)
	}
	result := results[0]
	if result.Status != "degraded" || result.FieldCount != 6 || result.MatchedFieldCount != 5 ||
		result.NotComparableCount != 1 || result.MismatchCount != 0 || len(result.FieldComparisons) != 6 {
		t.Fatalf("unexpected field-level comparison summary: %+v", result)
	}
	var amount *model.ProviderProbeFieldComparison
	for i := range result.FieldComparisons {
		if strings.HasSuffix(result.FieldComparisons[i].Field, ".amount") {
			amount = &result.FieldComparisons[i]
			break
		}
	}
	if amount == nil || amount.Status != "not_comparable" || amount.Reason != "amount_missing_or_unit_not_cny" {
		t.Fatalf("missing amount evidence should remain explicitly not comparable: %+v", result.FieldComparisons)
	}
	if amount.Left != nil || amount.Right != nil || amount.Difference != nil {
		t.Fatalf("non-comparable amount must not be serialized as zero values: %+v", amount)
	}
}

func TestBuildProviderProbeCheckResultsMarksUnknownQualityAsDegraded(t *testing.T) {
	results := buildProviderProbeCheckResults([]model.ProviderProbeItem{{
		Provider: "unknown-quality", Capability: "quote", Status: "returned_unknown",
		QualityStatus: model.QualityUnknown, CheckedAt: time.Now().UTC(),
	}}, []string{"quality"})
	if len(results) != 1 || results[0].Status != "degraded" || results[0].ErrorCode != "quality_unknown" {
		t.Fatalf("unknown quality must not be confused with a skipped check: %+v", results)
	}
}

func TestBuildCrossProviderProbeCheckResultsExposesMismatchValuesAndTolerance(t *testing.T) {
	left := &provider.CrossProviderDailySnapshot{
		Provider: "left", SourceVersion: "left-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: 100, VolumeUnit: "shares",
		}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10.7,
			Volume: 100, VolumeUnit: "shares",
		}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected one comparison result, got %+v", results)
	}
	result := results[0]
	if result.Status != "failed" || result.MismatchCount != 1 || result.MatchedFieldCount != 4 ||
		result.NotComparableCount != 1 || len(result.FieldComparisons) != 6 {
		t.Fatalf("unexpected mismatch summary: %+v", result)
	}
	var closeField *model.ProviderProbeFieldComparison
	for i := range result.FieldComparisons {
		if strings.HasSuffix(result.FieldComparisons[i].Field, ".close") {
			closeField = &result.FieldComparisons[i]
			break
		}
	}
	if closeField == nil || closeField.Status != "mismatch" || closeField.Left == nil ||
		closeField.Right == nil || closeField.Difference == nil || closeField.Tolerance == nil {
		t.Fatalf("close mismatch must expose both values, difference and tolerance: %+v", result.FieldComparisons)
	}
	if *closeField.Left != 10.5 || *closeField.Right != 10.7 || *closeField.Difference < 0.199 ||
		*closeField.Difference > 0.201 || *closeField.Tolerance != 0.001 {
		t.Fatalf("unexpected close mismatch evidence: %+v", closeField)
	}
}


type providerProbeFreshnessCalendarFake struct{}

func (providerProbeFreshnessCalendarFake) Name() string { return "probe-freshness-calendar" }
func (providerProbeFreshnessCalendarFake) FetchTradingCalendar(_ context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	fetchedAt := time.Now().UTC()
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: startDate, EndDate: endDate,
		TotalDays: 1, TradingDays: 1,
		Items:  []model.TradingCalendarDay{{Date: startDate, IsTradingDay: true}},
		Source: "probe-freshness-calendar", SourceVersion: "calendar-v1",
		FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}, nil
}

func TestApplyFreshnessProbeResultsUsesPolicyAndMarketSession(t *testing.T) {
	location := marketLocation()
	checkedAt := time.Date(2026, 11, 3, 10, 0, 0, 0, location).UTC()
	sourceTime := checkedAt.Add(-30 * time.Second)
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{providerProbeFreshnessCalendarFake{}})
	run := &model.ProviderProbeRun{
		FinishedAt: checkedAt,
		Items: []model.ProviderProbeItem{{
			Provider: "breadth-provider", Capability: "market_breadth",
			ProviderTime: &sourceTime, ProviderTimeType: "oldest_component_quote_time",
		}},
		CheckResults: []model.ProviderProbeCheckResult{{
			Check: "freshness", Provider: "breadth-provider", Capability: "market_breadth",
			ProviderTime: &sourceTime, CheckedAt: checkedAt,
		}},
	}
	ds.applyFreshnessProbeResults(context.Background(), run)
	result := run.CheckResults[0]
	if result.Status != "passed" || result.FreshnessStatus != "fresh" ||
		result.FreshnessReason != "within_sla" || result.FreshnessDomain != "market_breadth" ||
		result.MarketPhase != model.MarketSessionContinuousAM || result.AgeSeconds == nil || *result.AgeSeconds != 30 {
		t.Fatalf("freshness check did not use the domain SLA and market session: %+v", result)
	}
}

func TestApplyFreshnessProbeResultsKeepsMissingMarketEvidenceUnknown(t *testing.T) {
	checkedAt := time.Date(2026, 11, 3, 10, 0, 0, 0, marketLocation()).UTC()
	sourceTime := checkedAt.Add(-30 * time.Second)
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run := &model.ProviderProbeRun{
		FinishedAt: checkedAt,
		Items: []model.ProviderProbeItem{{
			Provider: "index-provider", Capability: "index_quotes",
			ProviderTime: &sourceTime, ProviderTimeType: "provider_snapshot_time",
		}},
		CheckResults: []model.ProviderProbeCheckResult{{
			Check: "freshness", Provider: "index-provider", Capability: "index_quotes",
			ProviderTime: &sourceTime, CheckedAt: checkedAt,
		}},
	}
	ds.applyFreshnessProbeResults(context.Background(), run)
	result := run.CheckResults[0]
	if result.Status != "skipped" || result.FreshnessStatus != "unknown" ||
		result.FreshnessReason != "trading_calendar_evidence_missing" {
		t.Fatalf("missing calendar evidence must not be reported fresh: %+v", result)
	}
}
