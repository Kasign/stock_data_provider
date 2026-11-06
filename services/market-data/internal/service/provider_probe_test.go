package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/cache"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/freshness"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/normalize"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/provider"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/validation"
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
	if len(item.CoverageDimensions) != 4 {
		t.Fatalf("expected four field-specific coverage dimensions, got %+v", item.CoverageDimensions)
	}
	dimensions := make(map[string]model.ProviderProbeCoverageDimension, len(item.CoverageDimensions))
	for _, dimension := range item.CoverageDimensions {
		dimensions[dimension.Dimension] = dimension
		if dimension.Total != 3 || dimension.Returned != 2 || dimension.Missing != 1 ||
			dimension.Population != "provider_universe" || dimension.Source == "" {
			t.Fatalf("invalid field-specific denominator evidence: %+v", dimension)
		}
	}
	if dimensions["quote_response"].Validated != 2 || dimensions["quote_response"].Excluded != 0 ||
		dimensions["price_classification"].Validated != 1 || dimensions["price_classification"].Excluded != 1 ||
		dimensions["quote_timestamp"].Validated != 2 || dimensions["quote_timestamp"].Excluded != 0 ||
		dimensions["amount"].Validated != 2 || dimensions["amount"].Excluded != 0 {
		t.Fatalf("field-specific coverage counters do not match snapshot evidence: %+v", dimensions)
	}
}

func TestBuildMarketBreadthCoverageDimensionsPreservesZeroAndRejectsInvalidEvidence(t *testing.T) {
	zero := buildMarketBreadthCoverageDimensions(&model.MarketBreadthSnapshot{
		TotalSecurities: 3, CoveredSecurities: 0,
		UnclassifiedCount: 0, QuoteTimestampCoverageCount: 0, AmountCoverageCount: 0,
	})
	if len(zero) != 4 {
		t.Fatalf("zero-return snapshot must retain its proven denominator: %+v", zero)
	}
	for _, dimension := range zero {
		if dimension.Total != 3 || dimension.Returned != 0 || dimension.Validated != 0 ||
			dimension.Missing != 3 || dimension.Excluded != 0 || dimension.Ratio != 0 {
			t.Fatalf("unexpected zero-return dimension: %+v", dimension)
		}
	}
	invalid := buildMarketBreadthCoverageDimensions(&model.MarketBreadthSnapshot{
		TotalSecurities: 3, CoveredSecurities: 2,
		UnclassifiedCount: 0, QuoteTimestampCoverageCount: 4, AmountCoverageCount: 2,
	})
	if len(invalid) != 0 {
		t.Fatalf("invalid field coverage counters must not produce ratios: %+v", invalid)
	}
}

func (providerProbeQuoteFake) FetchQuote(_ context.Context, symbol string) (*model.Quote, error) {
	now := time.Now().UTC()
	price, change, pct, open, high, low, prev, amount := 10.0, 1.0, 11.111111, 9.0, 11.0, 8.0, 9.0, 100000.0
	volume := int64(10000)
	return &model.Quote{Symbol: symbol, Name: "测试", CurrentPrice: &price, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &volume, Amount: &amount, QuoteTime: &now, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: now, NormalizedAt: now, Source: "probe-test", SourceVersion: "probe-quote-v1", VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}, nil
}

type providerProbeParseOnlyQuoteFake struct{}

func (providerProbeParseOnlyQuoteFake) Name() string { return "probe-parse-only-quote" }

func (providerProbeParseOnlyQuoteFake) FetchQuote(ctx context.Context, symbol string) (*model.Quote, error) {
	quote, err := (providerProbeQuoteFake{}).FetchQuote(ctx, symbol)
	if err != nil {
		return nil, err
	}
	quote.CurrentPrice = nil
	report := validation.NewReport(10)
	report.Add(validation.FieldIssue{
		Stage: validation.StageParse, Field: "raw_price", ProviderField: "meta.regularMarketPrice",
		Code: "optional_parse_warning", Expected: "numeric source field",
		Severity: validation.SeverityWarning, Message: "adapter retained a parse warning",
	})
	validation.AddContextReport(ctx, report)
	return quote, nil
}

type providerProbeValidatedQuoteReportedFake struct{}

func (providerProbeValidatedQuoteReportedFake) Name() string { return "probe-validated-quote" }

func (providerProbeValidatedQuoteReportedFake) FetchQuote(ctx context.Context, symbol string) (*model.Quote, error) {
	quote, err := (providerProbeQuoteFake{}).FetchQuote(ctx, symbol)
	if err != nil {
		return nil, err
	}
	quote.Amount = nil
	quote.AmountUnit = "unknown"
	quote.QualityStatus = model.QualityDegraded
	quote.QualityReasons = []string{"provider_amount_unavailable"}
	report, validationErr := normalize.ValidateQuoteWithReport(quote)
	validation.AddContextReport(ctx, report)
	if validationErr != nil {
		return nil, validationErr
	}
	return quote, nil
}

func TestProviderProbeStillValidatesQuoteWhenAdapterOnlyReportsParseIssues(t *testing.T) {
	p := providerProbeParseOnlyQuoteFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"quote"}, Checks: []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "quote")
	if !found || item.Status != "failed" || item.ValidationTotalCount != 2 ||
		item.ValidationWarningCount != 1 || item.ValidationErrorCount != 1 {
		t.Fatalf("parse-only report must not bypass common quote validation: %+v", run.Items)
	}
	stages := map[string]bool{}
	for _, issue := range item.ValidationIssues {
		stages[string(issue.Stage)] = true
	}
	if !stages[string(validation.StageParse)] || !stages[string(validation.StageValidate)] {
		t.Fatalf("expected both parse and validate issues, got %+v", item.ValidationIssues)
	}
}

func TestProviderProbeDoesNotDuplicateAdapterQuoteValidationIssues(t *testing.T) {
	p := providerProbeValidatedQuoteReportedFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"quote"}, Checks: []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "quote")
	if !found || item.Status != "returned_degraded" || item.ValidationTotalCount != 1 ||
		item.ValidationWarningCount != 1 || len(item.ValidationIssues) != 1 {
		t.Fatalf("adapter validation report must be reused without duplicate counting: %+v", run.Items)
	}
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

