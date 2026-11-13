package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/service"
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

func TestPITDatabaseQueryFailureReturnsStorageUnavailable(t *testing.T) {
	db, err := sql.Open("mysql", "test:test@tcp(127.0.0.1:3306)/market_data_test?parseTime=true&timeout=1s")
	if err != nil {
		t.Fatalf("open SQL handle: %v", err)
	}
	store, err := repository.NewWithBorrowedDB(db)
	if err != nil {
		_ = db.Close()
		t.Fatalf("create borrowed store: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close SQL handle: %v", err)
	}

	svc := service.New(nil, cache.NewDisabled(), store, time.Second, time.Minute, 120, nil)
	handler := New("secret", "none", svc, nil, nil).Handler()
	asOf := url.QueryEscape(time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano))
	tradeDate := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).AddDate(0, 0, -1).Format("2006-01-02")
	paths := []string{
		"/api/v1/bars/600519.SH/daily?as_of=" + asOf,
		"/api/v1/stocks/600519.SH/fundamentals?year=2026&quarter=3&as_of=" + asOf,
		"/api/v1/stocks/600519.SH/trading-status?date=" + tradeDate + "&as_of=" + asOf,
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-API-Key", "secret")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("path=%q status=%d body=%s; expected storage unavailable", path, recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "PIT_STORAGE_UNAVAILABLE") {
			t.Errorf("path=%q body=%s; expected PIT_STORAGE_UNAVAILABLE", path, recorder.Body.String())
		}
	}
}
