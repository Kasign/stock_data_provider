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

type supplementHTTPFake struct{}

func (supplementHTTPFake) Name() string { return "miaoxiang" }
func (supplementHTTPFake) FetchChipDistribution(_ context.Context, symbol string) (*model.ChipDistribution, error) {
	ratio := 0.82
	return &model.ChipDistribution{
		Symbol: symbol, Date: "2026-10-21", AverageCost: 12.5, ProfitRatio: &ratio,
		Source: "miaoxiang", SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"publication_availability_timestamp_unavailable"},
	}, nil
}
func (supplementHTTPFake) FetchCapitalFlow(_ context.Context, symbol string) (*model.CapitalFlow, error) {
	amount := 1000000.0
	return &model.CapitalFlow{
		Symbol: symbol, MainNetInflow: &amount, Source: "miaoxiang", SourceVersion: "test-v1",
		FetchedAt: time.Now().UTC(), Observations: 1, QualityStatus: model.QualityDegraded,
	}, nil
}

func testSupplementHandler() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ds := service.NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Minute, 120, 1000, service.ResilienceConfig{}, logger)
	ds.SetSupplementProviders([]provider.SupplementProvider{supplementHTTPFake{}})
	return NewWithCapabilityInterfaces("test-secret", "forward_adjusted", ds,
		nil, nil, nil, 1000, time.Second, logger).Handler()
}

func TestSupplementEndpointsAndDataSourceCapability(t *testing.T) {
	h := testSupplementHandler()
	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/stocks/600519.SH/chip-distribution", `"average_cost":12.5`},
		{"/api/v1/stocks/600519.SH/capital-flow", `"main_net_inflow_yuan":1000000`},
		{"/api/v1/data-sources", `"chip_capable":true`},
		{"/api/v1/data-sources", `"capital_flow_capable":true`},
	}
	for _, item := range cases {
		req := httptest.NewRequest(http.MethodGet, item.path, nil)
		req.Header.Set("X-API-Key", "test-secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s status=%d body=%s", item.path, w.Code, w.Body.String())
			continue
		}
		if !strings.Contains(w.Body.String(), item.want) {
			t.Errorf("%s missing %s: %s", item.path, item.want, w.Body.String())
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Errorf("%s missing X-Request-ID", item.path)
		}
	}
}

func TestSupplementEndpointRequiresAuthentication(t *testing.T) {
	h := testSupplementHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600519.SH/chip-distribution", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request returned %d: %s", w.Code, w.Body.String())
	}
}

func TestDisabledSupplementReturnsProviderUnavailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ds := service.NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Minute, 120, 1000, service.ResilienceConfig{}, logger)
	h := NewWithCapabilityInterfaces("test-secret", "forward_adjusted", ds,
		nil, nil, nil, 1000, time.Second, logger).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stocks/600519.SH/chip-distribution", nil)
	req.Header.Set("X-API-Key", "test-secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled supplement returned %d: %s", w.Code, w.Body.String())
	}
}
