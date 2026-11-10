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

func TestEastmoneyFetchIndexQuotesReportsMissingOptionalEvidenceAndCoverage(t *testing.T) {
	definition := eastmoneyCoreIndexDefinitions[0]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"rc":0,"data":{"total":1,"diff":[{"f2":100,"f3":1,"f4":1,"f12":%q,"f13":%q,"f14":%q,"f18":99}]}}`,
			definition.Code, definition.Market, definition.Name)
	}))
	defer server.Close()

	reporter := validation.NewReporter(12)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewEastmoneyConfiguredWithIndices(2*time.Second, "", "", "", "", server.URL, "")
	got, err := p.FetchIndexQuotes(ctx)
	if err != nil {
		t.Fatalf("FetchIndexQuotes: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("valid index quote should be retained: %+v", got)
	}
	issues := reporter.Snapshot().Issues
	foundTime, foundAmount, foundCoverage := false, false, false
	for _, issue := range issues {
		if issue.Stage != validation.StageValidate || issue.Severity != validation.SeverityWarning {
			continue
		}
		switch issue.Field {
		case "provider_time":
			foundTime = issue.ProviderField == "f86" && issue.Code == "missing_or_invalid_value" &&
				issue.RowIndex != nil && *issue.RowIndex == 0
		case "amount":
			foundAmount = issue.ProviderField == "f6" && issue.Code == "missing_or_invalid_value" &&
				issue.RowIndex != nil && *issue.RowIndex == 0
		case "index_coverage":
			foundCoverage = issue.ProviderField == "Eastmoney core index allowlist" &&
				issue.Code == "allowlisted_index_unavailable" && issue.RowIndex == nil &&
				issue.ActualPreview != ""
		}
	}
	if !foundTime || !foundAmount || !foundCoverage {
		t.Fatalf("missing index quote Reporter evidence: %+v", issues)
	}
}
