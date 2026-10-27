undefined

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
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "stock_search_fallback_exhausted_using_stale_directory" {
			found = true
		}
	}
	if !found || first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing stale fallback marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls)
	}
}
