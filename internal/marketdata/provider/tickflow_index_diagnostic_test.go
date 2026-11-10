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

func TestTickFlowIndexQuotesReportsDuplicateUnrequestedAndMissingIndexes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]any, 0, len(tickflowTestRequestedIndexDefinitions(r))+2)
		for _, def := range tickflowTestRequestedIndexDefinitions(r) {
			if def.Symbol == "399001.SZ" { continue }
			rows = append(rows, map[string]any{"symbol": def.Symbol, "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "amount": 123.0, "timestamp": time.Now().UTC().Add(-time.Minute).UnixMilli()})
		}
		if strings.Contains(r.URL.Query().Get("symbols"), "000001.SH") {
			rows = append(rows, map[string]any{"symbol": "000001.SH", "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "amount": 123.0})
			rows = append(rows, map[string]any{"symbol": "999999.SH", "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": 98.0, "amount": 123.0})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
	}))
	defer server.Close()
	reporter := validation.NewReporter(32)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	got, err := p.FetchIndexQuotes(ctx)
	if err != nil { t.Fatal(err) }
	if got.QualityStatus != model.QualityDegraded { t.Fatalf("expected degraded coverage: %+v", got) }
	seen := map[string]bool{}
	for _, issue := range reporter.Snapshot().Issues { seen[issue.Code+":"+issue.Field] = true }
	for _, key := range []string{"duplicate_identity:symbol", "unrequested_identity:symbol", "missing_requested_identity:symbol"} {
		if !seen[key] { t.Errorf("missing Reporter issue %q; got %+v", key, reporter.Snapshot().Issues) }
	}
}

func TestTickFlowIndexQuotesReportsOHLCInvariantViolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]any, 0, len(tickflowTestRequestedIndexDefinitions(r)))
		for _, def := range tickflowTestRequestedIndexDefinitions(r) {
			low := 98.0
			if def.Symbol == "000001.SH" { low = 103.0 }
			rows = append(rows, map[string]any{"symbol": def.Symbol, "last_price": 101.0, "prev_close": 100.0,
				"open": 99.0, "high": 102.0, "low": low, "amount": 123.0, "timestamp": time.Now().UTC().Add(-time.Minute).UnixMilli()})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": rows})
	}))
	defer server.Close()
	reporter := validation.NewReporter(32)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	if _, err := p.FetchIndexQuotes(ctx); err == nil { t.Fatal("expected invalid OHLC error") }
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Field != "ohlc" || issues[0].Code != "invariant_violation" || issues[0].RowIndex == nil || *issues[0].RowIndex != 0 {
		t.Fatalf("unexpected OHLC Reporter issue: %+v", issues)
	}
}
