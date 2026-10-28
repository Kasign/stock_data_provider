package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type capitalFlowFallbackTestProvider struct {
	name  string
	item  *model.CapitalFlow
	err   error
	calls int
}

func (p *capitalFlowFallbackTestProvider) Name() string { return p.name }

func (p *capitalFlowFallbackTestProvider) FetchCapitalFlow(context.Context, string) (*model.CapitalFlow, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.item == nil {
		return nil, nil
	}
	out := *p.item
	return &out, nil
}

func capitalFlowServiceForFallbackTest(providers ...provider.CapitalFlowProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetCapitalFlowProviders(providers)
	return svc
}

func capitalFlowFallbackItemForTest(source string, quality model.QualityStatus) *model.CapitalFlow {
	main := 120.0
	five := 500.0
	ten := 900.0
	return &model.CapitalFlow{
		Symbol: "600519.SH",
		ProviderDate: time.Now().In(marketLocation()).AddDate(0, 0, -1).Format("2006-01-02"),
		MainNetInflow: &main, Inflow5D: &five, Inflow10D: &ten, Observations: 10,
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: quality, QualityReasons: []string{"test_quality"},
	}
}

func TestCapitalFlowFallsBackWhenFirstProviderHasNoUsableMetrics(t *testing.T) {
	now := time.Now().UTC()
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	empty := &capitalFlowFallbackTestProvider{
		name: "eastmoney",
		item: &model.CapitalFlow{
			Symbol: "600519.SH", ProviderDate: today, Observations: 10,
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_amount_metrics_missing"},
		},
	}
	fallback := &capitalFlowFallbackTestProvider{
		name: "miaoxiang", item: capitalFlowFallbackItemForTest("miaoxiang", model.QualityDegraded),
	}
	svc := capitalFlowServiceForFallbackTest(empty, fallback)

	got, cacheHit, err := svc.CapitalFlow(context.Background(), "600519.SH")
	if err != nil || cacheHit {
		t.Fatalf("CapitalFlow returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.Source != "miaoxiang" || got.MainNetInflow == nil || *got.MainNetInflow != 120 {
		t.Fatalf("expected usable fallback capital-flow metrics, got %+v", got)
	}
	if empty.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both Providers to be tried, eastmoney=%d miaoxiang=%d", empty.calls, fallback.calls)
	}
}

func TestCapitalFlowFallsBackFromUnknownQualitySnapshot(t *testing.T) {
	unknown := &capitalFlowFallbackTestProvider{
		name: "eastmoney", item: capitalFlowFallbackItemForTest("eastmoney", model.QualityUnknown),
	}
	fallback := &capitalFlowFallbackTestProvider{
		name: "miaoxiang", item: capitalFlowFallbackItemForTest("miaoxiang", model.QualityDegraded),
	}
	svc := capitalFlowServiceForFallbackTest(unknown, fallback)

	got, cacheHit, err := svc.CapitalFlow(context.Background(), "600519.SH")
	if err != nil || cacheHit {
		t.Fatalf("CapitalFlow returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.Source != "miaoxiang" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected a degraded fallback snapshot before unknown metrics, got %+v", got)
	}
	if unknown.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both capital-flow providers to be tried, unknown=%d fallback=%d", unknown.calls, fallback.calls)
	}
}

func TestCapitalFlowKeepsUnknownCandidateWithMoreMetrics(t *testing.T) {
	partial := capitalFlowFallbackItemForTest("eastmoney", model.QualityUnknown)
	partial.Inflow5D = nil
	partial.Inflow10D = nil
	partial.Observations = 1
	full := capitalFlowFallbackItemForTest("miaoxiang", model.QualityUnknown)
	first := &capitalFlowFallbackTestProvider{name: "eastmoney", item: partial}
	second := &capitalFlowFallbackTestProvider{name: "miaoxiang", item: full}
	failed := &capitalFlowFallbackTestProvider{name: "third-source", err: errors.New("test source unavailable")}
	svc := capitalFlowServiceForFallbackTest(first, second, failed)

	got, hit, err := svc.CapitalFlow(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("CapitalFlow returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "miaoxiang" || got.Inflow5D == nil || got.Inflow10D == nil ||
		got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected the unknown candidate with more metrics, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected all Providers to be tried, calls=%d/%d/%d", first.calls, second.calls, failed.calls)
	}
}

func TestCapitalFlowReturnsUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	unknown := &capitalFlowFallbackTestProvider{
		name: "eastmoney", item: capitalFlowFallbackItemForTest("eastmoney", model.QualityUnknown),
	}
	failed := &capitalFlowFallbackTestProvider{name: "miaoxiang", err: errors.New("test source unavailable")}
	svc := capitalFlowServiceForFallbackTest(unknown, failed)

	got, cacheHit, err := svc.CapitalFlow(context.Background(), "600519.SH")
	if err != nil || cacheHit {
		t.Fatalf("CapitalFlow returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected an unknown snapshot after fallback exhaustion, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "capital_flow_fallback_exhausted_without_usable_quality" {
			found = true
			break
		}
	}
	if !found || unknown.calls != 1 || failed.calls != 1 {
		t.Fatalf("missing fallback marker or source was not attempted: reasons=%v unknown=%d failed=%d", got.QualityReasons, unknown.calls, failed.calls)
	}
}

func TestCapitalFlowLegacyUnknownCacheDoesNotBlockFallback(t *testing.T) {
	legacy := capitalFlowFallbackItemForTest("eastmoney", model.QualityUnknown)
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v1:capital-flow:600519.SH"] = encoded
	first := &capitalFlowFallbackTestProvider{name: "eastmoney", item: legacy}
	fallback := &capitalFlowFallbackTestProvider{
		name: "miaoxiang", item: capitalFlowFallbackItemForTest("miaoxiang", model.QualityDegraded),
	}
	svc := NewWithCapabilities(nil, nil, nil, mem, nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetCapitalFlowProviders([]provider.CapitalFlowProvider{first, fallback})

	got, hit, err := svc.CapitalFlow(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("legacy unknown cache must not short-circuit fallback: hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "miaoxiang" || fallback.calls != 1 {
		t.Fatalf("expected fallback provider after bypassing unknown cache, got %+v calls=%d", got, fallback.calls)
	}
}

func TestCapitalFlowProvidersDeduplicatesNames(t *testing.T) {
	first := &capitalFlowFallbackTestProvider{name: "eastmoney", item: capitalFlowFallbackItemForTest("eastmoney", model.QualityDegraded)}
	duplicate := &capitalFlowFallbackTestProvider{name: "EASTMONEY", item: capitalFlowFallbackItemForTest("eastmoney", model.QualityDegraded)}
	svc := capitalFlowServiceForFallbackTest(first, duplicate)

	got := svc.CapitalFlowProviders()
	if len(got) != 1 {
		t.Fatalf("expected providers with duplicate names to be deduplicated, got %d", len(got))
	}
}
