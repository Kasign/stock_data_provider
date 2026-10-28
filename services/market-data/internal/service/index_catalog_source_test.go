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
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type indexCatalogSourceTestProvider struct {
	name string
}

func (p indexCatalogSourceTestProvider) Name() string { return p.name }

func (p indexCatalogSourceTestProvider) FetchIndexCatalog(context.Context, string) (*model.IndexCatalogSnapshot, error) {
	return nil, nil
}

func (p indexCatalogSourceTestProvider) FetchIndexConstituents(context.Context, string) (*model.IndexConstituentSnapshot, error) {
	return nil, nil
}

func (p indexCatalogSourceTestProvider) FetchIndexDailyBars(context.Context, string, string, string) (*model.IndexDailyBarsSnapshot, error) {
	return nil, nil
}

func TestIndexCatalogProvidersForSourcePinsRequestedProvider(t *testing.T) {
	hithink := indexCatalogSourceTestProvider{name: "hithink"}
	tushare := indexCatalogSourceTestProvider{name: "tushare"}
	svc := &DataService{indexCatalogProviders: []provider.IndexCatalogProvider{hithink, tushare}}

	selected, cacheKey, err := svc.indexCatalogProvidersForSource("TUSHARE")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Name() != "tushare" || cacheKey != "tushare" {
		t.Fatalf("unexpected selected provider/cache key: providers=%v key=%q", selected, cacheKey)
	}
}

func TestIndexCatalogProvidersForSourceKeepsAutoFallbackByDefault(t *testing.T) {
	svc := &DataService{indexCatalogProviders: []provider.IndexCatalogProvider{
		indexCatalogSourceTestProvider{name: "hithink"},
		indexCatalogSourceTestProvider{name: "tushare"},
	}}
	selected, cacheKey, err := svc.indexCatalogProvidersForSource("")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Name() != "hithink" || selected[1].Name() != "tushare" || cacheKey != "auto" {
		t.Fatalf("unexpected auto provider chain/cache key: count=%d key=%q", len(selected), cacheKey)
	}
}

func TestIndexCatalogProvidersForSourceRejectsUnavailableProvider(t *testing.T) {
	svc := &DataService{indexCatalogProviders: []provider.IndexCatalogProvider{indexCatalogSourceTestProvider{name: "hithink"}}}
	if _, _, err := svc.indexCatalogProvidersForSource("tushare"); err == nil {
		t.Fatal("expected a disabled/unregistered source to be rejected")
	}
}



func TestIndexSnapshotContractsRequireSourceVersion(t *testing.T) {
	catalog := indexCatalogDegradedDataForTest("hithink", "tushare")
	catalog.SourceVersion = ""
	if err := validateIndexCatalogSnapshot("tushare", catalog); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("index catalog without source version must be rejected, got %v", err)
	}

	constituents := indexConstituentsDegradedDataForTest("hithink")
	constituents.SourceVersion = ""
	if err := validateIndexConstituentsSnapshot("000300.SH", constituents); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("index constituents without source version must be rejected, got %v", err)
	}

	daily := populatedIndexDailyBarsForFallbackTest("tushare")
	daily.SourceVersion = ""
	if err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-20", "2026-10-23", daily); err == nil || !strings.Contains(err.Error(), "source_version") {
		t.Fatalf("index daily bars without source version must be rejected, got %v", err)
	}
}

func TestIndexDailyBarsRejectsMismatchedItemSourceVersion(t *testing.T) {
	snapshot := populatedIndexDailyBarsForFallbackTest("tushare")
	snapshot.Items[0].SourceVersion = "older-version"
	if err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-20", "2026-10-23", snapshot); err == nil || !strings.Contains(err.Error(), "source/version") {
		t.Fatalf("index daily bar version mismatch must be rejected, got %v", err)
	}
}

func TestValidateIndexDailyBarsRejectsValidSnapshotWithoutProviderTime(t *testing.T) {
	snapshot := populatedIndexDailyBarsForFallbackTest("tushare")
	snapshot.QualityStatus = model.QualityValid
	snapshot.Items[0].QualityStatus = model.QualityValid
	snapshot.Items[0].QualityReasons = nil

	err := validateIndexDailyBarsSnapshot("000300.SH", "2026-10-20", "2026-10-23", snapshot)
	if err == nil || !strings.Contains(err.Error(), "without a provider snapshot timestamp") {
		t.Fatalf("index daily-bars snapshot without provider time must not be valid, got %v", err)
	}
}