func TestProviderProbeReportsQuoteIdentityMismatch(t *testing.T) {
	p := providerProbeMismatchedIdentityFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilities(context.Background())
	if err != nil {
		t.Fatalf("ProbeProviderCapabilities() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "quote")
	if !found || item.Status != "failed" || item.ErrorCode != "contract_invalid" {
		t.Fatalf("expected mismatched quote identity to fail, got %+v", run)
	}
	if item.ValidationTotalCount != 1 || item.ValidationErrorCount != 1 || len(item.ValidationIssues) != 1 {
		t.Fatalf("expected one structured identity issue, got %+v", item)
	}
	issue := item.ValidationIssues[0]
	if issue.Field != "symbol" || issue.ProviderField != "symbol" || issue.Code != "identity_mismatch" ||
		issue.Expected != "600519.SH" || issue.ActualPreview != "000001.SZ" || issue.Severity != string(validation.SeverityError) {
		t.Fatalf("unexpected identity issue: %+v", issue)
	}
}

type providerProbeMismatchedIdentityFake struct{}

func (providerProbeMismatchedIdentityFake) Name() string { return "probe-mismatched-identity" }
func (providerProbeMismatchedIdentityFake) FetchQuote(ctx context.Context, symbol string) (*model.Quote, error) {
	quote, err := (providerProbeQuoteFake{}).FetchQuote(ctx, symbol)
	if err != nil {
		return nil, err
	}
	quote.Symbol = "000001.SZ"
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
		{Scope: "all", Capabilities: []string{"quote"}, DailyBarStartDate: "2025-07-07", DailyBarEndDate: "2025-07-17", TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarStartDate: "2025-07-07", TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarStartDate: "bad-date", DailyBarEndDate: "2025-07-17", TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarStartDate: "2025-07-07", DailyBarEndDate: "2025-08-08", TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarStartDate: "2027-01-01", DailyBarEndDate: "2027-01-02", TimeoutSeconds: 5},
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
	rawAmount := amount
	now := time.Now().UTC()
	return []model.DailyBar{{
		Symbol: symbol, TradeDate: "2026-10-29", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: p.close, Volume: &volume, Amount: &amount,
		AdjustmentBasis: adjustment, Source: p.name, SourceVersion: p.name + "-v1",
		ProviderTime: &now, FetchedAt: now, NormalizedAt: now,
		VolumeUnit: "shares", RawVolumeUnit: "shares",
		RawAmount: &rawAmount, RawAmountUnit: "CNY", RawAmountField: "amount",
		AmountUnit: "CNY", QualityStatus: p.quality,
	}}, nil
}

type providerProbeAdjustedDailyBarsFake struct {
	providerProbeDailyBarsFake
	method  string
	refDate string
}

func (p providerProbeAdjustedDailyBarsFake) FetchDailyBars(ctx context.Context, symbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	bars, err := p.providerProbeDailyBarsFake.FetchDailyBars(ctx, symbol, startDate, endDate, limit, adjustment)
	if err != nil {
		return nil, err
	}
	for i := range bars {
		bars[i].AdjustmentMethod = p.method
		bars[i].AdjustmentRefDate = p.refDate
	}
	return bars, nil
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

type providerProbeParseOnlyDailyBarsFake struct {
	providerProbeDailyBarsFake
}

func (p providerProbeParseOnlyDailyBarsFake) FetchDailyBars(ctx context.Context, symbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	bars, err := p.providerProbeDailyBarsFake.FetchDailyBars(ctx, symbol, startDate, endDate, limit, adjustment)
	if err != nil {
		return nil, err
	}
	if len(bars) > 0 {
		invalidAmount := -1.0
		bars[0].Amount = &invalidAmount
	}
	report := validation.NewReport(10)
	report.Add(validation.FieldIssue{
		Stage: validation.StageParse, Field: "raw_amount", ProviderField: "amount",
		Code: "optional_parse_warning", Expected: "numeric source field",
		Severity: validation.SeverityWarning, Message: "adapter retained a parse warning",
	})
	validation.AddContextReport(ctx, report)
	return bars, nil
}

type providerProbeHistoricalDailyBarsFake struct {
	name      string
	startDate string
	endDate   string
	limit     int
	rowCount  int
}

func (p *providerProbeHistoricalDailyBarsFake) Name() string { return p.name }
func (p *providerProbeHistoricalDailyBarsFake) FetchDailyBars(_ context.Context, symbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	p.startDate, p.endDate, p.limit = startDate, endDate, limit
	tradeDate := startDate
	if tradeDate == "" {
		tradeDate = endDate
	}
	end, err := time.ParseInLocation("2006-01-02", tradeDate, marketLocation())
	if err != nil {
		return nil, err
	}
	count := p.rowCount
	if count <= 0 {
		count = 1
	}
	now := time.Now().UTC()
	volume := int64(10000)
	amount := 100000.0
	bars := make([]model.DailyBar, 0, count)
	for day := end; len(bars) < count; day = day.AddDate(0, 0, -1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		bars = append(bars, model.DailyBar{
			Symbol: symbol, TradeDate: day.Format("2006-01-02"), Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, Amount: &amount, AdjustmentBasis: adjustment, AdjustmentMethod: "historical-test-none",
			Source: p.name, SourceVersion: p.name + "-v1", ProviderTime: &now, FetchedAt: now, NormalizedAt: now,
			VolumeUnit: "shares", RawVolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
		})
	}
	return bars, nil
}

func TestProviderProbeDefaultDailyBarsExcludeCurrentIncompleteDay(t *testing.T) {
	p := &providerProbeHistoricalDailyBarsFake{name: "default-range-bars"}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Symbols: []string{"600036.SH"},
		DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone},
		Checks:              []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("default Probe failed: %v", err)
	}
	wantEnd := time.Now().In(marketLocation()).AddDate(0, 0, -1).Format("2006-01-02")
	if p.startDate != "" || p.endDate != wantEnd || p.limit != 5 {
		t.Fatalf("default Probe must request at most the previous complete day: start=%q end=%q limit=%d, want end=%q limit=5", p.startDate, p.endDate, p.limit, wantEnd)
	}
	item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
	if !found || item.SampleDate != wantEnd {
		t.Fatalf("default Probe should retain the latest completed date as sample evidence: found=%v item=%+v want=%s", found, item, wantEnd)
	}
}

func TestProviderProbeCountsValidatedRowsBeforeLimitAndDoesNotDegradeForTruncation(t *testing.T) {
	p := &providerProbeHistoricalDailyBarsFake{name: "limited-history-bars", rowCount: 7}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Symbols: []string{"600036.SH"},
		DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone},
		Checks:              []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("default Probe failed: %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
	if !found {
		t.Fatalf("daily_bars result not found: %+v", run.Items)
	}
	if item.RowsReturned != 7 || item.RowsValidated != 7 {
		t.Fatalf("row counters must describe raw and pre-limit validated populations: returned=%d validated=%d", item.RowsReturned, item.RowsValidated)
	}
	if item.ValidationInfoCount != 2 {
		t.Fatalf("intentional limit must be recorded as two informational dropped rows: %+v", item)
	}
	foundTruncation := false
	for _, issue := range item.ValidationIssues {
		if issue.Code == "bars_truncated_to_limit" && issue.Count == 2 && issue.Severity == "info" {
			foundTruncation = true
		}
	}
	if !foundTruncation {
		t.Fatalf("truncation count must be explicit and informational: %+v", item.ValidationIssues)
	}
	for _, reason := range item.QualityReasons {
		if reason == "bars_truncated_to_limit" {
			t.Fatalf("intentional limit must not appear as a quality-degrading reason: %+v", item.QualityReasons)
		}
	}
}

func TestProviderProbePassesExplicitHistoricalRangeToDailyBarProvider(t *testing.T) {
	p := &providerProbeHistoricalDailyBarsFake{name: "historical-bars"}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Symbols: []string{"600036.SH"},
		DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone},
		DailyBarStartDate:   "2025-07-07", DailyBarEndDate: "2025-07-17",
		Checks: []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("historical Probe failed: %v", err)
	}
	if p.startDate != "2025-07-07" || p.endDate != "2025-07-17" || p.limit != 2000 {
		t.Fatalf("historical range not passed to Provider: start=%q end=%q limit=%d", p.startDate, p.endDate, p.limit)
	}
	if run.RequestedDailyBarStartDate != "2025-07-07" || run.RequestedDailyBarEndDate != "2025-07-17" {
		t.Fatalf("historical request range not echoed in run: %+v", run)
	}
}

