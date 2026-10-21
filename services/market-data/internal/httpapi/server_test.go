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
