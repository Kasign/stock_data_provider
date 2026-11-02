package validation

import (
	"math"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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
