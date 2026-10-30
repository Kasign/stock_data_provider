package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type unknownEmptyFundamentalsFallbackTestProvider struct {
	name     string
	snapshot *model.FundamentalsSnapshot
	err      error
	calls    int
}

func (p *unknownEmptyFundamentalsFallbackTestProvider) Name() string { return p.name }

func (p *unknownEmptyFundamentalsFallbackTestProvider) FetchFundamentals(_ context.Context, symbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test fundamentals snapshot is nil")
	}
	out := *p.snapshot
	out.Symbol, out.Year, out.Quarter = symbol, year, quarter
	out.Sections = append([]model.FundamentalSection(nil), p.snapshot.Sections...)
	for i := range out.Sections {
		out.Sections[i].Metrics = append([]model.FundamentalMetric(nil), p.snapshot.Sections[i].Metrics...)
	}
	return &out, nil
}

type indexCatalogFallbackTestProvider struct {
	name             string
	catalog          *model.IndexCatalogSnapshot
	constituents     *model.IndexConstituentSnapshot
	catalogErr       error
	constituentsErr  error
	catalogCalls     int
	constituentCalls int
}

func (p *indexCatalogFallbackTestProvider) Name() string { return p.name }

func (p *indexCatalogFallbackTestProvider) FetchIndexCatalog(_ context.Context, tag string) (*model.IndexCatalogSnapshot, error) {
	p.catalogCalls++
	if p.catalogErr != nil {
		return nil, p.catalogErr
	}
	if p.catalog == nil {
		return nil, errors.New("test index catalog is nil")
	}
	out := *p.catalog
	out.Tag = tag
	out.Items = append([]model.IndexCatalogItem(nil), p.catalog.Items...)
	for i := range out.Items {
		out.Items[i].Tag = tag
	}
	return &out, nil
}

func (p *indexCatalogFallbackTestProvider) FetchIndexConstituents(_ context.Context, indexSymbol string) (*model.IndexConstituentSnapshot, error) {
	p.constituentCalls++
	if p.constituentsErr != nil {
		return nil, p.constituentsErr
	}
	if p.constituents == nil {
		return nil, errors.New("test index constituents is nil")
	}
	out := *p.constituents
	out.IndexSymbol = indexSymbol
	out.Items = append([]model.IndexConstituent(nil), p.constituents.Items...)
	return &out, nil
}

func (p *indexCatalogFallbackTestProvider) FetchIndexDailyBars(context.Context, string, string, string) (*model.IndexDailyBarsSnapshot, error) {
	return nil, errors.New("unused test index daily bars")
}

func unknownEmptyFundamentalsFallbackService(providers ...provider.FundamentalsProvider) *DataService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, logger)
	svc.SetFundamentalsProviders(providers)
	return svc
}

