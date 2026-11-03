package provider

import (
	"context"
	"math"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/validation"
)

func TestNormalizePytdxDailyAmountPreservesConsistentValues(t *testing.T) {
	got, err := normalizePytdxDailyAmount(12345.0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount == nil || *got.Amount != 12345 || got.RawAmount == nil || *got.RawAmount != 12345 {
		t.Fatalf("amount values should be preserved: %+v", got)
	}
	if got.AmountUnit != "CNY" || got.RawAmountUnit != "CNY" || got.QualityStatus != model.QualityValid {
		t.Fatalf("consistent amount should retain CNY and valid quality: %+v", got)
	}
}

func TestNormalizePytdxDailyAmountDegradesZeroMismatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount float64
		volume float64
	}{
		{name: "positive amount without volume", amount: 12345, volume: 0},
		{name: "positive volume without amount", amount: 0, volume: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePytdxDailyAmount(tc.amount, tc.volume)
			if err != nil {
				t.Fatal(err)
			}
			if got.Amount != nil || got.AmountUnit != "unknown" || got.RawAmount == nil {
				t.Fatalf("mismatched amount should be unknown but auditable: %+v", got)
			}
			if got.QualityStatus != model.QualityDegraded || len(got.QualityReasons) == 0 {
				t.Fatalf("mismatched amount must degrade the daily bar: %+v", got)
			}
		})
	}
}

func TestNormalizePytdxDailyAmountRejectsInvalidNumbers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount float64
		volume float64
	}{
		{name: "negative amount", amount: -1, volume: 1},
		{name: "nan amount", amount: math.NaN(), volume: 1},
		{name: "infinite amount", amount: math.Inf(1), volume: 1},
		{name: "fractional hands", amount: 100, volume: 1.5},
		{name: "negative hands", amount: 100, volume: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := normalizePytdxDailyAmount(tc.amount, tc.volume); err == nil {
				t.Fatalf("invalid amount/volume pair should be rejected")
			}
		})
	}
}

func TestValidatePytdxDailyBarValuesReportsFieldIssue(t *testing.T) {
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	_, err := validatePytdxDailyBarValues(ctx, 7, "2026-10-20", 10, 11, 9, math.NaN(), 100, 10000)
	if err == nil {
		t.Fatal("expected invalid close price to fail")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 {
		t.Fatalf("expected one structured issue, got %+v", issues)
	}
	issue := issues[0]
	if issue.Stage != validation.StageValidate || issue.Field != "close" || issue.ProviderField != "Close" || issue.RowIndex == nil || *issue.RowIndex != 7 {
		t.Fatalf("unexpected structured issue: %+v", issue)
	}
}

func TestValidatePytdxDailyBarValuesReportsAmountVolumeMismatch(t *testing.T) {
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	info, err := validatePytdxDailyBarValues(ctx, 2, "2026-10-20", 10, 11, 9, 10.5, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if info.Amount != nil || info.QualityStatus != model.QualityDegraded {
		t.Fatalf("mismatched amount must remain null and degraded: %+v", info)
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Code != "amount_volume_mismatch" || issues[0].RowIndex == nil || *issues[0].RowIndex != 2 {
		t.Fatalf("expected amount/volume mismatch at row 2, got %+v", issues)
	}
}

func TestBuildPytdxQuoteReportsInvalidPrice(t *testing.T) {
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	_, err := buildPytdxQuote(ctx, "600519.SH", math.NaN(), 9.8, 10, 10.5, 9.5, 100, 10000)
	if err == nil {
		t.Fatal("expected non-finite current price to fail")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageValidate || issues[0].Field != "current_price" || issues[0].ProviderField != "Close" || issues[0].Code != "invalid_number" {
		t.Fatalf("expected structured current-price issue, got %+v", issues)
	}
}

func TestBuildPytdxQuoteReportsOHLCAndVolumeIssues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		current   float64
		prevClose float64
		open      float64
		high      float64
		low       float64
		volume    float64
		wantField string
		wantCode  string
	}{
		{name: "ohlc range", current: 12, prevClose: 10, open: 10, high: 11, low: 9, volume: 100, wantField: "ohlc", wantCode: "ohlc_invariant_violation"},
		{name: "fractional hands", current: 10, prevClose: 9, open: 9.5, high: 11, low: 9, volume: 1.5, wantField: "volume", wantCode: "invalid_number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reporter := validation.NewReporter(20)
			ctx := validation.WithReporter(context.Background(), reporter)
			_, err := buildPytdxQuote(ctx, "600519.SH", tc.current, tc.prevClose, tc.open, tc.high, tc.low, tc.volume, 10000)
			if err == nil {
				t.Fatal("expected invalid quote values to fail")
			}
			issues := reporter.Snapshot().Issues
			if len(issues) != 1 || issues[0].Field != tc.wantField || issues[0].Code != tc.wantCode {
				t.Fatalf("expected %s/%s issue, got %+v", tc.wantField, tc.wantCode, issues)
			}
		})
	}
}

func TestBuildPytdxQuoteReportsInvalidOptionalAmountAndDegrades(t *testing.T) {
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	quote, err := buildPytdxQuote(ctx, "600519.SH", 10, 9.8, 9.9, 10.5, 9.5, 100, math.NaN())
	if err != nil {
		t.Fatal(err)
	}
	if quote.Amount != nil || quote.QualityStatus != model.QualityDegraded {
		t.Fatalf("invalid optional amount must remain null and degraded: %+v", quote)
	}
	issues := reporter.Snapshot().Issues
	foundInvalidAmount := false
	for _, issue := range issues {
		if issue.Field == "amount" && issue.Code == "invalid_optional_number" && issue.Severity == validation.SeverityWarning {
			foundInvalidAmount = true
			break
		}
	}
	if !foundInvalidAmount {
		t.Fatalf("expected warning for invalid optional amount, got %+v", issues)
	}
}
