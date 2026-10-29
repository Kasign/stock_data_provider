package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

type marketSessionCalendarFixture struct{}

func (marketSessionCalendarFixture) Name() string { return "market-session-fixture" }

func (marketSessionCalendarFixture) FetchTradingCalendar(_ context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}
	items := make([]model.TradingCalendarDay, 0)
	tradingDays := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		// HTTP fixture marks the requested date as a trading day independent of weekday.
		items = append(items, model.TradingCalendarDay{Date: day.Format("2006-01-02"), IsTradingDay: true})
		tradingDays++
	}
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: startDate, EndDate: endDate,
		TotalDays: len(items), TradingDays: tradingDays, Items: items,
		Source: "market-session-fixture", SourceVersion: "fixture-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}

var _ provider.TradingCalendarProvider = marketSessionCalendarFixture{}

func marketSessionHTTPHandler() http.Handler {
	svc := service.NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, service.ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{marketSessionCalendarFixture{}})
	return New("secret", "forward_adjusted", svc, nil, nil).Handler()
}

func TestMarketSessionRouteReturnsScheduledPhase(t *testing.T) {
	handler := marketSessionHTTPHandler()
	path := "/api/v1/market/session?at=" + url.QueryEscape("2026-10-22T09:30:00+08:00")
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-API-Key", "secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("market-session status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var got model.MarketSessionSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode market-session response: %v", err)
	}
	if got.Phase != model.MarketSessionContinuousAM || !got.IsTradingDay ||
		!got.IsTradingSession || !got.IsContinuousTrading || got.IsAuction {
		t.Fatalf("unexpected market-session payload: %+v", got)
	}
	if got.Source != "market-session-fixture" || got.LocalDate != "2026-10-22" || got.LocalTime != "09:30:00" || got.TimeZone != "Asia/Shanghai" || got.ServerTime.IsZero() {
		t.Fatalf("market-session source/local time is incorrect: %+v", got)
	}
}

func TestMarketSessionRouteRejectsInvalidTimestamp(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/session?at=not-a-timestamp", nil)
	req.Header.Set("X-API-Key", "secret")
	recorder := httptest.NewRecorder()
	marketSessionHTTPHandler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid timestamp status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
