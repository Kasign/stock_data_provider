package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/service"
)

func TestBatchQuotesRejectsDuplicateSymbols(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("forward_adjusted", svc, []provider.Provider{p}, nil).InProcessHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/quotes?symbols=600519.SH,600519", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "duplicates") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBatchQuotesRejectsMoreThanFiftySymbols(t *testing.T) {
	p := httpFakeProvider{}
	svc := service.New([]provider.Provider{p}, cache.NewDisabled(), nil, time.Second, time.Minute, 120, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := New("forward_adjusted", svc, []provider.Provider{p}, nil).InProcessHandler()
	symbols := make([]string, 51)
	for i := range symbols {
		symbols[i] = "600519.SH"
	}
	// 超限检查必须先于重复检查。
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/quotes?symbols="+strings.Join(symbols, ","), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "no more than 50") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