func TestIndexDailyBarsRejectsFutureEndDateBeforeProviderSelection(t *testing.T) {
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	future := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	svc := &DataService{}
	if _, _, err := svc.IndexDailyBarsFromSource(context.Background(), "000300.SH", today, future, ""); err == nil || !strings.Contains(err.Error(), "must not be in the future") {
		t.Fatalf("future index daily-bars end date must be rejected before selecting a Provider, got %v", err)
	}
}


type indexDailyBarsFallbackTestProvider struct {
	name string
	snapshot *model.IndexDailyBarsSnapshot
	err error
	calls int
}

func (p *indexDailyBarsFallbackTestProvider) Name() string { return p.name }

func (p *indexDailyBarsFallbackTestProvider) FetchIndexCatalog(context.Context, string) (*model.IndexCatalogSnapshot, error) {
	return nil, nil
}

func (p *indexDailyBarsFallbackTestProvider) FetchIndexConstituents(context.Context, string) (*model.IndexConstituentSnapshot, error) {
	return nil, nil
}

func (p *indexDailyBarsFallbackTestProvider) FetchIndexDailyBars(_ context.Context, indexSymbol, startDate, endDate string) (*model.IndexDailyBarsSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, nil
	}
	out := *p.snapshot
	out.IndexSymbol = indexSymbol
	out.StartDate = startDate
	out.EndDate = endDate
	out.Items = append([]model.IndexDailyBar(nil), p.snapshot.Items...)
	return &out, nil
}

func unknownIndexDailyBarsForFallbackTest(source string) *model.IndexDailyBarsSnapshot {
	return &model.IndexDailyBarsSnapshot{
		IndexSymbol: "000300.SH", StartDate: "2026-10-20", EndDate: "2026-10-23",
		Items: []model.IndexDailyBar{}, Total: 0, Source: source, SourceVersion: "test-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown,
		QualityReasons: []string{"provider_index_daily_bars_empty"},
	}
}

