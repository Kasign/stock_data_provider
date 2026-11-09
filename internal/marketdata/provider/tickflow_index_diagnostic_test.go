package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestTickFlowIndexQuotesReportsRawFieldParseLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC().Add(-time.Minute).UnixMilli()
		for _, def := range tickflowTestRequestedIndexDefinitions(r) {
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":[{"symbol":%q,"name":%q,"last_price":"bad-price","prev_close":100,"open":99,"high":102,"low":98,"amount":123456,"timestamp":%d}]}`, def.Symbol, def.Name, now)))
			return
		}
	}))
	defer server.Close()

	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	if _, err := provider.FetchIndexQuotes(ctx); err == nil {
		t.Fatal("expected invalid index last_price")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Field != "last_price" || issues[0].ProviderField != "last_price" || issues[0].Code != "invalid_number" || issues[0].RowIndex == nil || *issues[0].RowIndex != 0 {
		t.Fatalf("unexpected TickFlow index parse issue: %+v", issues)
	}
}
