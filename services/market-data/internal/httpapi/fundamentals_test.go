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

type fundamentalsHTTPProvider struct{ httpFakeProvider }

func (fundamentalsHTTPProvider) FetchFundamentals(_ context.Context, symbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	now := time.Now().UTC()
	sections := []model.FundamentalSection{
		{Category: "profitability", ReportDate: "2026-06-30", PublicationDate: "2026-08-29", HasData: true,
			Metrics: []model.FundamentalMetric{{Key: "roe_avg", Name: "平均净资产收益率", Value: 0.074, Unit: "ratio", SourceField: "roeAvg"}}, QualityStatus: model.QualityValid},
		{Category: "operating", Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityUnknown},
		{Category: "growth", Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityUnknown},
		{Category: "solvency", Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityUnknown},
		{Category: "cash_flow", Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityUnknown},
		{Category: "dupont", Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityUnknown},
	}
	return &model.FundamentalsSnapshot{Symbol: symbol, Year: year, Quarter: quarter, HasData: true, Sections: sections, Source: "baostock",
		SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: []string{"partial_fixture"}}, nil
}

func TestFundamentalsRouteRequiresExplicitPeriod(t *testing.T) {
	p := fundamentalsHTTPProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetFundamentalsProviders([]provider.FundamentalsProvider{p})
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	for _, tc := range []struct {
		path         string
		status       int
		bodyContains string
	}{
		{"/api/v1/stocks/600519/fundamentals", http.StatusBadRequest, ""},
		{"/api/v1/stocks/600519/fundamentals?year=2026&quarter=5", http.StatusBadRequest, ""},
		{"/api/v1/stocks/600519/fundamentals?year=2026&quarter=2", http.StatusOK, ""},
		{"/api/v1/stocks/600519/fundamentals?year=2026&quarter=2&as_of=2026-07-01T00%3A00%3A00Z", http.StatusBadRequest, "as_of is not supported"},
		{"/api/v1/stocks/600519/fundamentals?year=2026&quarter=2&as_of=", http.StatusBadRequest, "as_of is not supported"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("X-API-Key", "secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s status=%d body=%s want=%d", tc.path, w.Code, w.Body.String(), tc.status)
		}
		if tc.bodyContains != "" && !strings.Contains(w.Body.String(), tc.bodyContains) {
			t.Fatalf("%s body=%s does not contain %q", tc.path, w.Body.String(), tc.bodyContains)
		}
		if tc.status == http.StatusOK && (w.Header().Get("X-Cache") != "MISS" ||
			!strings.Contains(w.Body.String(), "\"publication_date\":\"2026-08-29\"") ||
			!strings.Contains(w.Body.String(), "\"source_field\":\"roeAvg\"")) {
			t.Fatalf("fundamentals response contract incomplete: %s", w.Body.String())
		}
	}
}
