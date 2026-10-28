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

type fundamentalsFallbackTestProvider struct {
	name     string
	snapshot *model.FundamentalsSnapshot
	err      error
	calls    int
}

func (p *fundamentalsFallbackTestProvider) Name() string { return p.name }

func (p *fundamentalsFallbackTestProvider) FetchFundamentals(_ context.Context, symbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test fundamentals snapshot is nil")
	}
	out := *p.snapshot
	out.Symbol = symbol
	out.Year = year
	out.Quarter = quarter
	out.Sections = append([]model.FundamentalSection(nil), p.snapshot.Sections...)
	for index := range out.Sections {
		out.Sections[index].Metrics = append([]model.FundamentalMetric(nil), p.snapshot.Sections[index].Metrics...)
	}
	return &out, nil
}

func fundamentalsFallbackService(providers ...provider.FundamentalsProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetFundamentalsProviders(providers)
	return svc
}

func unknownFundamentalsFallbackSnapshot(source string) *model.FundamentalsSnapshot {
	item := validFundamentalsSnapshotForTest()
	item.Source = source
	setFundamentalsUnknownQuality(item)
	return item
}

func TestFundamentalsFallsBackFromUnknownQualityMetricsToUsableProvider(t *testing.T) {
	unknown := &fundamentalsFallbackTestProvider{
		name: "baostock", snapshot: unknownFundamentalsFallbackSnapshot("baostock"),
	}
	usableSnapshot := validFundamentalsSnapshotForTest()
	usableSnapshot.Source = "tushare"
	usable := &fundamentalsFallbackTestProvider{name: "tushare", snapshot: usableSnapshot}
	svc := fundamentalsFallbackService(unknown, usable)

	got, cacheHit, err := svc.Fundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil || cacheHit {
		t.Fatalf("Fundamentals returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityValid {
		t.Fatalf("expected point-in-time usable provider after unknown-quality metrics, got %+v", got)
	}
	if unknown.calls != 1 || usable.calls != 1 {
		t.Fatalf("expected both providers to be tried, baostock=%d tushare=%d", unknown.calls, usable.calls)
	}
}

func TestFundamentalsKeepsUnknownCandidateWithMoreMetricCoverage(t *testing.T) {
	more := unknownFundamentalsFallbackSnapshot("baostock")
	less := unknownFundamentalsFallbackSnapshot("tushare")
	for index := range less.Sections {
		if index >= 2 {
			less.Sections[index].HasData = false
			less.Sections[index].Metrics = []model.FundamentalMetric{}
			less.Sections[index].QualityReasons = []string{"test_section_metrics_unavailable"}
		}
	}
	less.HasData = true
	failed := &fundamentalsFallbackTestProvider{name: "third-source", err: errors.New("test source unavailable")}
	first := &fundamentalsFallbackTestProvider{name: "baostock", snapshot: more}
	second := &fundamentalsFallbackTestProvider{name: "tushare", snapshot: less}
	svc := fundamentalsFallbackService(first, second, failed)

	got, hit, err := svc.Fundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil || hit {
		t.Fatalf("Fundamentals returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "baostock" || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected the fuller unknown fundamentals snapshot, got %+v", got)
	}
	metricCount := 0
	for _, section := range got.Sections {
		metricCount += len(section.Metrics)
	}
	if metricCount != 6 || first.calls != 1 || second.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected six metrics from the best candidate and all Providers tried, metrics=%d calls=%d/%d/%d", metricCount, first.calls, second.calls, failed.calls)
	}
}

func TestFundamentalsUnknownQualityMetricsAreReturnedOnlyAfterFallbackExhaustion(t *testing.T) {
	unknown := &fundamentalsFallbackTestProvider{
		name: "baostock", snapshot: unknownFundamentalsFallbackSnapshot("baostock"),
	}
	failed := &fundamentalsFallbackTestProvider{name: "tushare", err: errors.New("test endpoint unavailable")}
	svc := fundamentalsFallbackService(unknown, failed)

	got, cacheHit, err := svc.Fundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil || cacheHit {
		t.Fatalf("Fundamentals returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.QualityStatus != model.QualityUnknown || !got.HasData {
		t.Fatalf("expected unknown-quality metrics only after fallback exhaustion, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "fundamentals_fallback_exhausted_without_usable_quality" {
			found = true
		}
	}
	if !found || unknown.calls != 1 || failed.calls != 1 {
		t.Fatalf("missing fallback exhaustion marker or incomplete provider fallback: reasons=%v unknown=%d failed=%d", got.QualityReasons, unknown.calls, failed.calls)
	}
}

func TestFundamentalsOldUnknownMetricsCacheDoesNotBlockProviderFallback(t *testing.T) {
	legacy := unknownFundamentalsFallbackSnapshot("baostock")
	legacy.FetchedAt = time.Now().UTC()
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v1:fundamentals:600519.SH:2026:2"] = encoded
	usableSnapshot := validFundamentalsSnapshotForTest()
	usableSnapshot.Source = "tushare"
	usable := &fundamentalsFallbackTestProvider{name: "tushare", snapshot: usableSnapshot}
	other := &fundamentalsFallbackTestProvider{name: "baostock", snapshot: legacy}
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetFundamentalsProviders([]provider.FundamentalsProvider{other, usable})

	got, cacheHit, err := svc.Fundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil || cacheHit {
		t.Fatalf("legacy unknown metrics cache must not short-circuit fallback: hit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityValid || usable.calls != 1 {
		t.Fatalf("expected usable provider after bypassing old unknown cache: %+v calls=%d", got, usable.calls)
	}
}
