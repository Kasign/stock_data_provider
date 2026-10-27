package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type stockSearchFallbackTestProvider struct {
	name     string
	snapshot *model.StockSearchSnapshot
	err      error
	calls    int
}

func (p *stockSearchFallbackTestProvider) Name() string { return p.name }

func (p *stockSearchFallbackTestProvider) SearchStocks(_ context.Context, query string, _ int) (*model.StockSearchSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test stock-search snapshot is nil")
	}
	out := *p.snapshot
	out.Query = query
	out.Items = append([]model.StockSearchItem(nil), p.snapshot.Items...)
	return &out, nil
}

func unknownStockSearchForFallbackTest(source string) *model.StockSearchSnapshot {
	return &model.StockSearchSnapshot{
		Query: "贵州茅台", Total: 0, Items: []model.StockSearchItem{},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_directory_not_confirmed"},
	}
}

func populatedStockSearchForFallbackTest(source string) *model.StockSearchSnapshot {
	now := time.Now().UTC()
	return &model.StockSearchSnapshot{
		Query: "贵州茅台", Total: 1,
		Items: []model.StockSearchItem{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_directory_timestamp_not_exposed"},
	}
}

func TestStockSearchFallsBackAfterUnknownEmptyMatch(t *testing.T) {
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: unknownStockSearchForFallbackTest("eastmoney")}
	second := &stockSearchFallbackTestProvider{name: "tencent", snapshot: populatedStockSearchForFallbackTest("tencent")}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got == nil || len(got.Items) != 1 || got.Items[0].Symbol != "600519.SH" || got.Source != "tencent" {
		t.Fatalf("expected matching result from fallback provider, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both search providers to be tried, first=%d second=%d", first.calls, second.calls)
	}
}

func TestStockSearchReturnsUnknownOnlyAfterAllProvidersExhausted(t *testing.T) {
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: unknownStockSearchForFallbackTest("eastmoney")}
	second := &stockSearchFallbackTestProvider{name: "tencent", err: errors.New("test search endpoint unavailable")}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("unknown empty match should be explicit after fallback exhaustion: %v", err)
	}
	if got == nil || len(got.Items) != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown empty match, got %+v", got)
	}
	if !hasStockSearchReason(got.QualityReasons, "stock_search_fallback_exhausted_without_confirmed_match") ||
		first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing fallback-exhaustion marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls)
	}
}

func TestStockSearchPrefersLaterNonStaleProvider(t *testing.T) {
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: populatedStockSearchForFallbackTest("eastmoney")}
	first.snapshot.QualityStatus = model.QualityStale
	second := &stockSearchFallbackTestProvider{name: "tencent", snapshot: populatedStockSearchForFallbackTest("tencent")}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got == nil || got.Source != "tencent" || got.QualityStatus == model.QualityStale {
		t.Fatalf("fresh fallback result should be preferred over stale directory, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both search providers to be tried, first=%d second=%d", first.calls, second.calls)
	}
}

func TestStockSearchReturnsStaleMatchesOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: populatedStockSearchForFallbackTest("eastmoney")}
	first.snapshot.QualityStatus = model.QualityStale
	second := &stockSearchFallbackTestProvider{name: "tencent", err: errors.New("test search endpoint unavailable")}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("stale matches should remain available as last resort: %v", err)
	}
	if got == nil || got.QualityStatus != model.QualityStale {
		t.Fatalf("expected stale matches to remain explicitly stale, got %+v", got)
	}
	if !hasStockSearchReason(got.QualityReasons, "stock_search_fallback_exhausted_using_stale_directory") ||
		first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing stale fallback marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls)
	}
}

func hasStockSearchReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