func TestProviderProbeRunsRequestedDailyBarAdjustmentsIndependently(t *testing.T) {
	p := providerProbeDailyBarsFake{name: "probe-adjustment-bars", close: 10.5, quality: model.QualityValid}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	adjustments := []model.AdjustmentBasis{model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward}
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarAdjustments: adjustments,
		Checks: []string{"cross_provider"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	if run.Total != len(adjustments) || len(run.RequestedDailyBarAdjustments) != len(adjustments) {
		t.Fatalf("expected one independent probe per adjustment: %+v", run)
	}
	seen := map[model.AdjustmentBasis]bool{}
	for _, item := range run.Items {
		if item.Status != "returned_degraded" || item.AdjustmentBasis == "" || seen[item.AdjustmentBasis] {
			t.Fatalf("unexpected or duplicate adjustment result: %+v", item)
		}
		seen[item.AdjustmentBasis] = true
	}
	for _, adjustment := range adjustments {
		if !seen[adjustment] {
			t.Fatalf("missing adjustment result for %s: %+v", adjustment, run.Items)
		}
	}
}

func TestProviderProbeDailyBarCrossComparisonSeparatesAdjustmentBases(t *testing.T) {
	left := providerProbeDailyBarsFake{name: "probe-adjustment-left", close: 10.5, quality: model.QualityValid}
	right := providerProbeDailyBarsFake{name: "probe-adjustment-right", close: 10.5, quality: model.QualityValid}
	ds := NewWithCapabilities(
		[]provider.NamedProvider{left, right}, nil, []provider.DailyBarProvider{left, right},
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
	)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"},
		DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward},
		Checks:              []string{"cross_provider"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	if len(run.CheckResults) != 3 {
		t.Fatalf("expected one cross-provider comparison per adjustment basis, got %+v", run.CheckResults)
	}
	seen := map[string]bool{}
	for _, result := range run.CheckResults {
		if result.Check != "cross_provider" || result.AdjustmentBasis == "" || seen[result.AdjustmentBasis] {
			t.Fatalf("unexpected or duplicated adjustment comparison: %+v", result)
		}
		seen[result.AdjustmentBasis] = true
	}
}

func TestProviderProbeClassifiesExplicitUnsupportedAdjustment(t *testing.T) {
	for _, message := range []string{
		"adjustment not supported",
		"unsupported adjustment",
		"provider does not support adjustment",
		"Sina historical K-line endpoint only supports verified adjustment basis \"none\"",
		"Netease historical CSV only supports verified adjustment basis \"none\"",
		"不支持的同花顺复权口径",
	} {
		if !isUnsupportedDailyBarAdjustmentError(errors.New(message)) {
			t.Errorf("expected unsupported adjustment classification for %q", message)
		}
	}
	for _, message := range []string{
		"upstream timeout",
		"Eastmoney daily-bars provider error rc=-1",
		"BaoStock adjustment mismatch: requested flag 2, got \"3\"",
		"TickFlow daily K-line request: HTTP status 503",
		"同花顺历史 K 线区间请求失败: HTTP 状态码 401",
		"Pytdx ex-rights/ex-dividend data required for forward_adjusted: connection refused",
		"Pytdx ex-rights/ex-dividend data required for backward_adjusted: context deadline exceeded",
		"adj_factor missing for requested date range",
	} {
		if isUnsupportedDailyBarAdjustmentError(errors.New(message)) {
			t.Errorf("provider/network/data failures must not be classified as unsupported: %q", message)
		}
	}
}

func TestProviderProbeRejectsInvalidDailyBarAdjustments(t *testing.T) {
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	cases := []ProviderProbeOptions{
		{Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarAdjustments: []model.AdjustmentBasis{"unknown"}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"daily_bars"}, DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone, model.AdjustmentNone}, TimeoutSeconds: 5},
		{Scope: "all", Capabilities: []string{"quote"}, DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone}, TimeoutSeconds: 5},
	}
	for i, options := range cases {
		if _, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), options); err == nil {
			t.Errorf("case %d should reject invalid daily-bar adjustments: %+v", i, options)
		}
	}
}

