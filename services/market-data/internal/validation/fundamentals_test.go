package validation

import (
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

func validFundamentalsFixture() *model.FundamentalsSnapshot {
	sections := make([]model.FundamentalSection, 0, 6)
	for _, category := range []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"} {
		sections = append(sections, model.FundamentalSection{
			Category: category, ReportDate: "2026-06-30", PublicationDate: "2026-08-20",
			HasData: true, QualityStatus: model.QualityValid,
			Metrics: []model.FundamentalMetric{{Key: "metric", Name: "指标", Value: 1, Unit: "ratio", SourceField: "provider_metric"}},
		})
	}
	return &model.FundamentalsSnapshot{
		Symbol: "600036.SH", Year: 2026, Quarter: 2, HasData: true, Sections: sections,
		Source: "test", SourceVersion: "test-v1", FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
	}
}

func TestValidateFundamentalsSnapshotReportsFieldLevelIssues(t *testing.T) {
	snapshot := validFundamentalsFixture()
	snapshot.Sections[0].PublicationDate = "not-a-date"
	snapshot.Sections[1].Metrics[0].Unit = ""
	snapshot.Sections[2].Metrics = append(snapshot.Sections[2].Metrics, snapshot.Sections[2].Metrics[0])
	report := ValidateFundamentalsSnapshot(snapshot, "600036.SH", 2026, 2)
	if report.ErrorCount < 3 {
		t.Fatalf("expected field-level errors, got %+v", report)
	}
	if len(report.Issues) == 0 || report.Issues[0].Stage != StageValidate {
		t.Fatalf("expected validation stage issues, got %+v", report.Issues)
	}
}

func TestValidateFundamentalsSnapshotAcceptsValidSnapshot(t *testing.T) {
	report := ValidateFundamentalsSnapshot(validFundamentalsFixture(), "600036.SH", 2026, 2)
	if report.HasErrors() {
		t.Fatalf("expected valid fundamentals snapshot, got %+v", report)
	}
}
