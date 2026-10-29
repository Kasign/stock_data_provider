package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func TestBatchQuotesRejectsDuplicateSymbols(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/quotes?symbols=600519.SH,600519", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "duplicates") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBatchQuotesRejectsMoreThanFiftySymbols(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("secret", "forward_adjusted", svc, []provider.Provider{p}, nil).Handler()
	symbols := make([]string, 51)
	for i := range symbols {
		symbols[i] = "600519.SH"
	}
	// 超限检查必须先于重复检查。
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/quotes?symbols="+strings.Join(symbols, ","), nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "no more than 50") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
