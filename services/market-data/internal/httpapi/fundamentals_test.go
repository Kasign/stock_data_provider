package httpapi

import (
	"context"
	"fmt"
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

func TestFundamentalsRouteRejectsFutureReportPeriod(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")
	futureQuarter := 0
	for quarter := 1; quarter <= 4; quarter++ {
		endDate := time.Date(now.Year(), time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		if endDate > today {
			futureQuarter = quarter
			break
		}
	}
	if futureQuarter == 0 {
		t.Skip("no future report quarter remains in the current calendar year")
	}

	p := fundamentalsHTTPProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetFundamentalsProviders([]provider.FundamentalsProvider{p})
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	path := fmt.Sprintf("/api/v1/stocks/600519/fundamentals?year=%d&quarter=%d", now.Year(), futureQuarter)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "fundamental report period must not be in the future") {
		t.Fatalf("future fundamentals period status=%d body=%s", w.Code, w.Body.String())
	}
}
