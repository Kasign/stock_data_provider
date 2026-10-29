package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

type securityStatusHTTPFixture struct{}

func (securityStatusHTTPFixture) Name() string { return "status-http-fixture" }

func (securityStatusHTTPFixture) FetchSecurityTradingStatus(_ context.Context, symbol, tradeDate string) (*model.SecurityTradingStatusSnapshot, error) {
	return &model.SecurityTradingStatusSnapshot{
		Symbol: symbol, TradeDate: tradeDate,
		SecurityName: "*ST贵州", NameEffectiveFrom: "2026-10-20",
		RiskWarningStatus: model.RiskWarningApplied, DelistingStatus: model.DelistingStatusNormal,
		ListingDate: "2026-10-20", ListingTradingDayNumber: 3, ListingStatusKnown: true,
		IsTradingDay: true, TradingDayKnown: true,
		SuspensionStatus: model.SuspensionStatusIntraday, SuspensionTiming: "09:30-10:00", SuspensionType: "S",
		Source: "status-http-fixture", SourceVersion: "fixture-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
	}, nil
}

var _ provider.SecurityTradingStatusProvider = securityStatusHTTPFixture{}

func securityStatusTestHandler() http.Handler {
	p := securityStatusHTTPFixture{}
	svc := service.NewWithCapabilities(
		[]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, service.ResilienceConfig{}, nil,
	)
	svc.SetSecurityTradingStatusProviders([]provider.SecurityTradingStatusProvider{p})
	return NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()
}

func TestSecurityTradingStatusHTTPReturnsHistoricalEvidence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600519.SH/trading-status?date=2026-10-22", nil)
	req.Header.Set("X-API-Key", "secret")
	recorder := httptest.NewRecorder()
	securityStatusTestHandler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("historical status code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected no-store on date-specific status, headers=%v", recorder.Header())
	}
	var got model.SecurityTradingStatusSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.RiskWarningStatus != model.RiskWarningApplied || got.SuspensionStatus != model.SuspensionStatusIntraday || got.TradeDate != "2026-10-22" {
		t.Fatalf("unexpected historical status response: %+v", got)
	}
}

func TestSecurityTradingStatusHTTPRequiresExactDate(t *testing.T) {
	for _, path := range []string{
		"/api/v1/stocks/600519.SH/trading-status",
		"/api/v1/stocks/600519.SH/trading-status?date=",
		"/api/v1/stocks/600519.SH/trading-status?date=2026-02-30",
		"/api/v1/stocks/600519.SH/trading-status?date=2027-01-01",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-API-Key", "secret")
		recorder := httptest.NewRecorder()
		securityStatusTestHandler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d body=%s; want 400", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestSecurityTradingStatusHTTPAsOfRequiresLocalStorage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/stocks/600519.SH/trading-status?date=2026-10-22&as_of=2026-10-23T00%3A00%3A00Z", nil)
	req.Header.Set("X-API-Key", "secret")
	recorder := httptest.NewRecorder()
	securityStatusTestHandler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("PIT storage disabled status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