func TestProviderProbeStillValidatesDailyBarsWhenAdapterOnlyReportsParseIssues(t *testing.T) {
	p := providerProbeParseOnlyDailyBarsFake{providerProbeDailyBarsFake{name: "probe-parse-only-bars", close: 10.5, quality: model.QualityValid}}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Checks: []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
	if !found || item.Status != "failed" || item.ValidationWarningCount != 1 || item.ValidationErrorCount == 0 {
		t.Fatalf("parse-only report must not bypass daily-bar validation: %+v", run.Items)
	}
	stages := map[string]bool{}
	for _, issue := range item.ValidationIssues {
		stages[string(issue.Stage)] = true
	}
	if !stages[string(validation.StageParse)] || !stages[string(validation.StageValidate)] {
		t.Fatalf("expected both parse and validate issues, got %+v", item.ValidationIssues)
	}
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
	left := providerProbeDailyBarsFake{name: "pytdx", close: 10.5, quality: model.QualityValid}
	right := providerProbeDailyBarsFake{name: "baostock", close: 10.5, quality: model.QualityValid}
	ds := NewWithCapabilities(
		[]provider.NamedProvider{left, right}, nil, []provider.DailyBarProvider{left, right},
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
	)
	ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{
		providerProbeContextCalendarFake{quality: model.QualityValid},
	})
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"},
		DailyBarStartDate: "2026-10-29", DailyBarEndDate: "2026-10-29",
		Checks: []string{"cross_provider"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	if len(run.CheckResults) != 1 {
		t.Fatalf("expected one cross-provider result, got %+v", run.CheckResults)
	}
	result := run.CheckResults[0]
	if result.Status != "passed" || result.Provider != "pytdx+baostock" || result.FieldCount != 6 || len(result.ComparedDates) != 1 {
		t.Fatalf("unexpected cross-provider comparison result: %+v", result)
	}
}

func TestProviderProbeDailyBarCrossComparisonEnforcesAdjustmentMetadata(t *testing.T) {
	cases := []struct {
		name         string
		leftMethod   string
		rightMethod  string
		leftRefDate  string
		rightRefDate string
		wantStatus   string
		wantReason   string
	}{
		{name: "different methods", leftMethod: "eastmoney_fqt_2", rightMethod: "baostock_pct_change", leftRefDate: "2026-11-03", rightRefDate: "2026-11-03", wantStatus: "skipped", wantReason: "adjustment_method_mismatch"},
		{name: "missing reference date", leftMethod: "same_hfq_v1", rightMethod: "same_hfq_v1", leftRefDate: "2026-11-03", wantStatus: "skipped", wantReason: "right_adjustment_reference_date_missing"},
		{name: "different reference dates", leftMethod: "same_hfq_v1", rightMethod: "same_hfq_v1", leftRefDate: "2026-11-03", rightRefDate: "2026-10-30", wantStatus: "skipped", wantReason: "adjustment_reference_date_mismatch"},
		{name: "compatible metadata", leftMethod: "shared_hfq_v1", rightMethod: "shared_hfq_v1", leftRefDate: "2026-11-03", rightRefDate: "2026-11-03", wantStatus: "passed"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			left := providerProbeAdjustedDailyBarsFake{
				providerProbeDailyBarsFake: providerProbeDailyBarsFake{name: "pytdx", close: 10.5, quality: model.QualityValid},
				method:                     test.leftMethod, refDate: test.leftRefDate,
			}
			right := providerProbeAdjustedDailyBarsFake{
				providerProbeDailyBarsFake: providerProbeDailyBarsFake{name: "baostock", close: 10.5, quality: model.QualityValid},
				method:                     test.rightMethod, refDate: test.rightRefDate,
			}
			ds := NewWithCapabilities(
				[]provider.NamedProvider{left, right}, nil, []provider.DailyBarProvider{left, right},
				cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
			)
			ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{
				providerProbeContextCalendarFake{quality: model.QualityValid},
			})
			run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
				Scope: "all", Capabilities: []string{"daily_bars"},
				DailyBarStartDate: "2026-10-29", DailyBarEndDate: "2026-10-29",
				DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentBackward},
				Checks:              []string{"cross_provider"}, TimeoutSeconds: 5,
			})
			if err != nil {
				t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
			}
			if len(run.CheckResults) != 1 {
				t.Fatalf("expected one cross-provider result, got %+v", run.CheckResults)
			}
			result := run.CheckResults[0]
			if result.Status != test.wantStatus || result.ErrorMessage != test.wantReason {
				t.Fatalf("unexpected adjustment gate result: %+v", result)
			}
			if result.LeftAdjustmentMethod != test.leftMethod || result.RightAdjustmentMethod != test.rightMethod ||
				result.LeftAdjustmentRefDate != test.leftRefDate || result.RightAdjustmentRefDate != test.rightRefDate {
				t.Fatalf("probe did not preserve both providers' adjustment evidence: %+v", result)
			}
		})
	}
}

func TestBuildCrossProviderProbeCheckResultsReportsUnavailableProvider(t *testing.T) {
	snapshot := &provider.CrossProviderDailySnapshot{
		Provider: "pytdx", SourceVersion: "pytdx-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2025-07-07", Open: 10, High: 11, Low: 9, Close: 10,
			Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1,
		}},
	}
	items := []model.ProviderProbeItem{{
		Provider: "baostock", Capability: "daily_bars", AdjustmentBasis: model.AdjustmentNone,
		Status: "failed", SampleSymbol: "600036.SH",
		ErrorCode: "network_error", ErrorStage: "connect", ErrorMessage: "connection timeout",
	}}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{snapshot}, items, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected one provider-unavailable pair result, got %+v", results)
	}
	result := results[0]
	if result.Status != "failed" || result.ErrorCode != "provider_unavailable" ||
		result.LeftProvider != "pytdx" || result.RightProvider != "baostock" ||
		result.ComparedSymbol != "600036.SH" || result.AdjustmentBasis != "none" ||
		!strings.Contains(result.ErrorMessage, "error_code=network_error") ||
		!strings.Contains(result.ErrorMessage, "error_stage=connect") {
		t.Fatalf("provider failure must remain explicit and attributable: %+v", result)
	}
}

