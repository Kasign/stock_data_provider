package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type qualityFallbackIndexQuoteProvider struct {
	name     string
	snapshot *model.IndexQuoteSnapshot
	err      error
	calls    int
}

func (p *qualityFallbackIndexQuoteProvider) Name() string { return p.name }

func (p *qualityFallbackIndexQuoteProvider) FetchIndexQuotes(context.Context) (*model.IndexQuoteSnapshot, error) {
	p.calls++
	return p.snapshot, p.err
}

type qualityFallbackMarketBreadthProvider struct {
	name     string
	snapshot *model.MarketBreadthSnapshot
	err      error
	calls    int
}

func (p *qualityFallbackMarketBreadthProvider) Name() string { return p.name }

func (p *qualityFallbackMarketBreadthProvider) FetchMarketBreadth(context.Context) (*model.MarketBreadthSnapshot, error) {
	p.calls++
	return p.snapshot, p.err
}

func indexQuoteSnapshotForFallback(source string, quality model.QualityStatus) *model.IndexQuoteSnapshot {
	now := time.Now().UTC()
	price, change := 3200.0, 1.0
	item := model.MarketIndexQuote{
		Symbol: "000001.SH", Code: "000001", Name: "上证指数",
		LatestPrice: &price, ChangePercent: &change, AmountUnit: "unknown",
		Source: source, SourceVersion: "test-v1", FetchedAt: now, QualityStatus: quality,
	}
	if quality == model.QualityValid {
		item.QuoteTime = &now
		item.QuoteTimeSemantics = "provider_snapshot_time"
	} else {
		item.QualityReasons = []string{"provider_quote_timestamp_missing_or_invalid"}
	}
	return &model.IndexQuoteSnapshot{
		Items: []model.MarketIndexQuote{item}, Source: source, SourceVersion: "test-v1",
		FetchedAt: now, QualityStatus: quality,
	}
}

func marketBreadthSnapshotForFallback(source string, quality model.QualityStatus) *model.MarketBreadthSnapshot {
	now := time.Now().UTC()
	reportDate := now.In(marketLocation()).Format("2006-01-02")
	snapshot := &model.MarketBreadthSnapshot{
		Market: "cn_a_share", TotalSecurities: 3, CoveredSecurities: 3,
		AdvancingCount: 1, DecliningCount: 1, UnchangedCount: 1,
		Source: source, SourceVersion: "test-v1", ProviderReportDate: reportDate,
		FetchedAt: now, QualityStatus: quality,
	}
	if quality == model.QualityValid {
		amount := 100.0
		snapshot.TotalAmountCNY = &amount
		snapshot.AmountCoverageCount = 3
		snapshot.QuoteTimestampCoverageCount = 3
		snapshot.OldestQuoteTime = &now
		snapshot.NewestQuoteTime = &now
	} else {
		snapshot.AmountCoverageCount = 2
		snapshot.QuoteTimestampCoverageCount = 2
		snapshot.QualityReasons = []string{"market_amount_quotes_collected_non_atomically"}
	}
	return snapshot
}

