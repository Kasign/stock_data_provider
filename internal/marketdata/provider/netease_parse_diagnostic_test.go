package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestNeteaseQuoteReportsIdentityAndConsistencyRejections(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		field   string
		code    string
	}{
		{
			name:    "missing requested identity",
			payload: `{"sh600519":{"code":"sh000001","symbol":"000001","price":10,"open":9,"high":11,"low":8,"yestclose":9,"updown":1,"percent":0.1}}`,
			field:   "symbol", code: "symbol_mismatch",
		},
		{
			name:    "change inconsistent",
			payload: `{"sh600519":{"code":"sh600519","symbol":"600519","price":10,"open":9,"high":11,"low":8,"yestclose":9,"updown":2,"percent":0.1}}`,
			field:   "change", code: "inconsistent_value",
		},
		{
			name:    "percent inconsistent",
			payload: `{"sh600519":{"code":"sh600519","symbol":"600519","price":10,"open":9,"high":11,"low":8,"yestclose":9,"updown":1,"percent":0.2}}`,
			field:   "change_percent", code: "inconsistent_value",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, "_ntes_quote_callback(%s);", tc.payload)
			}))
			defer server.Close()
			reporter := validation.NewReporter(10)
			ctx := validation.WithReporter(context.Background(), reporter)
			provider := NewNeteaseConfigured(2*time.Second, server.URL, "http://127.0.0.1:1/history")
			_, err := provider.FetchQuote(ctx, "600519.SH")
			if err == nil {
				t.Fatal("expected quote rejection")
			}
			found := false
			for _, issue := range reporter.Snapshot().Issues {
				if issue.Field == tc.field && issue.Code == tc.code && issue.Stage == validation.StageValidate {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s/%s diagnostic: %+v", tc.field, tc.code, reporter.Snapshot().Issues)
			}
		})
	}
}

func TestNeteaseQuoteReportsMissingRequestedIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `_ntes_quote_callback({"sh000001":{"code":"sh000001","symbol":"000001"}});`)
	}))
	defer server.Close()
	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewNeteaseConfigured(2*time.Second, server.URL, "http://127.0.0.1:1/history")
	_, err := provider.FetchQuote(ctx, "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "does not contain requested code") {
		t.Fatalf("expected missing requested identity error, got %v", err)
	}
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "symbol" && issue.Code == "missing_requested_identity" && issue.RowIndex == nil {
			return
		}
	}
	t.Fatalf("missing snapshot-level identity diagnostic: %+v", reporter.Snapshot().Issues)
}