func TestBuildCrossProviderProbeCheckResultsMatchesFailedProviderBySymbol(t *testing.T) {
	snapshots := []*provider.CrossProviderDailySnapshot{
		{
			Provider: "pytdx", SourceVersion: "pytdx-v1", Symbol: "600036.SH",
			AdjustmentBasis: "none", QualityStatus: "valid",
		},
		{
			Provider: "pytdx", SourceVersion: "pytdx-v1", Symbol: "002982.SZ",
			AdjustmentBasis: "none", QualityStatus: "valid",
		},
	}
	items := []model.ProviderProbeItem{{
		Provider: "baostock", Capability: "daily_bars", AdjustmentBasis: model.AdjustmentNone,
		Status: "failed", SampleSymbol: "600036.SH", ErrorCode: "network_error",
		ErrorStage: "connect", ErrorMessage: "connection timeout",
	}}
	results := buildCrossProviderProbeCheckResults(snapshots, items, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("failed provider must only pair with its requested symbol, got %+v", results)
	}
	if results[0].ComparedSymbol != "600036.SH" || results[0].RightProvider != "baostock" {
		t.Fatalf("failed provider was paired with the wrong symbol: %+v", results[0])
	}
}

func TestBuildCrossProviderProbeCheckResultsPreservesAdjustmentEvidence(t *testing.T) {
	left := &provider.CrossProviderDailySnapshot{
		Provider: "left", SourceVersion: "left-v1", Symbol: "600036.SH",
		AdjustmentBasis: "backward_adjusted", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1, AdjustmentMethod: "left_hfq", AdjustmentRefDate: "2026-11-03"}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "backward_adjusted", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1, AdjustmentMethod: "right_hfq", AdjustmentRefDate: "2026-11-03"}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, nil, time.Now().UTC())
	if len(results) != 1 {
		t.Fatalf("expected one adjusted comparison result, got %+v", results)
	}
	result := results[0]
	if result.Status != "skipped" || result.ErrorMessage != "adjustment_method_mismatch" ||
		result.LeftAdjustmentMethod != "left_hfq" || result.RightAdjustmentMethod != "right_hfq" ||
		result.LeftAdjustmentRefDate != "2026-11-03" || result.RightAdjustmentRefDate != "2026-11-03" {
		t.Fatalf("adjustment evidence missing or incorrectly passed: %+v", result)
	}
}

func TestBuildCrossProviderProbeCheckResultsRejectsDegradedSnapshots(t *testing.T) {
	left := &provider.CrossProviderDailySnapshot{
		Provider: "left", SourceVersion: "left-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "degraded",
		Bars: []provider.CrossProviderDailyBar{{TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10, Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, nil, time.Now().UTC())
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
			Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1,
		}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1,
		}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, nil, time.Now().UTC())
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
			Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1,
		}},
	}
	right := &provider.CrossProviderDailySnapshot{
		Provider: "right", SourceVersion: "right-v1", Symbol: "600036.SH",
		AdjustmentBasis: "none", QualityStatus: "valid",
		Bars: []provider.CrossProviderDailyBar{{
			TradeDate: "2026-10-29", Open: 10, High: 11, Low: 9, Close: 10.7,
			Volume: 100, VolumeUnit: "shares", VolumeGranularity: 1,
		}},
	}
	results := buildCrossProviderProbeCheckResults([]*provider.CrossProviderDailySnapshot{left, right}, nil, time.Now().UTC())
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

type providerProbeSecurityTradingStatusFake struct {
	status model.SuspensionStatus
	fail   bool
}

func (p providerProbeSecurityTradingStatusFake) Name() string {
	return "probe-freshness-security-status"
}

func (p providerProbeSecurityTradingStatusFake) FetchSecurityTradingStatus(_ context.Context, symbol, tradeDate string) (*model.SecurityTradingStatusSnapshot, error) {
	if p.fail {
		return nil, errors.New("security status unavailable")
	}
	return &model.SecurityTradingStatusSnapshot{
		Symbol: symbol, TradeDate: tradeDate, SecurityName: "测试证券",
		NameEffectiveFrom: "2020-01-01", ListingDate: "2020-01-01", ListingTradingDayNumber: 1,
		ListingStatusKnown: true, RiskWarningStatus: model.RiskWarningNormal,
		DelistingStatus: model.DelistingStatusNormal, IsTradingDay: true, TradingDayKnown: true,
		SuspensionStatus: p.status, Source: p.Name(), SourceVersion: "security-status-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
	}, nil
}