func unknownFundamentalsForFallbackTest(source string) *model.FundamentalsSnapshot {
	sections := make([]model.FundamentalSection, 0, 6)
	for _, category := range []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"} {
		sections = append(sections, model.FundamentalSection{Category: category, Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_period_not_available"}})
	}
	return &model.FundamentalsSnapshot{Symbol: "600519.SH", Year: 2025, Quarter: 4, HasData: false, Sections: sections, Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_period_not_available"}}
}

func validFundamentalsForFallbackTest(source string) *model.FundamentalsSnapshot {
	sections := make([]model.FundamentalSection, 0, 6)
	for _, category := range []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"} {
		sections = append(sections, model.FundamentalSection{Category: category, ReportDate: "2025-12-31", PublicationDate: "2026-03-31", HasData: true, Metrics: []model.FundamentalMetric{{Key: "test_metric", Name: "测试指标", Value: 1, Unit: "元", SourceField: "test_field"}}, QualityStatus: model.QualityDegraded, QualityReasons: []string{"test_provider_timestamp_not_exposed"}})
	}
	return &model.FundamentalsSnapshot{Symbol: "600519.SH", Year: 2025, Quarter: 4, HasData: true, Sections: sections, Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded, QualityReasons: []string{"test_provider_timestamp_not_exposed"}}
}

func unknownIndexCatalogForFallbackTest(source, tag string) *model.IndexCatalogSnapshot {
	return &model.IndexCatalogSnapshot{Tag: tag, Items: []model.IndexCatalogItem{}, Total: 0, Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_catalog_empty_unconfirmed"}}
}

func validIndexCatalogForFallbackTest(source, tag string) *model.IndexCatalogSnapshot {
	return &model.IndexCatalogSnapshot{Tag: tag, Items: []model.IndexCatalogItem{{Symbol: "000300.SH", Name: "沪深300", Tag: tag}}, Total: 1, Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"}}
}

func unknownIndexConstituentsForFallbackTest(source string) *model.IndexConstituentSnapshot {
	return &model.IndexConstituentSnapshot{IndexSymbol: "000300.SH", Items: []model.IndexConstituent{}, Total: 0, Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_constituents_empty_unconfirmed"}}
}

func validIndexConstituentsForFallbackTest(source string) *model.IndexConstituentSnapshot {
	return &model.IndexConstituentSnapshot{IndexSymbol: "000300.SH", Items: []model.IndexConstituent{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"}}, Total: 1, ProviderReturned: 1, ExcludedCount: 0, Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"}}
}

func TestFundamentalsFallsBackAfterUnknownEmptyReportPeriod(t *testing.T) {
	first := &unknownEmptyFundamentalsFallbackTestProvider{name: "baostock", snapshot: unknownFundamentalsForFallbackTest("baostock")}
	second := &unknownEmptyFundamentalsFallbackTestProvider{name: "tushare", snapshot: validFundamentalsForFallbackTest("tushare")}
	got, hit, err := unknownEmptyFundamentalsFallbackService(first, second).Fundamentals(context.Background(), "600519.SH", 2025, 4)
	if err != nil || hit {
		t.Fatalf("Fundamentals returned hit=%v err=%v", hit, err)
	}
	if got == nil || !got.HasData || got.Source != "tushare" || first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected the second provider's financial metrics, snapshot=%+v calls=%d/%d", got, first.calls, second.calls)
	}
}

func TestIndexCatalogFallsBackAfterUnknownEmptyCatalog(t *testing.T) {
	first := &indexCatalogFallbackTestProvider{name: "eastmoney", catalog: unknownIndexCatalogForFallbackTest("eastmoney", "cn_concept")}
	second := &indexCatalogFallbackTestProvider{name: "hithink", catalog: validIndexCatalogForFallbackTest("hithink", "cn_concept")}
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetIndexCatalogProviders([]provider.IndexCatalogProvider{first, second})
	got, hit, err := svc.IndexCatalog(context.Background(), "cn_concept")
	if err != nil || hit {
		t.Fatalf("IndexCatalog returned hit=%v err=%v", hit, err)
	}
	if got == nil || len(got.Items) != 1 || got.Source != "hithink" || first.catalogCalls != 1 || second.catalogCalls != 1 {
		t.Fatalf("expected fallback index catalog, snapshot=%+v calls=%d/%d", got, first.catalogCalls, second.catalogCalls)
	}
}

func TestIndexConstituentsFallsBackAfterUnknownEmptyMembership(t *testing.T) {
	first := &indexCatalogFallbackTestProvider{name: "hithink", constituents: unknownIndexConstituentsForFallbackTest("hithink")}
	second := &indexCatalogFallbackTestProvider{name: "tushare", constituents: validIndexConstituentsForFallbackTest("tushare")}
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetIndexCatalogProviders([]provider.IndexCatalogProvider{first, second})
	got, hit, err := svc.IndexConstituentsFromSource(context.Background(), "000300.SH", "")
	if err != nil || hit {
		t.Fatalf("IndexConstituents returned hit=%v err=%v", hit, err)
	}
	if got == nil || len(got.Items) != 1 || got.Source != "tushare" || first.constituentCalls != 1 || second.constituentCalls != 1 {
		t.Fatalf("expected fallback index constituents, snapshot=%+v calls=%d/%d", got, first.constituentCalls, second.constituentCalls)
	}
}
