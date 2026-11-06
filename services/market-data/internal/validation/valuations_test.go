package validation

import (
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

func valuationFloatPointer(v float64) *float64 { return &v }

func validValuationSnapshot() *model.ValuationSnapshot {
	now := time.Now()
	return &model.ValuationSnapshot{
		RequestedSymbols: []string{"600000.SH"}, Total: 1,
		ProviderReportDate: now.Format("2006-01-02"), ProviderTimestamp: &now,
		Source: "test", SourceVersion: "v1", FetchedAt: now, QualityStatus: model.QualityValid,
		Items: []model.ValuationItem{{
			Symbol: "600000.SH", Code: "600000", Source: "test", SourceVersion: "v1",
			FetchedAt: now, QualityStatus: model.QualityValid,
			PETTM: valuationFloatPointer(-3.2), PBMRQ: valuationFloatPointer(1.1),
		}},
	}
}

func TestValidateValuationSnapshotAllowsNegativeMultiplesAndNullableMetrics(t *testing.T) {
	report := ValidateValuationSnapshot(validValuationSnapshot(), []string{"600000.SH"})
	if report.HasErrors() {
		t.Fatalf("expected valid snapshot, got %+v", report.Issues)
	}
	for _, issue := range report.Issues {
		if issue.Code == "value_not_provided" && issue.Severity != SeverityInfo {
			t.Fatalf("nullable metric must remain informational: %+v", issue)
		}
	}
}

func TestValidateValuationSnapshotReportsFieldAndCoverageIssues(t *testing.T) {
	snapshot := validValuationSnapshot()
	snapshot.Items[0].TurnoverRatePercent = valuationFloatPointer(-1)
	snapshot.RequestedSymbols = []string{"600000.SH", "600001.SH"}
	snapshot.MissingSymbols = []string{"600001.SH"}
	report := ValidateValuationSnapshot(snapshot, []string{"600000.SH", "600001.SH"})
	if !report.HasErrors() {
		t.Fatal("expected negative ratio to be an error")
	}
	foundNegative, foundMissing := false, false
	for _, issue := range report.Issues {
		foundNegative = foundNegative || issue.Field == "turnover_rate_percent" && issue.Code == "negative_ratio"
		foundMissing = foundMissing || issue.Code == "requested_symbol_missing"
	}
	if !foundNegative || !foundMissing {
		t.Fatalf("expected negative ratio and missing symbol issues, got %+v", report.Issues)
	}
}

func TestValidateValuationSnapshotNil(t *testing.T) {
	report := ValidateValuationSnapshot(nil, []string{"600000.SH"})
	if !report.HasErrors() || len(report.Issues) != 1 || report.Issues[0].Code != "required" {
		t.Fatalf("unexpected nil snapshot report: %+v", report)
	}
}