func populatedIndexDailyBarsForFallbackTest(source string) *model.IndexDailyBarsSnapshot {
	now := time.Now().UTC()
	volume := int64(1000000)
	amount := 100000000.0
	change := 1.2
	return &model.IndexDailyBarsSnapshot{
		IndexSymbol: "000300.SH", StartDate: "2026-10-20", EndDate: "2026-10-23",
		Items: []model.IndexDailyBar{{
			IndexSymbol: "000300.SH", TradeDate: "2026-10-23",
			Open: 3900, High: 3950, Low: 3880, Close: 3940,
			ChangePercent: &change, Volume: &volume, Amount: &amount,
			VolumeUnit: "shares", AmountUnit: "CNY", Source: source, SourceVersion: "test-v1",
			FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		}},
		Total: 1, Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
}

func TestIndexDailyBarsFallsBackAfterUnknownEmptyProvider(t *testing.T) {
	first := &indexDailyBarsFallbackTestProvider{name: "hithink", snapshot: unknownIndexDailyBarsForFallbackTest("hithink")}
	second := &indexDailyBarsFallbackTestProvider{name: "tushare", snapshot: populatedIndexDailyBarsForFallbackTest("tushare")}
	svc := &DataService{
		cache: cache.NewDisabled(),
		indexCatalogProviders: []provider.IndexCatalogProvider{first, second},
	}
	got, hit, err := svc.IndexDailyBarsFromSource(context.Background(), "000300.SH", "2026-10-20", "2026-10-23", "")
	if err != nil || hit {
		t.Fatalf("IndexDailyBarsFromSource returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || len(got.Items) != 1 {
		t.Fatalf("expected index daily-bars fallback data, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both Providers to be called, first=%d second=%d", first.calls, second.calls)
	}
}

func unknownQualityIndexDailyBarsForFallbackTest(source string) *model.IndexDailyBarsSnapshot {
	snapshot := populatedIndexDailyBarsForFallbackTest(source)
	snapshot.QualityStatus = model.QualityUnknown
	snapshot.QualityReasons = []string{"provider_index_daily_bars_quality_unknown"}
	return snapshot
}

func TestIndexDailyBarsFallsBackFromUnknownQualityRows(t *testing.T) {
	first := &indexDailyBarsFallbackTestProvider{name: "hithink", snapshot: unknownQualityIndexDailyBarsForFallbackTest("hithink")}
	second := &indexDailyBarsFallbackTestProvider{name: "tushare", snapshot: populatedIndexDailyBarsForFallbackTest("tushare")}
	svc := &DataService{
		cache: cache.NewDisabled(),
		indexCatalogProviders: []provider.IndexCatalogProvider{first, second},
	}

	got, hit, err := svc.IndexDailyBarsFromSource(context.Background(), "000300.SH", "2026-10-20", "2026-10-23", "")
	if err != nil || hit {
		t.Fatalf("IndexDailyBarsFromSource returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityDegraded || len(got.Items) != 1 {
		t.Fatalf("expected degraded bars from the later provider, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected fallback after unknown-quality rows, first=%d second=%d", first.calls, second.calls)
	}
}

func TestIndexDailyBarsLegacyUnknownQualityCacheDoesNotBlockFallback(t *testing.T) {
	legacy := unknownQualityIndexDailyBarsForFallbackTest("hithink")
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v2:index-daily-bars:auto:000300.SH:2026-10-20:2026-10-23"] = encoded
	first := &indexDailyBarsFallbackTestProvider{name: "hithink", snapshot: legacy}
	second := &indexDailyBarsFallbackTestProvider{name: "tushare", snapshot: populatedIndexDailyBarsForFallbackTest("tushare")}
	svc := &DataService{
		cache: mem,
		indexCatalogProviders: []provider.IndexCatalogProvider{first, second},
	}

	got, hit, err := svc.IndexDailyBarsFromSource(context.Background(), "000300.SH", "2026-10-20", "2026-10-23", "")
	if err != nil || hit {
		t.Fatalf("legacy unknown-quality cache must not short-circuit fallback: hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || second.calls != 1 {
		t.Fatalf("expected fallback provider after bypassing legacy cache, got %+v secondCalls=%d", got, second.calls)
	}
}

func TestIndexDailyBarsKeepsUnknownCandidateWithMoreFieldCoverage(t *testing.T) {
	sparse := populatedIndexDailyBarsForFallbackTest("hithink")
	sparse.QualityStatus = model.QualityUnknown
	sparse.QualityReasons = []string{"provider_index_quality_unknown"}
	sparse.Items[0].ChangePercent = nil
	sparse.Items[0].Volume = nil
	sparse.Items[0].VolumeUnit = "unknown"
	sparse.Items[0].Amount = nil
	sparse.Items[0].AmountUnit = "unknown"

	full := populatedIndexDailyBarsForFallbackTest("tushare")
	full.QualityStatus = model.QualityUnknown
	full.QualityReasons = []string{"provider_index_quality_unknown"}
	first := &indexDailyBarsFallbackTestProvider{name: "hithink", snapshot: sparse}
	second := &indexDailyBarsFallbackTestProvider{name: "tushare", snapshot: full}
	failed := &indexDailyBarsFallbackTestProvider{name: "third-source", err: errors.New("test index endpoint unavailable")}
	svc := &DataService{
		cache: cache.NewDisabled(),
		indexCatalogProviders: []provider.IndexCatalogProvider{first, second, failed},
	}

	got, hit, err := svc.IndexDailyBarsFromSource(context.Background(), "000300.SH", "2026-10-20", "2026-10-23", "")
	if err != nil || hit {
		t.Fatalf("IndexDailyBarsFromSource returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityUnknown ||
		len(got.Items) != 1 || got.Items[0].Volume == nil || got.Items[0].Amount == nil || got.Items[0].ChangePercent == nil {
		t.Fatalf("expected the more complete unknown daily-bars candidate, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected all providers to be tried, calls=%d/%d/%d", first.calls, second.calls, failed.calls)
	}
}

func TestIndexDailyBarsCachesUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &indexDailyBarsFallbackTestProvider{name: "hithink", snapshot: unknownIndexDailyBarsForFallbackTest("hithink")}
	second := &indexDailyBarsFallbackTestProvider{name: "tushare", err: errors.New("test endpoint unavailable")}
	svc := &DataService{
		cache: newMemoryCache(),
		indexCatalogProviders: []provider.IndexCatalogProvider{first, second},
	}
	got, hit, err := svc.IndexDailyBarsFromSource(context.Background(), "000300.SH", "2026-10-20", "2026-10-23", "")
	if err != nil || hit {
		t.Fatalf("IndexDailyBarsFromSource returned hit=%v err=%v", hit, err)
	}
	if got == nil || len(got.Items) != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown empty index daily-bars result, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "index_daily_bars_fallback_exhausted_without_confirmed_data" {
			found = true
		}
	}
	if !found || first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing fallback exhaustion marker or incomplete fallback: reasons=%v first=%d second=%d", got.QualityReasons, first.calls, second.calls)
	}
}

type indexCatalogAsOfSourceTestProvider struct {
	indexCatalogSourceTestProvider
	snapshot     *model.IndexConstituentSnapshot
	asOfCalls    int
	currentCalls int
}

func (p *indexCatalogAsOfSourceTestProvider) FetchIndexConstituents(context.Context, string) (*model.IndexConstituentSnapshot, error) {
	p.currentCalls++
	return p.snapshot, nil
}

func (p *indexCatalogAsOfSourceTestProvider) FetchIndexConstituentsAsOf(_ context.Context, indexSymbol, asOfDate string) (*model.IndexConstituentSnapshot, error) {
	p.asOfCalls++
	out := *p.snapshot
	out.IndexSymbol = indexSymbol
	out.AsOfDate = asOfDate
	out.Items = append([]model.IndexConstituent(nil), p.snapshot.Items...)
	return &out, nil
}

func TestIndexConstituentsAsOfUsesOnlyHistoricalCapability(t *testing.T) {
	now := time.Now().In(marketLocation())
	asOfDate := now.AddDate(0, 0, -5).Format("2006-01-02")
	reportDate := now.AddDate(0, 0, -6).Format("2006-01-02")
	snapshot := &model.IndexConstituentSnapshot{
		IndexSymbol: "000300.SH", ProviderReportDate: reportDate,
		Items: []model.IndexConstituent{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"}},
		Total: 1, ProviderReturned: 1, Source: "tushare", SourceVersion: "test-index-constituents-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_as_of_uses_latest_index_weight_report_not_effective_membership_interval"},
	}
	p := &indexCatalogAsOfSourceTestProvider{
		indexCatalogSourceTestProvider: indexCatalogSourceTestProvider{name: "tushare"},
		snapshot: snapshot,
	}
	svc := &DataService{cache: cache.NewDisabled(), indexCatalogProviders: []provider.IndexCatalogProvider{p}}
	got, hit, err := svc.IndexConstituentsAsOfFromSource(context.Background(), "000300.SH", asOfDate, "")
	if err != nil || hit {
		t.Fatalf("IndexConstituentsAsOfFromSource hit=%v err=%v", hit, err)
	}
	if got == nil || got.AsOfDate != asOfDate || got.ProviderReportDate != reportDate || len(got.Items) != 1 {
		t.Fatalf("unexpected historical constituent response: %+v", got)
	}
	if p.asOfCalls != 1 || p.currentCalls != 0 {
		t.Fatalf("historical lookup must call only as-of capability, historical=%d current=%d", p.asOfCalls, p.currentCalls)
	}
}

func TestIndexConstituentsAsOfDoesNotFallBackToCurrentOnlyProvider(t *testing.T) {
	p := indexCatalogSourceTestProvider{name: "hithink"}
	svc := &DataService{cache: cache.NewDisabled(), indexCatalogProviders: []provider.IndexCatalogProvider{p}}
	_, _, err := svc.IndexConstituentsAsOfFromSource(context.Background(), "000300.SH", time.Now().In(marketLocation()).AddDate(0, 0, -2).Format("2006-01-02"), "")
	if err == nil || !strings.Contains(err.Error(), "supports historical as_of queries") {
		t.Fatalf("current-only Provider must not be used for historical constituents, got %v", err)
	}
}

 
func TestIndexConstituentsAsOfCachesUnknownOnlyAfterFallbackExhausted(t *testing.T) {
	asOfDate := time.Now().In(marketLocation()).AddDate(0, 0, -20).Format("2006-01-02")
	p := &indexCatalogAsOfSourceTestProvider{
		indexCatalogSourceTestProvider: indexCatalogSourceTestProvider{name: "tushare"},
		snapshot: &model.IndexConstituentSnapshot{
			IndexSymbol: "000300.SH", Items: []model.IndexConstituent{}, Total: 0,
			Source: "tushare", SourceVersion: "test-index-constituents-unknown-v1",
			FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown,
			QualityReasons: []string{"provider_has_no_index_weight_snapshot_on_or_before_requested_as_of_within_120_months"},
		},
	}
	mem := newMemoryCache()
	svc := &DataService{cache: mem, indexCatalogProviders: []provider.IndexCatalogProvider{p}}

	first, hit, err := svc.IndexConstituentsAsOfFromSource(context.Background(), "000300.SH", asOfDate, "")
	if err != nil || hit {
		t.Fatalf("first historical lookup hit=%v err=%v", hit, err)
	}
	if first == nil || first.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown historical snapshot, got %+v", first)
	}

	second, hit, err := svc.IndexConstituentsAsOfFromSource(context.Background(), "000300.SH", asOfDate, "")
	if err != nil || !hit {
		t.Fatalf("repeated historical lookup should use exhausted-fallback cache: hit=%v err=%v", hit, err)
	}
	if second == nil || second.QualityStatus != model.QualityUnknown || p.asOfCalls != 1 {
		t.Fatalf("unknown cache changed quality or repeated upstream call: snapshot=%+v as_of_calls=%d", second, p.asOfCalls)
	}
}
