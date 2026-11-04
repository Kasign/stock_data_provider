package validation

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

func floatPointer(value float64) *float64 { return &value }
func intPointer(value int64) *int64       { return &value }

func TestValidateQuoteReturnsDeterministicCompleteIssuesWithoutMutation(t *testing.T) {
	quote := &model.Quote{
		Symbol: "600000.SH", Source: "test",
		CurrentPrice: floatPointer(10), Open: floatPointer(10), High: floatPointer(11),
		Low: floatPointer(9), PrevClose: floatPointer(10),
		Volume: intPointer(100), VolumeUnit: "shares",
		Amount: nil, AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityDegraded,
	}
	first := ValidateQuote(quote)
	second := ValidateQuote(quote)
	if len(first.Issues) != 1 || first.Issues[0].Code != "missing_optional" {
		t.Fatalf("unexpected issues: %+v", first.Issues)
	}
	if len(second.Issues) != len(first.Issues) || second.Issues[0] != first.Issues[0] {
		t.Fatalf("validation is not deterministic: first=%+v second=%+v", first, second)
	}
	if quote.AmountUnit != "CNY" || len(quote.QualityReasons) != 0 {
		t.Fatalf("validator mutated input quote: %+v", quote)
	}
}

func TestValidateQuoteCollectsMultipleIssuesInStableOrder(t *testing.T) {
	quote := &model.Quote{Symbol: "bad", QualityStatus: model.QualityInvalid}
	report := ValidateQuote(quote)
	want := []string{"symbol", "source", "quality_status", "adjustment_basis", "current_price", "open", "high", "low", "prev_close", "volume", "volume_unit"}
	if len(report.Issues) < len(want) {
		t.Fatalf("expected at least %d issues, got %+v", len(want), report.Issues)
	}
	for index, field := range want {
		if report.Issues[index].Field != field {
			t.Fatalf("issue[%d].Field = %q, want %q", index, report.Issues[index].Field, field)
		}
	}
}

