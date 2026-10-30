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
		Items:  []model.StockSearchItem{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_directory_timestamp_not_exposed"},
	}
}

func TestStockSearchRejectsMissingSourceVersion(t *testing.T) {
	snapshot := populatedStockSearchForFallbackTest("eastmoney")
	snapshot.SourceVersion = ""
	if err := validateStockSearchSnapshot("贵州茅台", 10, snapshot); err == nil {
		t.Fatal("stock-search snapshot without source version must be rejected")
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

func unknownStockSearchWithMatchesForFallbackTest(source string) *model.StockSearchSnapshot {
	snapshot := populatedStockSearchForFallbackTest(source)
	snapshot.QualityStatus = model.QualityUnknown
	snapshot.QualityReasons = []string{"provider_directory_quality_unknown"}
	return snapshot
}

func TestStockSearchFallsBackFromUnknownQualityMatches(t *testing.T) {
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: unknownStockSearchWithMatchesForFallbackTest("eastmoney")}
	second := &stockSearchFallbackTestProvider{name: "tencent", snapshot: populatedStockSearchForFallbackTest("tencent")}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got == nil || got.Source != "tencent" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected confirmed-quality fallback match, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both sources to be tried once, first=%d second=%d", first.calls, second.calls)
	}
}

func TestStockSearchReturnsUnknownMatchesOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: unknownStockSearchWithMatchesForFallbackTest("eastmoney")}
	second := &stockSearchFallbackTestProvider{name: "tencent", err: errors.New("test provider unavailable")}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got == nil || len(got.Items) != 1 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected the unknown match to remain as last-resort data, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "stock_search_fallback_exhausted_with_unconfirmed_quality" {
			found = true
			break
		}
	}
	if !found || first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing fallback-exhaustion reason or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls)
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

func TestStockSearchKeepsBestCoverageStaleCandidate(t *testing.T) {
	now := time.Now().UTC()
	more := populatedStockSearchForFallbackTest("eastmoney")
	more.QualityStatus = model.QualityStale
	more.FetchedAt = now.Add(-time.Minute)
	more.Items = append(more.Items, model.StockSearchItem{
		Symbol: "000858.SZ", Code: "000858", Name: "五粮液", Exchange: "SZ",
	})
	more.Total = len(more.Items)

	less := populatedStockSearchForFallbackTest("tencent")
	less.QualityStatus = model.QualityStale
	less.FetchedAt = now
	failed := &stockSearchFallbackTestProvider{name: "third-source", err: errors.New("test search endpoint unavailable")}
	first := &stockSearchFallbackTestProvider{name: "eastmoney", snapshot: more}
	second := &stockSearchFallbackTestProvider{name: "tencent", snapshot: less}
	svc := &DataService{stockSearchProviders: []provider.StockSearchProvider{first, second, failed}}

	got, err := svc.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("stale matches should remain available as last resort: %v", err)
	}
	if got == nil || got.Source != "eastmoney" || got.QualityStatus != model.QualityStale || len(got.Items) != 2 {
		t.Fatalf("expected the stale candidate with the better match coverage, got %+v", got)
	}
	if !hasStockSearchReason(got.QualityReasons, "stock_search_fallback_exhausted_using_stale_directory") ||
		first.calls != 1 || second.calls != 1 || failed.calls != 1 {
		t.Fatalf("missing stale fallback marker or incomplete fallback: reasons=%v calls=%d/%d/%d", got.QualityReasons, first.calls, second.calls, failed.calls)
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
