package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func TestPriceLimitRuleReturnsUnknownWithoutTrustedProviders(t *testing.T) {
	server := &Server{svc: &service.DataService{}}
	request := httptest.NewRequest("GET", "/api/v1/stocks/600519.SH/price-limit-rule?date=2026-10-29", nil)
	request.SetPathValue("symbol", "600519.SH")
	response := httptest.NewRecorder()

	server.priceLimitRule(response, request)

	if response.Code != 200 {
		t.Fatalf("expected unknown rule response, got HTTP %d: %s", response.Code, response.Body.String())
	}
	var got model.PriceLimitRuleSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != model.PriceLimitStatusUnknown || got.QualityStatus != model.QualityUnknown || got.LimitPercent != nil {
		t.Fatalf("missing trusted provider evidence must fail closed, got %+v", got)
	}
}

func TestPriceLimitRuleRejectsInvalidDate(t *testing.T) {
	server := &Server{svc: &service.DataService{}}
	request := httptest.NewRequest("GET", "/api/v1/stocks/600519.SH/price-limit-rule?date=2026-02-30", nil)
	request.SetPathValue("symbol", "600519.SH")
	response := httptest.NewRecorder()

	server.priceLimitRule(response, request)

	if response.Code != 400 {
		t.Fatalf("expected HTTP 400 for invalid date, got %d", response.Code)
	}
}