func TestApplyFreshnessProbeResultsUsesSecurityTradingStatusEvidence(t *testing.T) {
	location := marketLocation()
	checkedAt := time.Date(2026, 11, 2, 10, 0, 0, 0, location).UTC()
	sourceTime := checkedAt.Add(-30 * time.Second)
	fetchedAt := checkedAt.Add(-time.Second)
	tests := []struct {
		name          string
		status        model.SuspensionStatus
		fail          bool
		wantStatus    string
		wantFreshness string
		wantReason    string
		wantSecurity  string
	}{
		{name: "full suspension", status: model.SuspensionStatusSuspended, wantStatus: "skipped", wantFreshness: "not_applicable", wantReason: freshness.ReasonSecuritySuspended, wantSecurity: "suspended"},
		{name: "intraday suspension", status: model.SuspensionStatusIntraday, wantStatus: "skipped", wantFreshness: "not_applicable", wantReason: freshness.ReasonSecuritySuspended, wantSecurity: "intraday_suspension"},
		{name: "resumed", status: model.SuspensionStatusResumed, wantStatus: "passed", wantFreshness: "fresh", wantReason: freshness.ReasonWithinSLA, wantSecurity: "normal"},
		{name: "no suspension event", status: model.SuspensionStatusNoEvent, wantStatus: "passed", wantFreshness: "fresh", wantReason: freshness.ReasonWithinSLA, wantSecurity: "normal"},
		{name: "missing security evidence", fail: true, wantStatus: "skipped", wantFreshness: "unknown", wantReason: freshness.ReasonSecurityStatusUnknown, wantSecurity: "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
			ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{providerProbeFreshnessCalendarFake{}})
			ds.SetSecurityTradingStatusProviders([]provider.SecurityTradingStatusProvider{providerProbeSecurityTradingStatusFake{status: tc.status, fail: tc.fail}})
			run := &model.ProviderProbeRun{
				FinishedAt: checkedAt, TimeoutSeconds: 5, RequestedSymbols: []string{"600519.SH"},
				Items: []model.ProviderProbeItem{{
					Provider: "quote-provider", Capability: "quote", SampleSymbol: "600519.SH",
					ProviderTime: &sourceTime, ProviderTimeType: "provider_snapshot_time", FetchedAt: &fetchedAt,
				}},
				CheckResults: []model.ProviderProbeCheckResult{{
					Check: "freshness", Provider: "quote-provider", Capability: "quote",
					ProviderTime: &sourceTime, ProviderTimeType: "provider_snapshot_time",
					FetchedAt: &fetchedAt, CheckedAt: checkedAt,
				}},
			}
			ds.applyFreshnessProbeResults(context.Background(), run)
			result := run.CheckResults[0]
			if result.Status != tc.wantStatus || result.FreshnessStatus != tc.wantFreshness ||
				result.FreshnessReason != tc.wantReason || result.SecurityTradingStatus != tc.wantSecurity ||
				result.MarketPhase != model.MarketSessionContinuousAM {
				t.Fatalf("unexpected quote freshness context result: %+v", result)
			}
		})
	}
}

func TestApplyFreshnessProbeResultsUsesPolicyAndMarketSession(t *testing.T) {
	location := marketLocation()
	checkedAt := time.Date(2026, 11, 3, 10, 0, 0, 0, location).UTC()
	sourceTime := checkedAt.Add(-30 * time.Second)
	fetchedAt := checkedAt.Add(-time.Second)
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{providerProbeFreshnessCalendarFake{}})
	run := &model.ProviderProbeRun{
		FinishedAt: checkedAt,
		Items: []model.ProviderProbeItem{{
			Provider: "breadth-provider", Capability: "market_breadth",
			ProviderTime: &sourceTime, ProviderTimeType: "oldest_component_quote_time", FetchedAt: &fetchedAt,
		}},
		CheckResults: []model.ProviderProbeCheckResult{{
			Check: "freshness", Provider: "breadth-provider", Capability: "market_breadth",
			ProviderTime: &sourceTime, ProviderTimeType: "oldest_component_quote_time",
			FetchedAt: &fetchedAt, CheckedAt: checkedAt,
		}},
	}
	ds.applyFreshnessProbeResults(context.Background(), run)
	result := run.CheckResults[0]
	if result.Status != "passed" || result.FreshnessStatus != "fresh" ||
		result.FreshnessReason != "within_sla" || result.FreshnessDomain != "market_breadth" ||
		result.MarketPhase != model.MarketSessionContinuousAM || result.AgeSeconds == nil || *result.AgeSeconds != 30 ||
		result.ProviderTimeType != "oldest_component_quote_time" || result.FetchedAt == nil ||
		result.CalendarKnown == nil || !*result.CalendarKnown || result.IsTradingDay == nil || !*result.IsTradingDay {
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
			ProviderTime: &sourceTime, ProviderTimeType: "provider_snapshot_time", CheckedAt: checkedAt,
		}},
	}
	ds.applyFreshnessProbeResults(context.Background(), run)
	result := run.CheckResults[0]
	if result.Status != "skipped" || result.FreshnessStatus != "unknown" ||
		result.FreshnessReason != "trading_calendar_evidence_missing" {
		t.Fatalf("missing calendar evidence must not be reported fresh: %+v", result)
	}
}

// TestRecordProviderProbeFetchedAtKeepsCollectionTimeSeparateFromSourceTime.
func TestRecordProviderProbeFetchedAtKeepsCollectionTimeSeparateFromSourceTime(t *testing.T) {
	evidence := &providerProbeEvidence{}
	ctx := context.WithValue(context.Background(), providerProbeEvidenceContextKey{}, evidence)
	fetchedAt := time.Date(2026, 11, 3, 10, 0, 2, 0, time.UTC)
	recordProviderProbeFetchedAt(ctx, fetchedAt)
	if evidence.fetchedAt == nil || !evidence.fetchedAt.Equal(fetchedAt) {
		t.Fatalf("collection timestamp was not captured: %+v", evidence)
	}
	if evidence.providerTime != nil || evidence.providerTimeType != "" {
		t.Fatalf("collection timestamp must not be promoted to source timestamp: %+v", evidence)
	}
	recordProviderProbeFetchedAt(ctx, time.Time{})
	if evidence.fetchedAt == nil || !evidence.fetchedAt.Equal(fetchedAt) {
		t.Fatalf("zero collection timestamp must not erase valid evidence: %+v", evidence)
	}
}

func containsProbeReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected || strings.HasPrefix(reason, expected) {
			return true
		}
	}
	return false
}

type providerProbeEmptyDailyBarsFake struct{}

func (providerProbeEmptyDailyBarsFake) Name() string { return "probe-empty-bars" }

func (providerProbeEmptyDailyBarsFake) FetchDailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, error) {
	return nil, nil
}

func TestProviderProbeKeepsEmptyDailyBarResponseUnknownWhenContextUnavailable(t *testing.T) {
	p := providerProbeEmptyDailyBarsFake{}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Checks: []string{"quality"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
	if !found || item.Status != "returned_unknown" {
		t.Fatalf("empty response without context must remain unknown, not failed: %+v", run.Items)
	}
	if !containsProbeReason(item.QualityReasons, "daily_bar_empty_response") ||
		!containsProbeReason(item.QualityReasons, "daily_bar_context_calendar_unavailable") {
		t.Fatalf("empty response context reasons are missing: %+v", item)
	}
}

type providerProbePytdxDailyBarFailureFake struct {
	err error
}

func (p providerProbePytdxDailyBarFailureFake) Name() string { return "pytdx" }

func (p providerProbePytdxDailyBarFailureFake) FetchDailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, error) {
	return nil, p.err
}