func TestValidateDailyBarChecksIdentityAndPreservesInput(t *testing.T) {
	bar := model.DailyBar{
		Symbol: "000001.SZ", TradeDate: "2026-10-30", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10,
		Volume: intPointer(100), VolumeUnit: "shares",
		Amount: floatPointer(1000), AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, Source: "test", QualityStatus: model.QualityValid,
	}
	report := ValidateDailyBar(bar, DailyBarContext{RequestedSymbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-11-01"})
	if len(report.Issues) == 0 || report.Issues[0].Code != "identity_mismatch" {
		t.Fatalf("expected identity mismatch, got %+v", report.Issues)
	}
	if bar.Symbol != "000001.SZ" {
		t.Fatalf("validator mutated bar symbol: %s", bar.Symbol)
	}
}

func TestReportTruncatesAndCountsAllIssues(t *testing.T) {
	report := NewReport(2)
	for i := 0; i < 4; i++ {
		report.Add(issue(StageParse, "field", "type_mismatch", "number", "bad type", SeverityError))
	}
	if report.TotalCount != 4 || len(report.Issues) != 2 || !report.Truncated || !report.HasErrors() {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidateQuoteRejectsNonFiniteNumbers(t *testing.T) {
	quote := &model.Quote{
		Symbol: "600000.SH", Source: "test",
		CurrentPrice: floatPointer(math.NaN()), Open: floatPointer(10), High: floatPointer(11),
		Low: floatPointer(9), PrevClose: floatPointer(10),
		Volume: intPointer(1), VolumeUnit: "shares", AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityDegraded,
	}
	report := ValidateQuote(quote)
	found := false
	for _, item := range report.Issues {
		if item.Field == "current_price" && item.Code == "out_of_range" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected current_price non-finite issue, got %+v", report.Issues)
	}
}

func TestReportHasErrorsWhenErrorIssueIsTruncated(t *testing.T) {
	report := NewReport(1)
	report.Add(issue(StageParse, "optional", "missing_optional", "optional", "not supplied", SeverityWarning))
	report.Add(issue(StageParse, "price", "type_mismatch", "number", "wrong type", SeverityError))
	if len(report.Issues) != 1 || !report.Truncated {
		t.Fatalf("expected only first issue to be retained: %+v", report)
	}
	if report.ErrorCount != 1 || report.WarningCount != 1 || !report.HasErrors() {
		t.Fatalf("truncated error must still fail the report: %+v", report)
	}
}

func TestReportMergePreservesSeverityCountsBeyondIssueLimit(t *testing.T) {
	left := NewReport(1)
	left.Add(issue(StageParse, "a", "missing_optional", "optional", "missing", SeverityWarning))
	left.Add(issue(StageParse, "b", "type_mismatch", "number", "bad type", SeverityError))
	right := NewReport(2)
	right.Add(issue(StageValidate, "c", "out_of_range", "positive", "bad range", SeverityError))
	right.Add(issue(StageValidate, "d", "notice", "n/a", "notice", SeverityInfo))

	merged := left.Merge(right)
	if merged.TotalCount != 4 || merged.ErrorCount != 2 || merged.WarningCount != 1 || merged.InfoCount != 1 {
		t.Fatalf("severity counters were not preserved: %+v", merged)
	}
	if !merged.HasErrors() || !merged.Truncated {
		t.Fatalf("merged report must preserve failure and truncation: %+v", merged)
	}
}

func TestReporterIsContextScopedAndMergesConcurrentReports(t *testing.T) {
	reporter := NewReporter(10)
	ctx := WithReporter(context.Background(), reporter)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			AddContextReport(ctx, Report{TotalCount: 1, ErrorCount: 1, Issues: []FieldIssue{
				issue(StageParse, "price", "type_mismatch", "number", "invalid type", SeverityError),
			}})
		}()
	}
	wg.Wait()
	got := reporter.Snapshot()
	if got.TotalCount != 4 || got.ErrorCount != 4 || !got.HasErrors() || len(got.Issues) != 4 {
		t.Fatalf("unexpected concurrent report aggregation: %+v", got)
	}
	if ReporterFromContext(ctx) != reporter {
		t.Fatal("reporter was not retained in request context")
	}
	if ReporterFromContext(context.Background()) != nil {
		t.Fatal("reporter leaked into unrelated context")
	}
}

func TestReportAddCountsAggregatedRows(t *testing.T) {
	report := NewReport(1)
	report.Add(FieldIssue{Stage: StageNormalize, Code: "bars_truncated_to_limit", Severity: SeverityInfo, Count: 7})
	report.Add(FieldIssue{Stage: StageNormalize, Code: "duplicate_trade_date_dropped", Severity: SeverityWarning})
	if report.TotalCount != 8 || report.InfoCount != 7 || report.WarningCount != 1 || len(report.Issues) != 1 || !report.Truncated {
		t.Fatalf("unexpected aggregated report: %+v", report)
	}
}

func TestReportMergePreservesAggregatedCountsWithoutMarkingTruncated(t *testing.T) {
	left := NewReport(10)
	left.Add(FieldIssue{Stage: StageNormalize, Code: "bars_truncated_to_limit", Severity: SeverityInfo, Count: 7})
	right := NewReport(10)
	right.Add(FieldIssue{Stage: StageNormalize, Code: "duplicate_trade_date_dropped", Severity: SeverityWarning, Count: 3})

	merged := left.Merge(right)
	if merged.TotalCount != 10 || merged.InfoCount != 7 || merged.WarningCount != 3 {
		t.Fatalf("aggregated severity counts were not preserved: %+v", merged)
	}
	if merged.Truncated || len(merged.Issues) != 2 {
		t.Fatalf("aggregated rows must not imply issue-detail truncation: %+v", merged)
	}
}

func TestReportSeverityCountsFallbackUsesAggregatedIssueCounts(t *testing.T) {
	report := Report{
		TotalCount: 8,
		Issues: []FieldIssue{
			{Severity: SeverityInfo, Count: 7},
			{Severity: SeverityWarning},
		},
	}
	errors, warnings, info := report.severityCounts()
	if errors != 0 || warnings != 1 || info != 7 {
		t.Fatalf("fallback severity counts ignored FieldIssue.Count: error=%d warning=%d info=%d", errors, warnings, info)
	}
}
