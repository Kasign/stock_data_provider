package httpapi

import (
	"context"
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

type httpTradingCalendarFake struct{}

func (httpTradingCalendarFake) Name() string { return "calendar-http-test" }

func (httpTradingCalendarFake) FetchTradingCalendar(_ context.Context, start, end string) (*model.TradingCalendarSnapshot, error) {
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: start, EndDate: end, TotalDays: 3, TradingDays: 2,
		Items: []model.TradingCalendarDay{
			{Date: "2026-10-22", IsTradingDay: true},
			{Date: "2026-10-23", IsTradingDay: true},
			{Date: "2026-10-24", IsTradingDay: false},
		},
		Source: "calendar-http-test", SourceVersion: "fixture-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}

func TestTradingCalendarRouteAndValidation(t *testing.T) {
	p := httpTradingCalendarFake{}
	svc := service.NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, service.ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})
	h := NewWithCapabilityInterfaces("secret", "forward_adjusted", svc, []provider.NamedProvider{p}, nil, nil, 2000, time.Second, nil).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/trading-calendar?start=2026-10-22&end=2026-10-24", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"trading_days":2`) || !strings.Contains(w.Body.String(), `"is_trading_day":false`) {
		t.Fatalf("calendar route status=%d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/market/trading-calendar?start=2026-10-24&end=2026-10-22", nil)
	req.Header.Set("X-API-Key", "secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("invalid range status=%d body=%s", w.Code, w.Body.String())
	}
}
