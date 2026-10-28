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

type indexCatalogQualityFallbackProvider struct {
	name          string
	catalog       *model.IndexCatalogSnapshot
	constituents  *model.IndexConstituentSnapshot
	catalogErr    error
	constituentErr error
	catalogCalls  int
	constituentCalls int
}

func (p *indexCatalogQualityFallbackProvider) Name() string { return p.name }

func (p *indexCatalogQualityFallbackProvider) FetchIndexCatalog(_ context.Context, tag string) (*model.IndexCatalogSnapshot, error) {
	p.catalogCalls++
	if p.catalogErr != nil {
		return nil, p.catalogErr
	}
	if p.catalog == nil {
		return nil, errors.New("test index catalog snapshot is nil")
	}
	out := *p.catalog
	out.Tag = tag
	out.Items = append([]model.IndexCatalogItem(nil), p.catalog.Items...)
	return &out, nil
}

func (p *indexCatalogQualityFallbackProvider) FetchIndexConstituents(_ context.Context, indexSymbol string) (*model.IndexConstituentSnapshot, error) {
	p.constituentCalls++
	if p.constituentErr != nil {
		return nil, p.constituentErr
	}
	if p.constituents == nil {
		return nil, errors.New("test index constituent snapshot is nil")
	}
	out := *p.constituents
	out.IndexSymbol = indexSymbol
	out.Items = append([]model.IndexConstituent(nil), p.constituents.Items...)
	return &out, nil
}

func (p *indexCatalogQualityFallbackProvider) FetchIndexDailyBars(context.Context, string, string, string) (*model.IndexDailyBarsSnapshot, error) {
	return nil, errors.New("not used by this test")
}

func indexCatalogUnknownDataForTest(source, tag string) *model.IndexCatalogSnapshot {
	return &model.IndexCatalogSnapshot{
		Tag: tag, Total: 1, Items: []model.IndexCatalogItem{{Symbol: "000300.SH", Name: "沪深300", Tag: tag}},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_catalog_timestamp_unknown"},
	}
}

func indexCatalogDegradedDataForTest(source, tag string) *model.IndexCatalogSnapshot {
	return &model.IndexCatalogSnapshot{
		Tag: tag, Total: 1, Items: []model.IndexCatalogItem{{Symbol: "000300.SH", Name: "沪深300", Tag: tag}},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_catalog_timestamp_not_exposed"},
	}
}

func indexConstituentsUnknownDataForTest(source string) *model.IndexConstituentSnapshot {
	return &model.IndexConstituentSnapshot{
		IndexSymbol: "000300.SH", Total: 1, ProviderReturned: 1,
		Items: []model.IndexConstituent{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"}},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_constituent_timestamp_unknown"},
	}
}

func indexConstituentsDegradedDataForTest(source string) *model.IndexConstituentSnapshot {
	return &model.IndexConstituentSnapshot{
		IndexSymbol: "000300.SH", Total: 1, ProviderReturned: 1,
		Items: []model.IndexConstituent{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"}},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_constituent_timestamp_not_exposed"},
	}
}

func indexCatalogQualityFallbackService(providers ...provider.IndexCatalogProvider) *DataService {
	return &DataService{cache: cache.NewDisabled(), indexCatalogProviders: providers}
}

func TestIndexCatalogFallsBackFromUnknownQualityEntries(t *testing.T) {
	first := &indexCatalogQualityFallbackProvider{name: "hithink", catalog: indexCatalogUnknownDataForTest("hithink", "cn_concept")}
	second := &indexCatalogQualityFallbackProvider{name: "tushare", catalog: indexCatalogDegradedDataForTest("tushare", "cn_concept")}
	svc := indexCatalogQualityFallbackService(first, second)

	got, hit, err := svc.IndexCatalog(context.Background(), "cn_concept")
	if err != nil || hit {
		t.Fatalf("IndexCatalog returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded catalog from fallback source, got %+v", got)
	}
	if first.catalogCalls != 1 || second.catalogCalls != 1 {
		t.Fatalf("expected both catalog providers to be called once, first=%d second=%d", first.catalogCalls, second.catalogCalls)
	}
}

func TestIndexConstituentsKeepsBestCoverageUnknownCandidate(t *testing.T) {
	plain := indexConstituentsUnknownDataForTest("hithink")
	rich := indexConstituentsUnknownDataForTest("tushare")
	weight := 1.35
	rich.Items[0].WeightPercent = &weight
	reportTime := time.Now().UTC().Add(-time.Hour)
	rich.ProviderTime = &reportTime
	rich.ProviderReportDate = reportTime.Format("2006-01-02")
	first := &indexCatalogQualityFallbackProvider{name: "hithink", constituents: plain}
	second := &indexCatalogQualityFallbackProvider{name: "tushare", constituents: rich}
	failed := &indexCatalogQualityFallbackProvider{name: "third-source", constituentErr: errors.New("test constituent endpoint unavailable")}
	svc := indexCatalogQualityFallbackService(first, second, failed)

	got, hit, err := svc.IndexConstituentsFromSource(context.Background(), "000300.SH", "")
	if err != nil || hit {
		t.Fatalf("IndexConstituentsFromSource returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityUnknown ||
		got.Items[0].WeightPercent == nil || got.ProviderTime == nil || got.ProviderReportDate == "" {
		t.Fatalf("expected the richer unknown constituent snapshot, got %+v", got)
	}
	if first.constituentCalls != 1 || second.constituentCalls != 1 || failed.constituentCalls != 1 {
		t.Fatalf("expected all providers to be attempted, calls=%d/%d/%d", first.constituentCalls, second.constituentCalls, failed.constituentCalls)
	}
}

func TestIndexConstituentsFallsBackFromUnknownQualityEntries(t *testing.T) {
	first := &indexCatalogQualityFallbackProvider{name: "hithink", constituents: indexConstituentsUnknownDataForTest("hithink")}
	second := &indexCatalogQualityFallbackProvider{name: "tushare", constituents: indexConstituentsDegradedDataForTest("tushare")}
	svc := indexCatalogQualityFallbackService(first, second)

	got, hit, err := svc.IndexConstituentsFromSource(context.Background(), "000300.SH", "")
	if err != nil || hit {
		t.Fatalf("IndexConstituentsFromSource returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "tushare" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded constituents from fallback source, got %+v", got)
	}
	if first.constituentCalls != 1 || second.constituentCalls != 1 {
		t.Fatalf("expected both constituent providers to be called once, first=%d second=%d", first.constituentCalls, second.constituentCalls)
	}
}