func TestIndexQuotesFallsBackFromDegradedToValidProvider(t *testing.T) {
	first := &qualityFallbackIndexQuoteProvider{name: "eastmoney", snapshot: indexQuoteSnapshotForFallback("eastmoney", model.QualityDegraded)}
	second := &qualityFallbackIndexQuoteProvider{name: "tickflow", snapshot: indexQuoteSnapshotForFallback("tickflow", model.QualityValid)}
	svc := &DataService{cache: cache.NewDisabled(), indexQuoteProviders: []provider.IndexQuoteProvider{first, second}}

	got, hit, err := svc.IndexQuotes(context.Background())
	if err != nil || hit {
		t.Fatalf("IndexQuotes returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tickflow" || got.QualityStatus != model.QualityDegraded || !hasSnapshotQualityReason(got.QualityReasons, "provider_fallback_used") {
		t.Fatalf("expected valid fallback index quote, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both providers to be tried, calls=%d/%d", first.calls, second.calls)
	}
}

func TestIndexQuotesKeepsPriorityDegradedCandidateWhenNoValidProvider(t *testing.T) {
	first := &qualityFallbackIndexQuoteProvider{name: "eastmoney", snapshot: indexQuoteSnapshotForFallback("eastmoney", model.QualityDegraded)}
	second := &qualityFallbackIndexQuoteProvider{name: "tickflow", err: errors.New("upstream unavailable")}
	svc := &DataService{cache: cache.NewDisabled(), indexQuoteProviders: []provider.IndexQuoteProvider{first, second}}

	got, hit, err := svc.IndexQuotes(context.Background())
	if err != nil || hit {
		t.Fatalf("IndexQuotes returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "eastmoney" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected first-priority degraded candidate, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both providers to be tried, calls=%d/%d", first.calls, second.calls)
	}
}

func TestMarketBreadthFallsBackFromDegradedToValidProvider(t *testing.T) {
	first := &qualityFallbackMarketBreadthProvider{name: "eastmoney", snapshot: marketBreadthSnapshotForFallback("eastmoney", model.QualityDegraded)}
	second := &qualityFallbackMarketBreadthProvider{name: "tickflow", snapshot: marketBreadthSnapshotForFallback("tickflow", model.QualityValid)}
	svc := &DataService{cache: cache.NewDisabled(), marketBreadthProviders: []provider.MarketBreadthProvider{first, second}}

	got, hit, err := svc.MarketBreadth(context.Background())
	if err != nil || hit {
		t.Fatalf("MarketBreadth returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tickflow" || got.QualityStatus != model.QualityDegraded || !hasSnapshotQualityReason(got.QualityReasons, "provider_fallback_used") {
		t.Fatalf("expected valid fallback market breadth, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both providers to be tried, calls=%d/%d", first.calls, second.calls)
	}
}

func TestMarketBreadthKeepsPriorityDegradedCandidateWhenNoValidProvider(t *testing.T) {
	first := &qualityFallbackMarketBreadthProvider{name: "eastmoney", snapshot: marketBreadthSnapshotForFallback("eastmoney", model.QualityDegraded)}
	second := &qualityFallbackMarketBreadthProvider{name: "tickflow", err: errors.New("upstream unavailable")}
	svc := &DataService{cache: cache.NewDisabled(), marketBreadthProviders: []provider.MarketBreadthProvider{first, second}}

	got, hit, err := svc.MarketBreadth(context.Background())
	if err != nil || hit {
		t.Fatalf("MarketBreadth returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "eastmoney" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected first-priority degraded candidate, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both providers to be tried, calls=%d/%d", first.calls, second.calls)
	}
}

func TestIndexQuotesMarksDegradedFallbackWhenPrimaryFails(t *testing.T) {
	first := &qualityFallbackIndexQuoteProvider{name: "eastmoney", err: errors.New("primary unavailable")}
	second := &qualityFallbackIndexQuoteProvider{name: "tickflow", snapshot: indexQuoteSnapshotForFallback("tickflow", model.QualityDegraded)}
	svc := &DataService{cache: cache.NewDisabled(), indexQuoteProviders: []provider.IndexQuoteProvider{first, second}}

	got, hit, err := svc.IndexQuotes(context.Background())
	if err != nil || hit {
		t.Fatalf("IndexQuotes returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tickflow" || got.QualityStatus != model.QualityDegraded ||
		!hasSnapshotQualityReason(got.QualityReasons, "provider_fallback_used") ||
		!hasSnapshotQualityReason(got.Items[0].QualityReasons, "provider_fallback_used") {
		t.Fatalf("degraded fallback must preserve fallback provenance: %+v", got)
	}
}

func TestMarketBreadthMarksDegradedFallbackWhenPrimaryFails(t *testing.T) {
	first := &qualityFallbackMarketBreadthProvider{name: "eastmoney", err: errors.New("primary unavailable")}
	second := &qualityFallbackMarketBreadthProvider{name: "tickflow", snapshot: marketBreadthSnapshotForFallback("tickflow", model.QualityDegraded)}
	svc := &DataService{cache: cache.NewDisabled(), marketBreadthProviders: []provider.MarketBreadthProvider{first, second}}

	got, hit, err := svc.MarketBreadth(context.Background())
	if err != nil || hit {
		t.Fatalf("MarketBreadth returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tickflow" || got.QualityStatus != model.QualityDegraded ||
		!hasSnapshotQualityReason(got.QualityReasons, "provider_fallback_used") {
		t.Fatalf("degraded fallback must preserve fallback provenance: %+v", got)
	}
}

