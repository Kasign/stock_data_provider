package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func TestDailyBarsAsOfReturnsUnavailableWhenRevisionStorageIsDisabled(t *testing.T) {
	handler := New("secret", "none", &service.DataService{}, nil, nil).Handler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/bars/600519.SH/daily?as_of=2026-10-01T00%3A00%3A00Z", nil)
	req.Header.Set("X-API-Key", "secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("PIT storage disabled status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "PIT_STORAGE_UNAVAILABLE") {
		t.Fatalf("expected explicit PIT storage error, body=%s", recorder.Body.String())
	}
}

func TestDailyBarsAsOfRejectsInvalidTimestamp(t *testing.T) {
	handler := New("secret", "none", &service.DataService{}, nil, nil).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bars/600519.SH/daily?as_of=invalid", nil)
	req.Header.Set("X-API-Key", "secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid as_of status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
