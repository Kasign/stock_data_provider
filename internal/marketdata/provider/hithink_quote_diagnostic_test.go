package provider

import (
	"context"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHithinkQuoteRawFieldTypeIssue(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"timestamp":1793680000000,"item":[{"thscode":"600519.SH","name":"贵州茅台","last_price":{},"prev_price":100,"open_price":99,"high_price":102,"low_price":98,"volume":1000,"turnover":100000}]}}`))
	}))
	defer s.Close()
	rep := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), rep)
	p := NewHithinkConfigured(2*time.Second, s.URL, "test-key")
	if _, err := p.FetchQuote(ctx, "600519.SH"); err == nil {
		t.Fatal("expected invalid required field")
	}
	issues := rep.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Field != "current_price" || issues[0].ProviderField != "last_price" || issues[0].Code != "type_mismatch" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}
