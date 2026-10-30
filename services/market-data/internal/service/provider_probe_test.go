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
	if run.Total != 1 || run.Returned != 1 || run.Failed != 0 || run.Items[0].Status != "returned" || run.Persisted {
		t.Fatalf("unexpected probe result: %+v", run)
	}
	if run.Items[0].Capability != "quote" || run.Items[0].Provider != "probe-test" {
		t.Fatalf("unexpected probe identity: %+v", run.Items[0])
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
	if run.Failed != 1 || len(run.Items) != 1 {
		t.Fatalf("expected one failed probe item, got %+v", run)
	}
	item := run.Items[0]
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
