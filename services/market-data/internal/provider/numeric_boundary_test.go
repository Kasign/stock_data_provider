package provider

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestHithinkInt64VolumeRejectsFloat64OverflowBoundary(t *testing.T) {
	boundary := float64(math.MaxInt64)
	if _, ok := hithinkNonNegativeInt64(&boundary); ok {
		t.Fatal("2^63 must not be converted to int64")
	}
	below := math.Nextafter(boundary, 0)
	got, ok := hithinkNonNegativeInt64(&below)
	if !ok || got < 0 || float64(got) != below {
		t.Fatalf("largest representable float below 2^63 should convert safely: got=%d ok=%v value=%.0f", got, ok, below)
	}
}

func TestHithinkMillisDateRejectsFloat64OverflowBoundary(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hithinkDateFromMillis(float64(math.MaxInt64), location); ok {
		t.Fatal("2^63 milliseconds must not be converted to int64")
	}
	millis := float64(time.Date(2026, time.October, 20, 0, 0, 0, 0, location).UnixMilli())
	got, ok := hithinkDateFromMillis(millis, location)
	if !ok || got != "2026-10-20" {
		t.Fatalf("valid millisecond timestamp converted incorrectly: got=%q ok=%v", got, ok)
	}
}

func TestTickflowInt64RejectsOutOfRangeFloat64Values(t *testing.T) {
	for _, raw := range []string{"9223372036854775808", "-9223372036854777856"} {
		if _, err := tickflowInt64(json.RawMessage(raw)); err == nil {
			t.Errorf("out-of-range integer %s must be rejected", raw)
		}
	}
	const validTimestamp int64 = 1792454400000
	got, err := tickflowInt64(json.RawMessage("1792454400000"))
	if err != nil || got != validTimestamp {
		t.Fatalf("valid timestamp conversion got=%d err=%v want=%d", got, err, validTimestamp)
	}
}

func TestTencentQuoteVolumeRejectsOverflowBoundary(t *testing.T) {
	boundary := float64(math.MaxInt64) / 100
	if _, _, _, err := tencentQuoteVolume(nil, boundary, 10); err == nil {
		t.Fatal("normalized-volume overflow boundary must be rejected before unit inference")
	}
}

func TestNormalizePytdxDailyAmountOmitsInconsistentZeroAmount(t *testing.T) {
	got, err := normalizePytdxDailyAmount(0, 120)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != nil || got.AmountUnit != "unknown" || got.RawAmount == nil ||
		*got.RawAmount != 0 || got.RawAmountUnit != "CNY" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("zero amount with positive volume should be retained as raw but omitted from normalized data: %+v", got)
	}
	if len(got.QualityReasons) != 1 || got.QualityReasons[0] != "amount_volume_zero_mismatch_amount_omitted" {
		t.Fatalf("missing amount/volume mismatch reason: %+v", got.QualityReasons)
	}
}

func TestNormalizePytdxDailyAmountOmitsPositiveAmountWithZeroVolume(t *testing.T) {
	got, err := normalizePytdxDailyAmount(1500, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount != nil || got.AmountUnit != "unknown" || got.RawAmount == nil ||
		*got.RawAmount != 1500 || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("positive amount with zero volume should be marked unavailable: %+v", got)
	}
}

func TestNormalizePytdxDailyAmountKeepsConsistentAmounts(t *testing.T) {
	got, err := normalizePytdxDailyAmount(1500, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.Amount == nil || *got.Amount != 1500 || got.AmountUnit != "CNY" ||
		got.QualityStatus != model.QualityValid || got.RawAmount == nil || *got.RawAmount != 1500 {
		t.Fatalf("consistent amount should remain valid: %+v", got)
	}
	zero, err := normalizePytdxDailyAmount(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if zero.Amount == nil || *zero.Amount != 0 || zero.QualityStatus != model.QualityValid {
		t.Fatalf("zero amount and zero volume may represent a no-trade bar: %+v", zero)
	}
}

func TestNormalizePytdxDailyAmountRejectsInvalidNumbers(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := normalizePytdxDailyAmount(value, 10); err == nil {
			t.Errorf("invalid raw amount %v must be rejected", value)
		}
	}
}