func TestProviderProbeDoesNotClassifyPytdxTransportFailureAsNotSupported(t *testing.T) {
	transportErr := errors.New("Pytdx ex-rights/ex-dividend data required for forward_adjusted adjustment: dial tcp 127.0.0.1:7709: i/o timeout")
	p := providerProbePytdxDailyBarFailureFake{err: transportErr}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)

	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"}, Symbols: []string{"600519.SH"},
		DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentForward}, Checks: []string{"connectivity", "contract"},
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
	if !found {
		t.Fatalf("missing Pytdx daily-bar probe item: %+v", run.Items)
	}
	if item.Status == "not_supported" || item.Status != "failed" {
		t.Fatalf("transport failure must remain failed, not not_supported: %+v", item)
	}
}

func TestProviderProbeCrossProviderSnapshotUsesTradingContextQuality(t *testing.T) {
	left := providerProbeDailyBarsFake{name: "probe-context-left", close: 10.5, quality: model.QualityValid}
	right := providerProbeDailyBarsFake{name: "probe-context-right", close: 10.5, quality: model.QualityValid}
	ds := NewWithCapabilities(
		[]provider.NamedProvider{left, right}, nil, []provider.DailyBarProvider{left, right},
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
	)
	ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{
		providerProbeContextCalendarFake{quality: model.QualityValid},
	})
	ds.SetSecurityTradingStatusProviders([]provider.SecurityTradingStatusProvider{
		providerProbeContextSecurityStatusFake{fail: true},
	})
	run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
		Scope: "all", Capabilities: []string{"daily_bars"},
		DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone},
		Checks:              []string{"quality", "cross_provider"}, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
	}
	var crossProviderResults []model.ProviderProbeCheckResult
	qualityResults := 0
	for _, result := range run.CheckResults {
		switch result.Check {
		case "quality":
			qualityResults++
			if result.Status != "degraded" {
				t.Errorf("trading-context degradation must be visible in provider quality result: %+v", result)
			}
		case "cross_provider":
			crossProviderResults = append(crossProviderResults, result)
		}
	}
	if qualityResults != 2 {
		t.Fatalf("expected two provider quality results, got %+v", run.CheckResults)
	}
	if len(crossProviderResults) != 1 {
		t.Fatalf("expected one cross-provider comparison, got %+v", run.CheckResults)
	}
	result := crossProviderResults[0]
	if result.Status != "skipped" || result.ErrorCode != "comparison_not_comparable" ||
		result.LeftQualityStatus != model.QualityDegraded || result.RightQualityStatus != model.QualityDegraded {
		t.Fatalf("cross-provider snapshots must inherit degraded trading-context quality: %+v", result)
	}
	for _, item := range run.Items {
		if item.Status != "returned_degraded" ||
			!containsProbeReason(item.QualityReasons, "daily_bar_context_security_status_unavailable:") {
			t.Fatalf("provider result should retain the context degradation: %+v", item)
		}
	}
}

type providerProbeContextCalendarFake struct {
	quality         model.QualityStatus
	reasons         []string
	conflictDate    string
	conflictTrading bool
}

func (p providerProbeContextCalendarFake) Name() string { return "probe-context-calendar" }

func (p providerProbeContextCalendarFake) FetchTradingCalendar(_ context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}
	items := make([]model.TradingCalendarDay, 0, int(end.Sub(start).Hours()/24)+1)
	tradingDays := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		isTrading := day.Weekday() != time.Saturday && day.Weekday() != time.Sunday
		if p.conflictDate == day.Format("2006-01-02") {
			isTrading = p.conflictTrading
		}
		items = append(items, model.TradingCalendarDay{Date: day.Format("2006-01-02"), IsTradingDay: isTrading})
		if isTrading {
			tradingDays++
		}
	}
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: startDate, EndDate: endDate,
		TotalDays: len(items), TradingDays: tradingDays, Items: items,
		Source: p.Name(), SourceVersion: "calendar-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: p.quality, QualityReasons: append([]string(nil), p.reasons...),
	}, nil
}

type providerProbeContextDailyBarsFake struct{ tradeDate string }

func (providerProbeContextDailyBarsFake) Name() string { return "probe-context-bars" }

func (p providerProbeContextDailyBarsFake) FetchDailyBars(_ context.Context, symbol, _, _ string, _ int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	volume, amount := int64(10000), 100000.0
	now := time.Now().UTC()
	return []model.DailyBar{{
		Symbol: symbol, TradeDate: p.tradeDate, Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount,
		AdjustmentBasis: adjustment, Source: p.Name(), SourceVersion: "bars-v1",
		ProviderTime: &now, FetchedAt: now, NormalizedAt: now,
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
	}}, nil
}

type providerProbeContextSecurityStatusFake struct {
	status model.SuspensionStatus
	fail   bool
}

func (p providerProbeContextSecurityStatusFake) Name() string { return "probe-context-security-status" }

func (p providerProbeContextSecurityStatusFake) FetchSecurityTradingStatus(_ context.Context, symbol, tradeDate string) (*model.SecurityTradingStatusSnapshot, error) {
	if p.fail {
		return nil, errors.New("security status unavailable")
	}
	return &model.SecurityTradingStatusSnapshot{
		Symbol: symbol, TradeDate: tradeDate, SecurityName: "测试证券",
		NameEffectiveFrom: "2020-01-01", ListingDate: "2020-01-01", ListingTradingDayNumber: 1,
		ListingStatusKnown: true, RiskWarningStatus: model.RiskWarningNormal,
		DelistingStatus: model.DelistingStatusNormal, IsTradingDay: true, TradingDayKnown: true,
		SuspensionStatus: p.status, Source: p.Name(), SourceVersion: "security-status-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
	}, nil
}

