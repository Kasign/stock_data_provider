package provider

import (
	"context"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/validation"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTickFlowQuoteRawFieldInvalidNumberIssue(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":[{"symbol":"600519.SH","last_price":"NaN","prev_close":100,"open":99,"high":102,"low":98,"volume":1000,"amount":100000}]}`))
	}))
	defer s.Close()
	rep := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), rep)
	p := NewTickFlowConfigured(2*time.Second, s.URL, "", "test-key")
	if _, err := p.FetchQuote(ctx, "600519.SH"); err == nil {
		t.Fatal("expected invalid required field")
	}
	issues := rep.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Field != "current_price" || issues[0].ProviderField != "last_price" || issues[0].Code != "invalid_number" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}
