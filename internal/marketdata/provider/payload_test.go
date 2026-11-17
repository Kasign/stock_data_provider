package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestReadBoundedBodyRejectsOversizedPayload(t *testing.T) {
	if _, err := readBoundedBody(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("expected oversized payload rejection")
	}
	body, err := readBoundedBody(strings.NewReader("1234"), 4)
	if err != nil || string(body) != "1234" {
		t.Fatalf("exact-limit payload should pass: body=%q err=%v", body, err)
	}
}

func TestDecodeSingleJSONRejectsTrailingValuesAndJunk(t *testing.T) {
	var target map[string]any
	for _, payload := range []string{`{"ok":true}{"other":true}`, `{"ok":true}junk`} {
		if err := decodeSingleJSON([]byte(payload), &target); err == nil {
			t.Fatalf("expected strict JSON rejection for %q", payload)
		}
	}
	if err := decodeSingleJSON(bytes.TrimSpace([]byte(" {\"ok\":true} ")), &target); err != nil {
		t.Fatalf("valid single JSON should pass: %v", err)
	}
}

func TestValidProviderSnapshotTimeRejectsZeroAndFutureValues(t *testing.T) {
	reference := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	withinSkew := reference.Add(5 * time.Minute)
	past := reference.Add(-24 * time.Hour)
	future := reference.Add(5*time.Minute + time.Nanosecond)

	if got := validProviderSnapshotTime(nil, reference); got != nil {
		t.Fatalf("nil timestamp should remain nil, got %v", got)
	}
	zero := time.Time{}
	if got := validProviderSnapshotTime(&zero, reference); got != nil {
		t.Fatalf("zero timestamp should be rejected, got %v", got)
	}
	if got := validProviderSnapshotTime(&future, reference); got != nil {
		t.Fatalf("timestamp beyond the allowed future skew should be rejected, got %v", got)
	}
	if got := validProviderSnapshotTime(&withinSkew, reference); got == nil || !got.Equal(withinSkew) {
		t.Fatalf("timestamp exactly at the allowed skew should be accepted, got %v", got)
	}
	if got := validProviderSnapshotTime(&past, reference); got == nil || !got.Equal(past) {
		t.Fatalf("older timestamp should remain valid for freshness handling, got %v", got)
	}
}

func TestNormalizeProviderDailyBarsWithReportKeepsOriginalRows(t *testing.T) {
	ctx := validation.WithReporter(context.Background(), validation.NewReporter(20))
	volume := int64(1000)
	amount := 10000.0
	valid := model.DailyBar{
		Symbol: "600000.SH", TradeDate: "2026-10-30", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &volume, Amount: &amount, VolumeUnit: "shares", AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, Source: "fixture", QualityStatus: model.QualityValid,
	}
	mismatch := valid
	mismatch.Symbol = "000001.SZ"
	mismatch.TradeDate = "2026-10-29" // 不同交易日，确保身份错误行不能靠重复日期过滤掩盖
	bars := []model.DailyBar{valid, mismatch, valid}
	got, err := normalizeProviderDailyBarsWithReport(ctx, "600000.SH", "2026-10-29", "2026-10-30", 10, bars, 0)
	if err != nil {
		t.Fatalf("normalizeProviderDailyBarsWithReport() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("normalized bars = %d, want 1", len(got))
	}
	report := validation.ReporterFromContext(ctx).Snapshot()
	foundMismatch, foundDuplicate := false, false
	for _, item := range report.Issues {
		if item.Code == "identity_mismatch" && item.RowIndex != nil && *item.RowIndex == 1 {
			foundMismatch = true
		}
		if item.Code == "duplicate_trade_date" && item.RowIndex != nil && *item.RowIndex == 2 {
			foundDuplicate = true
		}
	}
	if !foundMismatch || !foundDuplicate {
		t.Fatalf("expected original-row identity and duplicate diagnostics, got %+v", report)
	}
}

func TestProviderJSONValueTypeAndPreview(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"null", "null"}, {"\"abc\"", "string"}, {"12.5", "number"},
		{"true", "boolean"}, {"{}", "object"}, {"[]", "array"}, {"", "missing"}, {"{", "invalid"},
	}
	for _, item := range cases {
		if got := providerJSONValueType([]byte(item.raw)); got != item.want {
			t.Errorf("providerJSONValueType(%q) = %q, want %q", item.raw, got, item.want)
		}
	}
	if got := providerJSONPreview(json.RawMessage("{\"secret\":\"not-exposed\"}")); got != "" {
		t.Fatalf("object preview must be omitted, got %q", got)
	}
}

func TestNormalizeProviderDailyBarsDegradesUnverifiedForwardAdjustment(t *testing.T) {
	volume := int64(1000)
	amount := 105000.0
	bars := []model.DailyBar{{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 100, High: 110, Low: 90, Close: 105, Volume: &volume, Amount: &amount,
		VolumeUnit: "shares", AmountUnit: "CNY", Source: "test-provider", SourceVersion: "test-v1",
		AdjustmentBasis: model.AdjustmentForward, QualityStatus: model.QualityValid,
	}}
	got, err := normalizeProviderDailyBars("600519.SH", "2026-10-20", "2026-10-20", 10, bars, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("forward adjustment without method/reference must be degraded: %+v", got)
	}
	if !strings.Contains(strings.Join(got[0].QualityReasons, ","), "daily_bar_adjustment_method_unreported") || !strings.Contains(strings.Join(got[0].QualityReasons, ","), "daily_bar_adjustment_reference_date_unverified") {
		t.Fatalf("missing adjustment metadata reasons: %+v", got[0].QualityReasons)
	}
}

func TestNormalizeProviderDailyBarsAllowsCumulativeHFQWithoutReferenceDate(t *testing.T) {
	volume := int64(1000)
	amount := 105000.0
	bars := []model.DailyBar{{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 100, High: 110, Low: 90, Close: 105, Volume: &volume, Amount: &amount,
		VolumeUnit: "shares", AmountUnit: "CNY", Source: "test-provider", SourceVersion: "test-v1",
		AdjustmentBasis: model.AdjustmentBackward, AdjustmentMethod: "tushare_adj_factor_hfq_v1", QualityStatus: model.QualityValid,
	}}
	got, err := normalizeProviderDailyBars("600519.SH", "2026-10-20", "2026-10-20", 10, bars, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].QualityStatus != model.QualityValid {
		t.Fatalf("cumulative HFQ without request reference date should remain valid: %+v", got)
	}
}
