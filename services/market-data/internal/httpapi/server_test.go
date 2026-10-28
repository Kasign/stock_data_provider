package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

type httpFakeProvider struct{}

func (httpFakeProvider) Name() string { return "eastmoney" }
func (httpFakeProvider) SearchStocks(_ context.Context, query string, limit int) (*model.StockSearchSnapshot, error) {
	items := []model.StockSearchItem{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"}}
	if limit < len(items) {
		items = items[:limit]
	}
	return &model.StockSearchSnapshot{Query: query, Total: len(items), Items: items, Source: "eastmoney", SourceVersion: "test", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid}, nil
}
func (httpFakeProvider) FetchIndexQuotes(_ context.Context) (*model.IndexQuoteSnapshot, error) {
	now := time.Now().UTC()
	price, change, pct, amount := 3200.5, 24.5, 0.8, 123456789.0
	return &model.IndexQuoteSnapshot{Source: "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"}, Items: []model.MarketIndexQuote{{Symbol: "000001.SH", Code: "000001", Name: "上证指数", LatestPrice: &price, Change: &change, ChangePercent: &pct, Amount: &amount, AmountUnit: "CNY", Source: "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"}}}}, nil
}
func (httpFakeProvider) FetchSectorRankings(_ context.Context, category string, _ int) (*model.SectorRankingSnapshot, error) {
	now := time.Now().UTC()
	pct, price, amount := 2.4, 123.45, 987654321.0
	return &model.SectorRankingSnapshot{
		Category: category, Total: 1, Source: "eastmoney", SourceVersion: "test",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		Items: []model.SectorRanking{{
			Rank: 1, Code: "BK0475", Name: "测试板块", Category: category,
			LatestPrice: &price, ChangePercent: &pct, Amount: &amount, AmountUnit: "CNY",
			Source: "eastmoney", SourceVersion: "test", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		}},
	}, nil
}

