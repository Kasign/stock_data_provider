package service

import (
	"context"
	"testing"

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