func TestProviderProbeDailyBarsReusesTradingContextDiagnosticsEndToEnd(t *testing.T) {
	location := marketLocation()
	barDay := time.Now().In(location).AddDate(0, 0, -7)
	for barDay.Weekday() == time.Saturday || barDay.Weekday() == time.Sunday {
		barDay = barDay.AddDate(0, 0, -1)
	}
	barDate := barDay.Format("2006-01-02")
	cases := []struct {
		name            string
		calendarQuality model.QualityStatus
		calendarReasons []string
		status          model.SuspensionStatus
		statusFail      bool
		conflict        bool
		wantStatus      string
		wantReasons     []string
		forbidReasons   []string
	}{
		{
			name:            "suspended missing dates are excluded",
			calendarQuality: model.QualityValid, status: model.SuspensionStatusSuspended,
			wantStatus: "returned", wantReasons: []string{"daily_bar_excluded_suspended:"},
			forbidReasons: []string{"daily_bar_missing_on_confirmed_trading_day:"},
		},
		{
			name:            "confirmed trading dates missing bars are diagnosed",
			calendarQuality: model.QualityValid, status: model.SuspensionStatusNoEvent,
			wantStatus: "returned_degraded", wantReasons: []string{"daily_bar_missing_on_confirmed_trading_day:"},
		},
		{
			name:            "unavailable security status remains unknown",
			calendarQuality: model.QualityValid, statusFail: true,
			wantStatus: "returned_degraded", wantReasons: []string{"daily_bar_context_security_status_unavailable:"},
			forbidReasons: []string{"daily_bar_missing_on_confirmed_trading_day:"},
		},
		{
			name:            "degraded calendar degrades context but retains suspension evidence",
			calendarQuality: model.QualityDegraded, status: model.SuspensionStatusSuspended,
			wantStatus: "returned_degraded", wantReasons: []string{"daily_bar_context_calendar_degraded", "daily_bar_excluded_suspended:"},
		},
		{
			name:            "conflicting calendars do not confirm missing bars",
			calendarQuality: model.QualityValid, status: model.SuspensionStatusNoEvent, conflict: true,
			wantStatus: "returned_degraded", wantReasons: []string{"daily_bar_context_calendar_sources_disagree"},
			forbidReasons: []string{"daily_bar_missing_on_confirmed_trading_day:"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calendar := providerProbeContextCalendarFake{quality: tc.calendarQuality, reasons: tc.calendarReasons}
			calendars := []provider.TradingCalendarProvider{calendar}
			if tc.conflict {
				conflictDay := barDay.AddDate(0, 0, 1)
				for conflictDay.Weekday() == time.Saturday || conflictDay.Weekday() == time.Sunday {
					conflictDay = conflictDay.AddDate(0, 0, 1)
				}
				conflictDate := conflictDay.Format("2006-01-02")
				calendars = append(calendars, providerProbeContextCalendarFake{
					quality: model.QualityValid, conflictDate: conflictDate, conflictTrading: false,
				})
			}
			p := providerProbeContextDailyBarsFake{tradeDate: barDate}
			ds := NewWithCapabilities([]provider.NamedProvider{p}, nil, []provider.DailyBarProvider{p},
				cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
			ds.SetTradingCalendarProviders(calendars)
			ds.SetSecurityTradingStatusProviders([]provider.SecurityTradingStatusProvider{
				providerProbeContextSecurityStatusFake{status: tc.status, fail: tc.statusFail},
			})
			run, err := ds.ProbeProviderCapabilitiesWithOptions(context.Background(), ProviderProbeOptions{
				Scope: "all", Capabilities: []string{"daily_bars"}, Symbols: []string{"600036.SH"},
				DailyBarAdjustments: []model.AdjustmentBasis{model.AdjustmentNone},
				Checks:              []string{"quality"}, TimeoutSeconds: 5,
			})
			if err != nil {
				t.Fatalf("ProbeProviderCapabilitiesWithOptions() error = %v", err)
			}
			item, found := findProbeItem(run.Items, p.Name(), "daily_bars")
			if !found || item.Status != tc.wantStatus {
				t.Fatalf("unexpected daily-bars probe status: found=%v item=%+v", found, item)
			}
			for _, expected := range tc.wantReasons {
				matched := false
				for _, actual := range item.QualityReasons {
					if strings.HasPrefix(actual, expected) {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("missing reason prefix %q in %+v", expected, item.QualityReasons)
				}
			}
			for _, forbidden := range tc.forbidReasons {
				for _, actual := range item.QualityReasons {
					if strings.HasPrefix(actual, forbidden) {
						t.Errorf("unexpected reason prefix %q in %+v", forbidden, item.QualityReasons)
					}
				}
			}
		})
	}
}


func TestApplyFreshnessProbeResultsDoesNotPromoteBusinessDatesOrFetchedAt(t *testing.T) {
	checkedAt := time.Date(2026, 11, 3, 10, 0, 0, 0, marketLocation()).UTC()
	fetchedAt := checkedAt.Add(-time.Second)
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	ds.SetTradingCalendarProviders([]provider.TradingCalendarProvider{providerProbeFreshnessCalendarFake{}})
	tests := []struct {
		capability string
		domain     string
	}{
		{capability: "sector_rankings", domain: freshness.DomainSector},
		{capability: "market_capital_flow", domain: freshness.DomainFundFlow},
	}
	for _, tc := range tests {
		t.Run(tc.capability, func(t *testing.T) {
			run := &model.ProviderProbeRun{
				FinishedAt: checkedAt,
				Items: []model.ProviderProbeItem{{
					Provider: "date-only-provider", Capability: tc.capability, FetchedAt: &fetchedAt,
				}},
				CheckResults: []model.ProviderProbeCheckResult{{
					Check: "freshness", Provider: "date-only-provider", Capability: tc.capability,
					FetchedAt: &fetchedAt, CheckedAt: checkedAt,
				}},
			}
			ds.applyFreshnessProbeResults(context.Background(), run)
			result := run.CheckResults[0]
			if result.Status != "skipped" || result.FreshnessStatus != "unknown" ||
				result.FreshnessReason != freshness.ReasonMissingDataTime || result.FreshnessDomain != tc.domain {
				t.Fatalf("business date or fetched_at was incorrectly treated as source time: %+v", result)
			}
			if result.AgeSeconds != nil {
				t.Fatalf("unknown source time must not produce an age: %+v", result)
			}
		})
	}
}