func (p httpFakeProvider) FetchSectorRankingsDirection(ctx context.Context, category, direction string, limit int) (*model.SectorRankingSnapshot, error) {
	snapshot, err := p.FetchSectorRankings(ctx, category, limit)
	if snapshot != nil {
		snapshot.Direction = direction
	}
	return snapshot, err
}
func (httpFakeProvider) FetchStockBasic(_ context.Context, symbol string) (*model.StockBasic, error) {
	return &model.StockBasic{Symbol: symbol, Name: "测试证券", IPODate: "2000-01-01", SecurityType: 1, SecurityTypeLabel: "stock", ListingStatus: 1, ListingStatusLabel: "listed", Source: "eastmoney", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid}, nil
}
func (httpFakeProvider) FetchStockBoards(_ context.Context, symbol string) (*model.StockBoardSnapshot, error) {
	now := time.Now().UTC()
	change := 1.25
	return &model.StockBoardSnapshot{
		Symbol: symbol, Total: 1, HasData: true,
		Items:  []model.StockBoard{{Code: "BK0477", Name: "酿酒行业", ChangePercent: &change, Source: "eastmoney", FetchedAt: now, QualityStatus: model.QualityDegraded}},
		Source: "eastmoney", SourceVersion: "test-stock-boards-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}
func (httpFakeProvider) FetchDragonTiger(_ context.Context, tradeDate string, page, pageSize int) (*model.DragonTigerSnapshot, error) {
	now := time.Now().UTC()
	netBuy, buy, sell, closePrice, change := 123456.0, 234567.0, 111111.0, 10.25, 2.5
	item := model.DragonTigerItem{
		Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", TradeDate: tradeDate,
		ChangePercent: &change, ClosePrice: &closePrice, NetBuyAmountCNY: &netBuy,
		BuyAmountCNY: &buy, SellAmountCNY: &sell, Reason: "日涨幅偏离值达到7%",
		Source: "eastmoney", SourceVersion: "test", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
	return &model.DragonTigerSnapshot{
		TradeDate: tradeDate, ProviderReportDate: tradeDate, Page: page, PageSize: pageSize,
		Total: 1, Pages: 1, Returned: 1, SourceReturned: 1, ExcludedNonAShareCount: 0, HasData: true, HasMore: false,
		Items: []model.DragonTigerItem{item}, Source: "eastmoney", SourceVersion: "test",
		FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}

func (httpFakeProvider) FetchStockAnnouncements(_ context.Context, rawSymbol, startDate, endDate string, page, pageSize int) (*model.StockAnnouncementSnapshot, error) {
	now := time.Now().UTC()
	published := time.Date(2026, 10, 23, 10, 30, 0, 0, time.UTC)
	item := model.StockAnnouncement{
		ArticleID: "AN202610231234567890", Symbol: rawSymbol, StockName: "招商银行",
		Title: "招商银行第三季度报告", Category: "定期报告", AnnouncementDate: "2026-10-23",
		PublishedAt: &published, DisplayTime: "2026-10-23 18:30:00",
		SourceURL: "https://data.eastmoney.com/notices/detail/600036/AN202610231234567890.html",
		Source:    "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityValid,
	}
	return &model.StockAnnouncementSnapshot{
		Symbol: rawSymbol, StartDate: startDate, EndDate: endDate, Page: page, PageSize: pageSize,
		Total: 1, Pages: 1, Returned: 1, HasData: true, HasMore: false,
		Items: []model.StockAnnouncement{item}, Source: "eastmoney", SourceVersion: "test",
		FetchedAt: now, QualityStatus: model.QualityValid,
	}, nil
}

func (httpFakeProvider) FetchQuote(context.Context, string) (*model.Quote, error) {
	p := 10.0
	open := 9.8
	high := 10.5
	low := 9.7
	prev := 9.8
	vol := int64(100000)
	amount := 1000000.0
	change := 0.2
	pct := 2.04
	t := time.Now().UTC()
	return &model.Quote{Symbol: "600519.SH", Name: "test", CurrentPrice: &p, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &vol, Amount: &amount, QuoteTime: &t, FetchedAt: t, NormalizedAt: t, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid}, nil
}
func (httpFakeProvider) FetchDailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, error) {
	v := int64(100000)
	a := 1000000.0
	return []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 9.8, High: 10.5, Low: 9.7, Close: 10, Volume: &v, Amount: &a, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid}}, nil
}

func (httpFakeProvider) FetchMarketBreadth(_ context.Context) (*model.MarketBreadthSnapshot, error) {
	now := time.Now().UTC()
	amount := 1000000.0
	return &model.MarketBreadthSnapshot{
		Market: "cn_a_share", TotalSecurities: 3, CoveredSecurities: 3,
		AdvancingCount: 1, DecliningCount: 1, UnchangedCount: 1,
		TotalAmountCNY: &amount, AmountCoverageCount: 3, Source: "eastmoney",
		SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}
func (httpFakeProvider) FetchLimitPool(_ context.Context, poolType, tradeDate string, _ int) (*model.LimitPoolSnapshot, error) {
	now := time.Now().UTC()
	change := 10.0
	return &model.LimitPoolSnapshot{
		PoolType: poolType, TradeDate: tradeDate, Total: 1, HasData: true,
		Items:  []model.LimitPoolItem{{Symbol: "600519.SH", Code: "600519", Name: "测试证券", ChangePercent: &change, Source: "eastmoney", FetchedAt: now, QualityStatus: model.QualityDegraded}},
		Source: "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}

func TestAuthIsRequiredForDataRoutes(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/600519", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
func TestQuoteRouteReturnsNormalizedUnits(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/600519", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"volume_unit":"shares"`) || !strings.Contains(w.Body.String(), `"amount_unit":"CNY"`) {
		t.Fatalf("missing unit contract: %s", w.Body.String())
	}
}
func TestUnsupportedBShareReturnsValidationErrorNotProviderFailure(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/900901", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("unsupported B-share should be a client validation error, got status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMismatchedExchangeSuffixReturnsValidationError(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, nil)
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/600519.SZ", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("mismatched exchange suffix should be a client validation error, got status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLiveHealthDoesNotRequireAuth(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, nil)
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestFutureAsOfIsValidationError(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	future := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bundles/600519?as_of="+future, nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("unexpected error code: %s", w.Body.String())
	}
}

type quoteOnlyCapabilityFake struct{}

func (quoteOnlyCapabilityFake) Name() string { return "sina" }
func (quoteOnlyCapabilityFake) FetchQuote(context.Context, string) (*model.Quote, error) {
	return nil, nil
}

func TestStockBasicProfileEndpointReturnsIndependentProviderData(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities(
		[]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p},
		cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil,
	)
	svc.SetStockBasicProviders([]provider.StockBasicProvider{p})
	svc.SetStockSearchProviders([]provider.StockSearchProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc,
		[]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, 2000, time.Second, nil,
	).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600519.SH/profile", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for _, want := range []string{"\"symbol\":\"600519.SH\"", "\"name\":\"测试证券\""} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("profile response missing %s: %s", want, w.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), "\"stock_basic_capable\":true") ||
		!strings.Contains(w.Body.String(), "\"stock_search_capable\":true") {
		t.Fatalf("data-sources must advertise independent stock-basic and stock-search capabilities: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDomesticMarketBreadthAndLimitPoolRoutes(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetMarketBreadthProviders([]provider.MarketBreadthProvider{p})
	svc.SetLimitPoolProviders([]provider.LimitPoolProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/breadth", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"advancing_count":1`) || !strings.Contains(w.Body.String(), `"covered_securities":3`) {
		t.Fatalf("breadth route status=%d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/market/limit-pools?type=up&date=2026-10-23&limit=10", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"type":"up"`) || !strings.Contains(w.Body.String(), `"trade_date":"2026-10-23"`) {
		t.Fatalf("limit-pool route status=%d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/market/limit-pools?type=invalid", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("invalid pool type status=%d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"market_breadth_capable":true`) || !strings.Contains(w.Body.String(), `"limit_pool_capable":true`) {
		t.Fatalf("market capabilities missing: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestStockBoardsRouteAndCapabilityAdvertisement(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockBoardsProviders([]provider.StockBoardsProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600519.SH/boards", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"symbol":"600519.SH"`) || !strings.Contains(w.Body.String(), `"code":"BK0477"`) {
		t.Fatalf("stock-boards route status=%d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"stock_boards_capable":true`) {
		t.Fatalf("data-sources must advertise stock boards: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDragonTigerEndpointUsesDedicatedCapabilityAndAdvertisesIt(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetDragonTigerProviders([]provider.DragonTigerProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/dragon-tiger?date=2026-10-22&page=1&page_size=50", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for _, want := range []string{`"trade_date":"2026-10-22"`, `"net_buy_amount_cny":123456`, `"quality_status":"degraded"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, w.Body.String())
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"dragon_tiger_capable":true`) {
		t.Fatalf("data sources missing capability: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDragonTigerRejectsInvalidPagination(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetDragonTigerProviders([]provider.DragonTigerProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/dragon-tiger?page=0", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("invalid page should be rejected: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestStockAnnouncementsEndpointAndCapabilityAdvertisement(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600036.SH/announcements?start_date=2026-10-20&end_date=2026-10-26", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for _, want := range []string{`"article_id":"AN202610231234567890"`, `"announcement_date":"2026-10-23"`, `"source_url":"https://data.eastmoney.com/notices/detail/600036/AN202610231234567890.html"`, `"quality_status":"valid"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("announcement response missing %s: %s", want, w.Body.String())
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"announcements_capable":true`) {
		t.Fatalf("data sources should advertise announcements: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestStockAnnouncementsRejectsInvalidPagination(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600036/announcements?page_size=101", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("invalid announcement page_size should be rejected: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDataSourcesReportsQuoteOnlyProviderWithoutDailyBars(t *testing.T) {
	p := quoteOnlyCapabilityFake{}
	h := NewWithCapabilityInterfaces(
		"secret", "forward_adjusted", nil,
		[]provider.NamedProvider{p},
		[]provider.QuoteProvider{p},
		nil, 2000, time.Second, nil,
	).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"\"name\":\"sina\"",
		"\"quote_capable\":true",
		"\"daily_bar_capable\":false",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("data-sources response missing %s: %s", want, body)
		}
	}
}

func TestSectorRankingEndpointSeparatesIndustryAndConcept(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities(
		[]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p},
		cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil,
	)
	svc.SetSectorRankingProviders([]provider.SectorRankingProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc,
		[]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, 2000, time.Second, nil,
	).Handler()
	for _, category := range []string{"industry", "concept"} {
		for _, direction := range []string{"up", "down"} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/market/sectors?type="+category+"&direction="+direction+"&limit=5", nil)
			req.Header.Set("X-API-Key", "secret")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("category=%s direction=%s status=%d body=%s", category, direction, w.Code, w.Body.String())
			}
			for _, want := range []string{`"category":"` + category + `"`, `"direction":"` + direction + `"`, `"code":"BK0475"`, `"amount_unit":"CNY"`, `"quality_status":"degraded"`} {
				if !strings.Contains(w.Body.String(), want) {
					t.Fatalf("category=%s direction=%s response missing %s: %s", category, direction, want, w.Body.String())
				}
			}
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/sectors?type=all", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("invalid sector category should be rejected, got status=%d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/market/sectors?type=industry&direction=sideways", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("invalid sector direction should be rejected, got status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMarketIndicesEndpointUsesDedicatedIndexCapability(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetIndexQuoteProviders([]provider.IndexQuoteProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, 2000, time.Second, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/indices", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for _, want := range []string{`"symbol":"000001.SH"`, `"name":"上证指数"`, `"amount_unit":"CNY"`, `"quality_status":"degraded"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("index snapshot missing %s: %s", want, w.Body.String())
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/data-sources", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"index_quote_capable":true`) {
		t.Fatalf("data sources missing index capability: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestStockSearchEndpointReturnsCanonicalMatchesAndValidatesParameters(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p},
		cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetStockSearchProviders([]provider.StockSearchProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc,
		[]provider.NamedProvider{p}, []provider.QuoteProvider{p}, []provider.DailyBarProvider{p}, 2000, time.Second, nil,
	).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/search?q=%E8%B4%B5%E5%B7%9E%E8%8C%85%E5%8F%B0", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing API key status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/stocks/search?q=%E8%B4%B5%E5%B7%9E%E8%8C%85%E5%8F%B0&limit=10", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Market-Data-Cache"); got != "" {
		t.Fatalf("non-stale stock search must not claim a service cache status, got %q", got)
	}
	for _, want := range []string{`"query":"贵州茅台"`, `"symbol":"600519.SH"`, `"code":"600519"`, `"name":"贵州茅台"`, `"exchange":"SH"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("stock-search response missing %s: %s", want, w.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/stocks/search?q=", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("empty query status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/stocks/search?q=%E8%B4%B5%E5%B7%9E%E8%8C%85%E5%8F%B0&limit=51", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status=%d body=%s", w.Code, w.Body.String())
	}
}

type httpHistoricalIndexProvider struct{}

func (httpHistoricalIndexProvider) Name() string { return "tushare" }
func (httpHistoricalIndexProvider) FetchIndexCatalog(context.Context, string) (*model.IndexCatalogSnapshot, error) {
	return nil, nil
}
func (httpHistoricalIndexProvider) FetchIndexConstituents(context.Context, string) (*model.IndexConstituentSnapshot, error) {
	return nil, nil
}
func (httpHistoricalIndexProvider) FetchIndexConstituentsAsOf(_ context.Context, indexSymbol, asOfDate string) (*model.IndexConstituentSnapshot, error) {
	location := marketLocationForHTTPTest()
	asOf, _ := time.ParseInLocation("2006-01-02", asOfDate, location)
	reportDate := asOf.AddDate(0, 0, -1).Format("2006-01-02")
	now := time.Now().UTC()
	return &model.IndexConstituentSnapshot{
		IndexSymbol: indexSymbol, AsOfDate: asOfDate, ProviderReportDate: reportDate,
		Items: []model.IndexConstituent{{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台"}},
		Total: 1, ProviderReturned: 1, Source: "tushare", SourceVersion: "http-index-asof-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"test_historical_constituent_snapshot"},
	}, nil
}
func (httpHistoricalIndexProvider) FetchIndexDailyBars(context.Context, string, string, string) (*model.IndexDailyBarsSnapshot, error) {
	return nil, nil
}

func marketLocationForHTTPTest() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}

func TestIndexConstituentsAsOfEndpointReturnsRequestedDateAndReportDate(t *testing.T) {
	p := httpHistoricalIndexProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetIndexCatalogProviders([]provider.IndexCatalogProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
	location := marketLocationForHTTPTest()
	asOfDate := time.Now().In(location).AddDate(0, 0, -4).Format("2006-01-02")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/indices/000300.SH/constituents?as_of="+asOfDate+"&source=tushare", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for _, want := range []string{`"as_of_date":"` + asOfDate + `"`, `"provider_report_date":"`, `"symbol":"600519.SH"`, `"quality_status":"degraded"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("historical constituent response missing %s: %s", want, w.Body.String())
		}
	}
}

func TestIndexConstituentsRejectsInvalidAsOfQueryAndCurrentOnlySource(t *testing.T) {
	p := httpHistoricalIndexProvider{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetIndexCatalogProviders([]provider.IndexCatalogProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
	for _, path := range []string{
		"/api/v1/market/indices/000300.SH/constituents?as_of=2026-1-5",
		"/api/v1/market/indices/000300.SH/constituents?as_of=2026-10-20&source=hithink",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-API-Key", "secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
			t.Fatalf("invalid historical query %q status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
