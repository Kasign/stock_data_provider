package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/application"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type serverData struct{}

func (serverData) FetchStockBundle(_ context.Context, code string, _ int, _ string) (domain.StockBundle, error) {
	return domain.StockBundle{Code: code, Name: code, DailyBars: []domain.DailyBar{{Date: "2026-10-20", Open: 1, High: 1, Low: 1, Close: 1, Volume: 1, Amount: 1}}}, nil
}
func (serverData) FetchMarketOverview(context.Context, string, string) (domain.MarketOverview, error) {
	return domain.MarketOverview{Date: "2026-10-20"}, nil
}

type serverSearch struct{}

func (serverSearch) SearchStockNews(context.Context, string, string, int) ([]domain.NewsItem, error) {
	return nil, nil
}
func (serverSearch) SearchMarketNews(context.Context, string) ([]domain.NewsItem, error) {
	return nil, nil
}

type serverAnalyzer struct{}

func (serverAnalyzer) AnalyzeStock(_ context.Context, _ domain.AnalysisContext, _ []domain.NewsItem, _ string, id, _ string) (domain.AnalysisResult, error) {
	return domain.AnalysisResult{Success: true, QueryID: id}, nil
}
func (serverAnalyzer) GenerateMarketReview(context.Context, domain.MarketOverview, []domain.NewsItem, string, string, string) (string, error) {
	return "ok", nil
}

func testServer() *Server {
	data := serverData{}
	search := serverSearch{}
	analyzer := serverAnalyzer{}
	return NewServer(Config{Token: "secret", RequestTimeout: time.Second}, application.NewStockFlow(data, search, analyzer, nil, nil), application.NewMarketReviewFlow(data, search, analyzer, nil, nil), nil)
}
func TestAnalysisEndpointRequiresToken(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/analysis/stock", bytes.NewBufferString(`{"stock_code":"600396"}`))
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}
func TestAnalysisEndpointRejectsUnknownFields(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/analysis/stock", bytes.NewBufferString(`{"stock_code":"600396","oops":1}`))
	req.Header.Set("X-Go-Core-Token", "secret")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
func TestAnalysisEndpointSuccess(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/analysis/stock", bytes.NewBufferString(`{"stock_code":"600396","search_news":false}`))
	req.Header.Set("X-Go-Core-Token", "secret")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["trace"]; !ok {
		t.Fatal("missing trace in response")
	}
}

func TestWorkflowErrorUsesGatewayFailureForUpstreamErrors(t *testing.T) {
	status, code, _ := workflowError(errors.New("python bridge unavailable"), "stock")
	if status != http.StatusBadGateway || code != "stock_workflow_failed" {
		t.Fatalf("unexpected upstream error mapping: status=%d code=%s", status, code)
	}
}

func TestWorkflowErrorUsesUnprocessableEntityForInputErrors(t *testing.T) {
	status, code, message := workflowError(&application.InputError{Message: "stock_code is required"}, "stock")
	if status != http.StatusUnprocessableEntity || code != "invalid_request" || message != "stock_code is required" {
		t.Fatalf("unexpected input error mapping: status=%d code=%s message=%s", status, code, message)
	}
}

func TestStockBatchEndpointReturnsPerStockResults(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/analysis/stocks", bytes.NewBufferString(`{"stock_codes":["600396","000001"],"search_news":false}`))
	req.Header.Set("X-Go-Core-Token", "secret")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body domain.StockBatchWorkflowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 2 || body.Succeeded != 2 || body.Failed != 0 || len(body.Results) != 2 {
		t.Fatalf("unexpected batch response: %#v", body)
	}
}

func TestStockBatchRejectsStaleAsOfBeforeStartingAnyItems(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	stale := time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339Nano)
	body := `{"stock_codes":["600396","000001"],"as_of":"` + stale + `"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/analysis/stocks", bytes.NewBufferString(body))
	req.Header.Set("X-Go-Core-Token", "secret")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for stale batch reference, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStockBatchRejectsDuplicateCodes(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/analysis/stocks", bytes.NewBufferString(`{"stock_codes":["600396","600396"]}`))
	req.Header.Set("X-Go-Core-Token", "secret")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReadinessRequiresWorkingBridgeProbe(t *testing.T) {
	data := serverData{}
	search := serverSearch{}
	analyzer := serverAnalyzer{}
	server := NewServer(Config{Token: "secret", ReadyCheck: func(context.Context) error { return errors.New("bridge down") }}, application.NewStockFlow(data, search, analyzer, nil, nil), application.NewMarketReviewFlow(data, search, analyzer, nil, nil), nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCapabilitiesEndpointRequiresToken(t *testing.T) {
	server := testServer()
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/v1/capabilities", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}
