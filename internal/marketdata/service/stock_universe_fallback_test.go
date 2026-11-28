package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

type stockUniverseStub struct {
	name     string
	snapshot *model.StockUniverseSnapshot
	err      error
	calls    int
}

func (s *stockUniverseStub) Name() string { return s.name }

func (s *stockUniverseStub) ListStockUniverse(context.Context) (*model.StockUniverseSnapshot, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.snapshot, nil
}

func newStockUniverseSnapshot(source string, items ...model.StockSearchItem) *model.StockUniverseSnapshot {
	return &model.StockUniverseSnapshot{
		Items: items, Total: len(items), Source: source, SourceVersion: source + "-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded, TotalIsLowerBound: true,
	}
}

func newUniverseTestService(t *testing.T) *DataService {
	t.Helper()
	return NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
}

func TestStockUniverseFallsBackToSecondaryDirectoryProvider(t *testing.T) {
	primary := &stockUniverseStub{name: "eastmoney", err: errors.New("push2 clist unreachable")}
	secondary := &stockUniverseStub{name: "baostock", snapshot: newStockUniverseSnapshot("baostock",
		model.StockSearchItem{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"},
	)}
	ds := newUniverseTestService(t)
	ds.SetStockUniverseProviders([]provider.StockUniverseProvider{primary, secondary})

	snapshot, err := ds.StockUniverse(context.Background())
	if err != nil {
		t.Fatalf("StockUniverse() error = %v", err)
	}
	if snapshot.Source != "baostock" {
		t.Fatalf("source = %q, want the secondary directory provider", snapshot.Source)
	}
	if primary.calls != 1 || secondary.calls != 1 {
		t.Fatalf("calls = (%d, %d), want both providers tried in order", primary.calls, secondary.calls)
	}
}

func TestStockUniverseUsesDirectoryProviderThatCannotSearch(t *testing.T) {
	directory := &stockUniverseStub{name: "baostock", snapshot: newStockUniverseSnapshot("baostock",
		model.StockSearchItem{Symbol: "000001.SZ", Code: "000001", Name: "平安银行", Exchange: "SZ"},
	)}
	ds := newUniverseTestService(t)
	// BaoStock 只提供全量目录、不提供关键词检索；目录注册不得依赖 stock_search 通道。
	ds.SetStockUniverseProviders([]provider.StockUniverseProvider{directory})

	if _, err := ds.StockUniverse(context.Background()); err != nil {
		t.Fatalf("StockUniverse() error = %v", err)
	}
	if directory.calls != 1 {
		t.Fatalf("directory calls = %d, want 1", directory.calls)
	}
}

func TestStockUniverseDoesNotQuerySharedProviderTwice(t *testing.T) {
	shared := &stockUniverseStub{name: "eastmoney", snapshot: newStockUniverseSnapshot("eastmoney",
		model.StockSearchItem{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"},
	)}
	ds := newUniverseTestService(t)
	ds.SetStockUniverseProviders([]provider.StockUniverseProvider{shared})
	ds.SetStockSearchProviders([]provider.StockSearchProvider{stockSearchStub{inner: shared}})

	if _, err := ds.StockUniverse(context.Background()); err != nil {
		t.Fatalf("StockUniverse() error = %v", err)
	}
	if shared.calls != 1 {
		t.Fatalf("shared provider calls = %d, want exactly 1", shared.calls)
	}
}

func TestStockUniverseReportsEveryProviderFailure(t *testing.T) {
	first := &stockUniverseStub{name: "eastmoney", err: errors.New("push2 clist unreachable")}
	second := &stockUniverseStub{name: "baostock", err: errors.New("query_all_stock unavailable")}
	ds := newUniverseTestService(t)
	ds.SetStockUniverseProviders([]provider.StockUniverseProvider{first, second})

	_, err := ds.StockUniverse(context.Background())
	if err == nil {
		t.Fatal("StockUniverse() must fail when every directory provider fails")
	}
	for _, want := range []string{"eastmoney", "push2 clist unreachable", "baostock", "query_all_stock unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestStockUniverseRejectsIncompleteSecondarySnapshot(t *testing.T) {
	broken := newStockUniverseSnapshot("baostock",
		model.StockSearchItem{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"},
	)
	broken.Total = 2 // 条目数与 Total 不一致，说明目录被截断
	ds := newUniverseTestService(t)
	ds.SetStockUniverseProviders([]provider.StockUniverseProvider{
		&stockUniverseStub{name: "eastmoney", err: errors.New("push2 clist unreachable")},
		&stockUniverseStub{name: "baostock", snapshot: broken},
	})

	if _, err := ds.StockUniverse(context.Background()); err == nil {
		t.Fatal("an incomplete directory must not be served as the stock universe")
	}
}

// stockSearchStub 同时实现搜索与目录能力，用于验证两条注册通道按名称去重。
type stockSearchStub struct {
	inner provider.StockUniverseProvider
}

func (s stockSearchStub) Name() string { return s.inner.Name() }

func (s stockSearchStub) SearchStocks(_ context.Context, query string, limit int) (*model.StockSearchSnapshot, error) {
	return &model.StockSearchSnapshot{Query: query, Source: s.inner.Name(), SourceVersion: "stub-v1", FetchedAt: time.Now().UTC()}, nil
}

func (s stockSearchStub) ListStockUniverse(ctx context.Context) (*model.StockUniverseSnapshot, error) {
	return s.inner.ListStockUniverse(ctx)
}
